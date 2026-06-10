package runner

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
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
	stubExec(t) // every tmux has-session succeeds
	srv, calls := captureRelay(t)
	cfg := &config.FleetConfig{
		Project: config.ProjectConfig{Name: "proj", Cwd: t.TempDir()},
		Agents: []config.AgentConfig{
			{Name: "dev", Color: "green", Role: "Developer", ReportsTo: "lead"},
			{Name: "lead", Color: "red", Role: "Tech Lead — décide", IsExecutive: true},
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
	if got := agents[0].Args; got["reports_to"] != "lead" || got["is_executive"] != false {
		t.Errorf("register_agent must carry the config hierarchy (full-replace relay resets omitted fields): %v", got)
	}
	if got := agents[1].Args; got["name"] != "lead" || got["role"] != "Tech Lead — décide" || got["profile_slug"] != "lead" {
		t.Errorf("register_agent must carry non-ASCII roles intact + profile_slug: %v", got)
	}
	if got := agents[1].Args; got["is_executive"] != true || got["reports_to"] != "" {
		t.Errorf("register_agent must carry is_executive for executives: %v", got)
	}
}

// Vault docs are pushed via set_memory with the same key/scope/tags shape as
// the curl the script used to emit.
func TestRegisterFleetPushesVaultDocs(t *testing.T) {
	stubExec(t)
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

// captureStdout redirects os.Stdout around fn and returns what was printed.
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	orig := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w
	defer func() { os.Stdout = orig }()
	fn()
	w.Close()
	out, _ := io.ReadAll(r)
	return string(out)
}

// "✓ vault injected" used to print unconditionally — even "✓ ... 0 docs"
// after a total push failure. ✓ is only honest when every doc was pushed;
// anything less is a ⚠ with the real N/M count.
func TestRegisterFleetVaultOutputHonesty(t *testing.T) {
	vaultCfg := func(t *testing.T) *config.FleetConfig {
		dir := t.TempDir()
		vaultShared := filepath.Join(dir, ".fleet", "vault", "shared")
		if err := os.MkdirAll(vaultShared, 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(vaultShared, "arch.md"), []byte("# Arch"), 0644); err != nil {
			t.Fatal(err)
		}
		return &config.FleetConfig{
			Project: config.ProjectConfig{Name: "proj", Cwd: dir},
			Agents:  []config.AgentConfig{{Name: "dev", Color: "green", Role: "Dev"}},
		}
	}

	t.Run("checkmark only when every doc pushed", func(t *testing.T) {
		stubExec(t)
		srv, _ := captureRelay(t)
		cfg := vaultCfg(t)
		out := captureStdout(t, func() {
			if err := registerFleet(cfg, relay.NewClient(srv.URL)); err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
		if !strings.Contains(out, "✓ vault injected for dev: 1 docs") {
			t.Errorf("expected ✓ for a full push, got:\n%s", out)
		}
	})

	t.Run("warning with real count on failure", func(t *testing.T) {
		stubExec(t)
		srv, _ := captureRelay(t, "set_memory")
		cfg := vaultCfg(t)
		out := captureStdout(t, func() {
			if err := registerFleet(cfg, relay.NewClient(srv.URL)); err == nil {
				t.Error("expected the vault failure to surface")
			}
		})
		if strings.Contains(out, "✓ vault injected") {
			t.Errorf("✓ must not lie when docs failed, got:\n%s", out)
		}
		if !strings.Contains(out, "⚠ vault for dev: 0/1 docs pushed") {
			t.Errorf("expected '⚠ vault for dev: 0/1 docs pushed', got:\n%s", out)
		}
	})
}

// A partially-failed launch must not leave ghosts on the relay: an agent whose
// tmux session never started is skipped entirely (no profile, no register, no
// vault) and named honestly in the joined error.
func TestRegisterFleetSkipsAgentsWithoutSession(t *testing.T) {
	srv, calls := captureRelay(t)
	orig := execCommand
	execCommand = func(name string, arg ...string) *exec.Cmd {
		// has-session fails only for ghost — its launch never created a pane.
		if len(arg) > 0 && arg[0] == "has-session" && strings.Contains(strings.Join(arg, " "), "ghost") {
			return exec.Command("false")
		}
		return exec.Command("true")
	}
	t.Cleanup(func() { execCommand = orig })

	cfg := &config.FleetConfig{
		Project: config.ProjectConfig{Name: "proj", Cwd: t.TempDir()},
		Agents: []config.AgentConfig{
			{Name: "dev", Color: "green", Role: "Dev"},
			{Name: "ghost", Color: "blue", Role: "Ops"},
		},
	}

	err := registerFleet(cfg, relay.NewClient(srv.URL))
	if err == nil || !strings.Contains(err.Error(), "skip register ghost: no tmux session") {
		t.Errorf("expected the skipped agent named in the joined error, got: %v", err)
	}
	for _, c := range *calls {
		if c.Args["name"] == "ghost" || c.Args["slug"] == "ghost" {
			t.Errorf("ghost must not reach the relay; got %s call: %v", c.Tool, c.Args)
		}
	}
	if got := len(callsFor(*calls, "register_agent")); got != 1 {
		t.Errorf("the live agent must still register, got %d register_agent calls", got)
	}
}

// A failed registration must surface with the agent's name — and must not
// stop the remaining agents from being registered.
func TestRegisterFleetSurfacesFailureAndContinues(t *testing.T) {
	stubExec(t)
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
