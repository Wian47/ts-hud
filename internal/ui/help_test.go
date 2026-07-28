package ui

import (
	"strings"
	"testing"
)

func TestRenderHelpPanelListsAllSections(t *testing.T) {
	out := renderHelpPanel(80)

	sections := []string{
		"Peer table",
		"Search",
		"Ssh session",
		"Exit node picker",
		"DERP latency matrix",
		"Peer detail",
		"Preferences panel",
		"Connection-down confirm",
		"Account switch",
	}
	for _, s := range sections {
		if !strings.Contains(out, s) {
			t.Errorf("renderHelpPanel output missing section %q\noutput:\n%s", s, out)
		}
	}
}

func TestRenderHelpPanelListsKnownKeys(t *testing.T) {
	out := renderHelpPanel(80)

	keys := []string{"j/k", "g/G", "ctrl+q", "l ", "y          confirm bringing the connection down", "esc/a"}
	for _, k := range keys {
		if !strings.Contains(out, k) {
			t.Errorf("renderHelpPanel output missing key %q\noutput:\n%s", k, out)
		}
	}
}
