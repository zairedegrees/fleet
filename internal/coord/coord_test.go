package coord

import "testing"

func TestNewReturnsServer(t *testing.T) {
	st := newTestStore(t)
	if New(st) == nil {
		t.Fatal("New returned nil")
	}
}
