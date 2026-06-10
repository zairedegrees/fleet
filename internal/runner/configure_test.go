package runner

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zairedegrees/fleet/internal/config"
)

// The auto `/relay talk` at boot is what defeats Axe 4 token-saving. It must be
// gated on the agent's AutoTalk knob, NOT on !IsExecutive (a field never set in
// production). Default agents (AutoTalk=false) stay idle until dispatch wakes them.
func TestBuildConfigureScriptGatesTalkOnAutoTalk(t *testing.T) {
	cfg := &config.FleetConfig{
		Project: config.ProjectConfig{Name: "proj", Cwd: t.TempDir()},
		Agents: []config.AgentConfig{
			{Name: "talker", Color: "green", Role: "Dev", AutoTalk: true},
			{Name: "idle", Color: "blue", Role: "Dev"}, // AutoTalk defaults to false
		},
	}

	script := buildConfigureScript(cfg, "/tmp/x.log")

	talkerLine := "tmux send-keys -t " + SessionName("proj", "talker") + " '/relay talk'"
	if !strings.Contains(script, talkerLine) {
		t.Errorf("AutoTalk=true agent should get auto /relay talk; script missing line:\n%s", talkerLine)
	}

	idleLine := "tmux send-keys -t " + SessionName("proj", "idle") + " '/relay talk'"
	if strings.Contains(script, idleLine) {
		t.Errorf("AutoTalk=false agent must NOT get auto /relay talk; script wrongly contains:\n%s", idleLine)
	}
}

// Profile + vault HTTP live in the typed relay.Client only. The ONE exception
// is the register_agent re-assert curl (see the re-assert tests below): the
// in-pane /relay register triggers a full-replace re-register that NULLs
// profile_slug, so the script must re-write it afterwards.
func TestBuildConfigureScriptKeepsProfileAndVaultOutOfScript(t *testing.T) {
	dir := t.TempDir()
	vaultShared := filepath.Join(dir, ".fleet", "vault", "shared")
	if err := os.MkdirAll(vaultShared, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(vaultShared, "doc.md"), []byte("# doc"), 0644); err != nil {
		t.Fatal(err)
	}
	cfg := &config.FleetConfig{
		Project: config.ProjectConfig{Name: "proj", Cwd: dir},
		Agents:  []config.AgentConfig{{Name: "dev", Color: "green", Role: "Dev"}},
	}

	script := buildConfigureScript(cfg, "/tmp/x.log")

	for _, banned := range []string{"register_profile", "set_memory", "ensure_profile"} {
		if strings.Contains(script, banned) {
			t.Errorf("profile/vault HTTP must stay in the typed client; found %q", banned)
		}
	}
	for _, kept := range []string{"wait_prompt", "/rename dev", "/relay register"} {
		if !strings.Contains(script, kept) {
			t.Errorf("pane-dependent work must stay in the script; missing %q", kept)
		}
	}
}

// The agent's in-pane /relay register makes its LLM call register_agent
// WITHOUT profile_slug, and the relay's re-register is a full-replace UPDATE
// that NULLs it — making dispatched tasks invisible. The script must re-assert
// the full registration via curl AFTER the in-pane register, carrying
// profile_slug, reports_to and is_executive (last write wins, as on main).
func TestBuildConfigureScriptReassertsRegistrationViaCurl(t *testing.T) {
	cfg := &config.FleetConfig{
		Project: config.ProjectConfig{Name: "proj", RelayURL: "http://relay.test:9999/mcp", Cwd: t.TempDir()},
		Agents: []config.AgentConfig{
			{Name: "dev", Color: "green", Role: "Développeur — auth & paiements", ReportsTo: "lead"},
			{Name: "lead", Color: "red", Role: "Tech Lead", IsExecutive: true},
		},
	}

	script := buildConfigureScript(cfg, "/tmp/x.log")

	if !strings.Contains(script, `RELAY_URL="http://relay.test:9999/mcp"`) {
		t.Error("script must pin the config relay URL for the re-assert curl")
	}
	for _, want := range []string{
		`"name":"dev"`,
		`"project":"proj"`,
		`"role":"Développeur — auth & paiements"`,
		`"profile_slug":"dev"`,
		`"reports_to":"lead"`,
		`"is_executive":false`,
		`"profile_slug":"lead"`,
		`"is_executive":true`,
	} {
		if !strings.Contains(script, want) {
			t.Errorf("re-assert curl must carry %s; script:\n%s", want, script)
		}
	}
}

// ORDER is the whole point of the re-assert: for each agent, the in-pane
// /relay register must come FIRST and the curl LAST (inside the wait_prompt
// gate) so the curl's full payload is the final write on the relay.
func TestBuildConfigureScriptCurlAfterInPaneRegisterInsideGate(t *testing.T) {
	cfg := &config.FleetConfig{
		Project: config.ProjectConfig{Name: "proj", Cwd: t.TempDir()},
		Agents: []config.AgentConfig{
			{Name: "dev", Color: "green", Role: "Dev"},
			{Name: "ops", Color: "blue", Role: "Ops"},
		},
	}

	script := buildConfigureScript(cfg, "/tmp/x.log")

	blocks := strings.Split(script, "# Configure ")
	if len(blocks) != 3 {
		t.Fatalf("expected one block per agent, got %d:\n%s", len(blocks)-1, script)
	}
	for _, block := range blocks[1:] {
		name := strings.TrimSpace(strings.SplitN(block, "\n", 2)[0])
		gateIdx := strings.Index(block, "if wait_prompt")
		registerIdx := strings.Index(block, "/relay register")
		curlIdx := strings.Index(block, "curl -s -X POST")
		elseIdx := strings.Index(block, "\nelse\n")
		if gateIdx == -1 || registerIdx == -1 || curlIdx == -1 || elseIdx == -1 {
			t.Fatalf("agent %s: block missing gate/register/curl/else:\n%s", name, block)
		}
		if !(gateIdx < registerIdx && registerIdx < curlIdx && curlIdx < elseIdx) {
			t.Errorf("agent %s: want wait_prompt < /relay register < curl < else, got %d/%d/%d/%d",
				name, gateIdx, registerIdx, curlIdx, elseIdx)
		}
	}
}
