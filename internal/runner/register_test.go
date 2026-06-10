package runner

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zairedegrees/fleet/internal/config"
	"github.com/zairedegrees/fleet/internal/relay"
)

type rpcCall struct {
	Tool string
	Args map[string]interface{}
}

// captureRelay spins a fake relay that records every tools/call. failTools
// lists tool names that must answer with an isError result.
func captureRelay(t *testing.T, failTools ...string) (*httptest.Server, *[]rpcCall) {
	t.Helper()
	calls := &[]rpcCall{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		data, _ := io.ReadAll(r.Body)
		var req struct {
			Params struct {
				Name      string                 `json:"name"`
				Arguments map[string]interface{} `json:"arguments"`
			} `json:"params"`
		}
		if err := json.Unmarshal(data, &req); err != nil {
			t.Errorf("relay received invalid JSON: %v", err)
		}
		*calls = append(*calls, rpcCall{Tool: req.Params.Name, Args: req.Params.Arguments})
		w.Header().Set("Content-Type", "application/json")
		for _, ft := range failTools {
			if ft == req.Params.Name {
				fmt.Fprint(w, `{"jsonrpc":"2.0","id":1,"result":{"content":[{"type":"text","text":"nope"}],"isError":true}}`)
				return
			}
		}
		fmt.Fprint(w, `{"jsonrpc":"2.0","id":1,"result":{"content":[{"type":"text","text":"{\"ok\":true}"}]}}`)
	}))
	t.Cleanup(srv.Close)
	return srv, calls
}

func callsFor(calls []rpcCall, tool string) []rpcCall {
	var out []rpcCall
	for _, c := range calls {
		if c.Tool == tool {
			out = append(out, c)
		}
	}
	return out
}

// Registration moved out of the generated bash into the typed client: each
// agent must get a register_profile AND a register_agent with the exact
// battle-tested curl argument shape — profile_slug included, or the agent
// never sees dispatched tasks.
func TestRegisterFleetRegistersProfilesAndAgents(t *testing.T) {
	srv, calls := captureRelay(t)
	cfg := &config.FleetConfig{
		Project: config.ProjectConfig{Name: "proj", Cwd: t.TempDir()},
		Agents: []config.AgentConfig{
			{Name: "dev", Color: "green", Role: "Developer"},
			{Name: "lead", Color: "red", Role: "Tech Lead — décide"},
		},
	}

	if err := registerFleet(cfg, relay.NewClient(srv.URL)); err != nil {
		t.Fatalf("registerFleet failed: %v", err)
	}

	profiles := callsFor(*calls, "register_profile")
	if len(profiles) != 2 {
		t.Fatalf("expected 2 register_profile calls, got %d", len(profiles))
	}
	if got := profiles[0].Args; got["slug"] != "dev" || got["name"] != "dev" || got["role"] != "Developer" || got["project"] != "proj" {
		t.Errorf("register_profile args wrong: %v", got)
	}

	agents := callsFor(*calls, "register_agent")
	if len(agents) != 2 {
		t.Fatalf("expected 2 register_agent calls, got %d", len(agents))
	}
	if got := agents[0].Args; got["name"] != "dev" || got["project"] != "proj" || got["role"] != "Developer" || got["profile_slug"] != "dev" {
		t.Errorf("register_agent args wrong: %v", got)
	}
	if got := agents[1].Args; got["name"] != "lead" || got["role"] != "Tech Lead — décide" || got["profile_slug"] != "lead" {
		t.Errorf("register_agent must carry non-ASCII roles intact + profile_slug: %v", got)
	}
}

// Vault docs are pushed via set_memory with the same key/scope/tags shape as
// the curl the script used to emit.
func TestRegisterFleetPushesVaultDocs(t *testing.T) {
	srv, calls := captureRelay(t)
	dir := t.TempDir()
	vaultShared := filepath.Join(dir, ".fleet", "vault", "shared")
	if err := os.MkdirAll(vaultShared, 0755); err != nil {
		t.Fatal(err)
	}
	content := "# Arch\nline 'two' with \"quotes\"\n"
	if err := os.WriteFile(filepath.Join(vaultShared, "arch.md"), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	cfg := &config.FleetConfig{
		Project: config.ProjectConfig{Name: "proj", Cwd: dir},
		Agents:  []config.AgentConfig{{Name: "dev", Color: "green", Role: "Dev"}},
	}

	if err := registerFleet(cfg, relay.NewClient(srv.URL)); err != nil {
		t.Fatalf("registerFleet failed: %v", err)
	}

	mems := callsFor(*calls, "set_memory")
	if len(mems) != 1 {
		t.Fatalf("expected 1 set_memory call, got %d", len(mems))
	}
	got := mems[0].Args
	if got["key"] != "vault:shared/arch.md" || got["scope"] != "project" || got["project"] != "proj" {
		t.Errorf("set_memory key/scope/project wrong: %v", got)
	}
	if got["value"] != content {
		t.Errorf("vault content must arrive verbatim, got %q", got["value"])
	}
	tags, _ := got["tags"].([]interface{})
	if len(tags) != 2 || tags[0] != "vault" || tags[1] != "auto-injected" {
		t.Errorf("expected tags [vault auto-injected], got %v", got["tags"])
	}
}

// A failed registration must surface with the agent's name — and must not
// stop the remaining agents from being registered.
func TestRegisterFleetSurfacesFailureAndContinues(t *testing.T) {
	srv, calls := captureRelay(t, "register_agent")
	cfg := &config.FleetConfig{
		Project: config.ProjectConfig{Name: "proj", Cwd: t.TempDir()},
		Agents: []config.AgentConfig{
			{Name: "dev", Color: "green", Role: "Dev"},
			{Name: "ops", Color: "blue", Role: "Ops"},
		},
	}

	err := registerFleet(cfg, relay.NewClient(srv.URL))
	if err == nil {
		t.Fatal("expected registration failures to surface, got nil")
	}
	if !strings.Contains(err.Error(), "dev") || !strings.Contains(err.Error(), "ops") {
		t.Errorf("error should name every failed agent, got: %v", err)
	}
	if got := len(callsFor(*calls, "register_agent")); got != 2 {
		t.Errorf("one failure must not stop the other registrations, got %d register_agent calls", got)
	}
}
