package wizard

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/zairedegrees/fleet/internal/relay"
)

// A relay failure while loading agents must be surfaced, not swallowed into an
// empty list with zero feedback.
func TestWizardCapturesRelayError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"jsonrpc":"2.0","id":1,"error":{"code":-32000,"message":"relay down"}}`)
	}))
	defer srv.Close()

	m := newWizardModel(relay.NewClient(srv.URL))
	updated, _ := m.Update(ProjectSelectedMsg{Name: "proj", Path: "/tmp"})
	wm := updated.(wizardModel)
	if !strings.Contains(wm.status, "relay") {
		t.Errorf("expected the relay error captured in status, got: %q", wm.status)
	}
}

// The captured status must actually render so the user sees it.
func TestWizardViewShowsStatus(t *testing.T) {
	m := newWizardModel(nil)
	m.status = "relay unreachable"
	if !strings.Contains(m.View(), "relay unreachable") {
		t.Errorf("View should surface the status message; got:\n%s", m.View())
	}
}
