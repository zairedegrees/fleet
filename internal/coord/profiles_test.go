package coord

import "testing"

func TestRegisterProfileUpsert(t *testing.T) {
	s := New(newTestStore(t))

	r1 := mustCall(t, s, "register_profile", map[string]any{"slug": "dev", "name": "Dev", "role": "r1", "project": "p"})
	var p1 Profile // bare object, no wrapper
	decodePayload(t, r1, &p1)
	if p1.Slug != "dev" {
		t.Errorf("slug = %q, want dev", p1.Slug)
	}
	if p1.AllowedTools != "[]" {
		t.Errorf("allowed_tools default = %q, want []", p1.AllowedTools)
	}
	if p1.PoolSize != 3 {
		t.Errorf("pool_size default = %d, want 3", p1.PoolSize)
	}

	r2 := mustCall(t, s, "register_profile", map[string]any{"slug": "dev", "name": "Dev2", "role": "r2", "project": "p"})
	var p2 Profile
	decodePayload(t, r2, &p2)
	if p2.CreatedAt != p1.CreatedAt {
		t.Errorf("created_at not preserved on upsert: %q vs %q", p2.CreatedAt, p1.CreatedAt)
	}
	if p2.Role != "r2" || p2.Name != "Dev2" {
		t.Errorf("update not applied: %+v", p2)
	}

	var count int
	if err := s.store.reader().QueryRow(`SELECT COUNT(*) FROM profiles WHERE project='p' AND slug='dev'`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Errorf("profile row count = %d, want 1 (upsert not insert)", count)
	}
}

// TestRegisterProfilePoolSizeAndAllowedTools covers the option-gated fields:
// they persist when provided, and on a later upsert that omits them the existing
// values are preserved (matching wrai.th's opt gating).
func TestRegisterProfilePoolSizeAndAllowedTools(t *testing.T) {
	s := New(newTestStore(t))

	r1 := mustCall(t, s, "register_profile", map[string]any{
		"slug": "dev", "name": "Dev", "project": "p",
		"pool_size": 5, "allowed_tools": `["Read","Bash"]`,
	})
	var p1 Profile
	decodePayload(t, r1, &p1)
	if p1.PoolSize != 5 {
		t.Errorf("pool_size = %d, want 5", p1.PoolSize)
	}
	if p1.AllowedTools != `["Read","Bash"]` {
		t.Errorf("allowed_tools = %q", p1.AllowedTools)
	}

	// Upsert changing pool_size but omitting allowed_tools: pool_size updates,
	// allowed_tools is preserved (not reset to []).
	r2 := mustCall(t, s, "register_profile", map[string]any{
		"slug": "dev", "name": "Dev", "project": "p", "pool_size": 2,
	})
	var p2 Profile
	decodePayload(t, r2, &p2)
	if p2.PoolSize != 2 {
		t.Errorf("pool_size not updated: %d, want 2", p2.PoolSize)
	}
	if p2.AllowedTools != `["Read","Bash"]` {
		t.Errorf("allowed_tools not preserved on omit: %q", p2.AllowedTools)
	}
}
