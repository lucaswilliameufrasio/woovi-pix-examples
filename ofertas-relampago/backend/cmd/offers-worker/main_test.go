package main

import "testing"

func TestRunRequiresLocalDemoMode(t *testing.T) {
	t.Setenv("DEMO_MODE", "false")
	if err := run(); err == nil {
		t.Fatal("worker started without explicit local demo mode")
	}
}
