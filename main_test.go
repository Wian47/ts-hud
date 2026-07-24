package main

import "testing"

func TestResolveVersionPrefersLdflagValue(t *testing.T) {
	old := version
	defer func() { version = old }()

	version = "v1.2.3"
	if got := resolveVersion(); got != "v1.2.3" {
		t.Errorf("resolveVersion() = %q, want %q (ldflag-injected version should win)", got, "v1.2.3")
	}
}
