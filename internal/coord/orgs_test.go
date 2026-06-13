package coord

import (
	"net/http/httptest"
	"testing"

	"github.com/zairedegrees/fleet/internal/relay"
)

// TestListOrgsEmptyDbPassesHealth drives coord through fleet's OWN relay client
// — the exact consumer — and asserts the health probe passes against a freshly
// migrated, empty database. This is the first Tier-1 contract check: if coord's
// envelope shape drifts from what client.call expects, Health fails here.
func TestListOrgsEmptyDbPassesHealth(t *testing.T) {
	srv := httptest.NewServer(New(newTestStore(t)).Handler())
	defer srv.Close()

	client := relay.NewClient(srv.URL + "/mcp")
	if err := client.Health(); err != nil {
		t.Fatalf("Health against empty coord: %v", err)
	}
}
