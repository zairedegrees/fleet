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
