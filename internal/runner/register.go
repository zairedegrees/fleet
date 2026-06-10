package runner

import (
	"errors"
	"fmt"
	"path/filepath"
	"time"

	"github.com/zairedegrees/fleet/internal/config"
	"github.com/zairedegrees/fleet/internal/relay"
)

// registerTimeout caps each synchronous registration call so a hanging relay
// can't stall the launch (the status client uses the same idea at 2s).
const registerTimeout = 3 * time.Second

// relayRegistrar is the slice of relay.Client the launch registration needs —
// a seam so registerFleet is testable against httptest or a fake.
type relayRegistrar interface {
	EnsureProfile(name, role, project string) error
	RegisterAgentFull(r relay.AgentRegistration) error
	PushVaultDoc(project, path string, content []byte) error
}

// registerFleet does the relay HTTP work that used to live as curl strings in
// the generated configure script: profile + agent registration (profile_slug
// is what routes dispatched tasks) and vault doc injection. None of it depends
// on pane readiness, so it runs synchronously before fleet exits. One agent's
// failure doesn't stop the others; every failure is named in the joined error.
func registerFleet(cfg *config.FleetConfig, rc relayRegistrar) error {
	var errs []error
	project := cfg.Project.Name
	vaultDir := filepath.Join(cfg.Project.Cwd, ".fleet", "vault")

	for _, agent := range cfg.Agents {
		// A partially-failed launch must not leave ghosts on the relay:
		// only register agents whose tmux session actually exists.
		if !HasSession(project, agent.Name) {
			errs = append(errs, fmt.Errorf("skip register %s: no tmux session", agent.Name))
			continue
		}
		if err := rc.EnsureProfile(agent.Name, agent.Role, project); err != nil {
			errs = append(errs, fmt.Errorf("profile %s: %w", agent.Name, err))
		}
		if err := rc.RegisterAgentFull(relay.AgentRegistration{
			Name: agent.Name, Project: project, Role: agent.Role,
			ProfileSlug: agent.Name, ReportsTo: agent.ReportsTo, IsExecutive: agent.IsExecutive,
		}); err != nil {
			errs = append(errs, fmt.Errorf("register %s: %w", agent.Name, err))
		}

		docs, err := config.ResolveVaultDocs(vaultDir, agent)
		if err != nil {
			errs = append(errs, fmt.Errorf("vault for %s: %w", agent.Name, err))
			continue
		}
		if len(docs) == 0 {
			continue
		}
		if total := config.VaultSize(docs); total > int64(config.VaultSizeWarningBytes) {
			fmt.Printf("  ⚠ vault for %s is %dKB (>50KB)\n", agent.Name, total/1024)
		}
		pushed := 0
		for _, doc := range docs {
			if err := rc.PushVaultDoc(project, doc.Path, doc.Content); err != nil {
				errs = append(errs, fmt.Errorf("vault doc %s for %s: %w", doc.Path, agent.Name, err))
				continue
			}
			pushed++
		}
		fmt.Printf("  ✓ vault injected for %s: %d docs\n", agent.Name, pushed)
	}

	return errors.Join(errs...)
}
