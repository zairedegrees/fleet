package relaymgr

import "testing"

func TestReachableUsesHealthProbe(t *testing.T) {
	called := false
	probe = func(url string) error { called = true; return nil } // seam
	defer func() { probe = defaultProbe }()

	if !Reachable("http://localhost:8090/mcp") {
		t.Error("Reachable should be true when probe returns nil")
	}
	if !called {
		t.Error("Reachable must use the probe seam")
	}
}

func TestBinPathUnderFleetDir(t *testing.T) {
	p := BinPath()
	if filepathBase(p) != "agent-relay" {
		t.Errorf("BinPath should end in agent-relay, got %s", p)
	}
}
