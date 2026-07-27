package ui

import (
	"errors"
	"strings"
	"testing"

	"tailscale.com/ipn"
)

func TestRenderAccountsPanelEmpty(t *testing.T) {
	got := renderAccountsPanel(ipn.LoginProfile{}, nil, 0, false, nil, 60)
	if !strings.Contains(got, "no accounts") {
		t.Errorf("renderAccountsPanel() = %q, want the empty-state message", got)
	}
}

func TestRenderAccountsPanelSingleProfile(t *testing.T) {
	p := ipn.LoginProfile{ID: "1ab3", Name: "alice@example.com"}
	got := renderAccountsPanel(p, []ipn.LoginProfile{p}, 0, false, nil, 60)
	if !strings.Contains(got, "alice@example.com") {
		t.Errorf("renderAccountsPanel() = %q, want it to contain the profile name", got)
	}
	if !strings.Contains(got, "(current)") {
		t.Errorf("renderAccountsPanel() = %q, want the current profile marked", got)
	}
}

func TestRenderAccountsPanelMultipleProfilesMarksCurrent(t *testing.T) {
	current := ipn.LoginProfile{ID: "1ab3", Name: "alice@example.com"}
	other := ipn.LoginProfile{ID: "9f2c", Name: "bob@example.com"}
	got := renderAccountsPanel(current, []ipn.LoginProfile{current, other}, 0, false, nil, 60)

	if !strings.Contains(got, "alice@example.com") || !strings.Contains(got, "bob@example.com") {
		t.Errorf("renderAccountsPanel() = %q, want both profile names", got)
	}
	aliceLine := lineContaining(got, "alice@example.com")
	bobLine := lineContaining(got, "bob@example.com")
	if !strings.Contains(aliceLine, "(current)") {
		t.Errorf("alice line = %q, want it marked (current)", aliceLine)
	}
	if strings.Contains(bobLine, "(current)") {
		t.Errorf("bob line = %q, want it NOT marked (current)", bobLine)
	}
}

func TestRenderAccountsPanelFallsBackToDisplayName(t *testing.T) {
	p := ipn.LoginProfile{ID: "1ab3", NetworkProfile: ipn.NetworkProfile{DisplayName: "Acme Tailnet"}}
	got := renderAccountsPanel(p, []ipn.LoginProfile{p}, 0, false, nil, 60)
	if !strings.Contains(got, "Acme Tailnet") {
		t.Errorf("renderAccountsPanel() = %q, want the NetworkProfile display name when Name is empty", got)
	}
}

func TestRenderAccountsPanelHighlightsCursorRow(t *testing.T) {
	all := []ipn.LoginProfile{{ID: "1ab3", Name: "alice"}, {ID: "9f2c", Name: "bob"}}
	got0 := renderAccountsPanel(all[0], all, 0, false, nil, 60)
	got1 := renderAccountsPanel(all[0], all, 1, false, nil, 60)
	if got0 == got1 {
		t.Error("renderAccountsPanel() output identical for cursor=0 and cursor=1, want the highlighted row to differ")
	}
}

func TestRenderAccountsPanelShowsLoadingState(t *testing.T) {
	got := renderAccountsPanel(ipn.LoginProfile{}, nil, 0, true, nil, 60)
	if !strings.Contains(got, "loading") {
		t.Errorf("renderAccountsPanel() = %q, want a loading message", got)
	}
}

func TestRenderAccountsPanelShowsErrorBeforeFirstFetch(t *testing.T) {
	got := renderAccountsPanel(ipn.LoginProfile{}, nil, 0, false, errors.New("list profiles failed"), 60)
	if !strings.Contains(got, "list profiles failed") {
		t.Errorf("renderAccountsPanel() = %q, want the error message", got)
	}
	if strings.Contains(got, "no accounts") {
		t.Errorf("renderAccountsPanel() = %q, want the error to replace the empty-state message, not both", got)
	}
}

func TestRenderAccountsPanelShowsErrorAlongsideLoadedList(t *testing.T) {
	p := ipn.LoginProfile{ID: "1ab3", Name: "alice@example.com"}
	got := renderAccountsPanel(p, []ipn.LoginProfile{p}, 0, false, errors.New("switch failed"), 60)
	if !strings.Contains(got, "alice@example.com") || !strings.Contains(got, "switch failed") {
		t.Errorf("renderAccountsPanel() = %q, want both the row list and the error", got)
	}
}

func lineContaining(s, substr string) string {
	for _, line := range strings.Split(s, "\n") {
		if strings.Contains(line, substr) {
			return line
		}
	}
	return ""
}
