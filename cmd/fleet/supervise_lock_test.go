package main

import "testing"

// The supervisor lock is exclusive: a second acquire while the first is held
// must fail, and re-acquire after release must succeed. flock(2) conflicts
// across distinct open file descriptions even within one process.
func TestAcquireSupervisorLockIsExclusive(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	f1, err := acquireSupervisorLock("demo")
	if err != nil {
		t.Fatalf("first acquire: %v", err)
	}
	if _, err := acquireSupervisorLock("demo"); err == nil {
		t.Error("second acquire on a held lock must fail")
	}
	f1.Close() // releases the flock

	f2, err := acquireSupervisorLock("demo")
	if err != nil {
		t.Fatalf("re-acquire after release: %v", err)
	}
	f2.Close()
}
