package ui

import (
	"errors"
	"net/netip"
	"strings"
	"testing"

	"tailscale.com/ipn"
	"tailscale.com/net/tsaddr"
)

func TestRenderPrefsPanelAllOff(t *testing.T) {
	prefs := &ipn.Prefs{}
	got := renderPrefsPanel(prefs, 0, false, nil, 60)
	for _, want := range []string{"SSH server", "Shields up", "Accept routes", "Accept DNS", "Advertise as exit node"} {
		if !strings.Contains(got, want) {
			t.Errorf("renderPrefsPanel() = %q, want to contain %q", got, want)
		}
	}
	if strings.Contains(got, "on") {
		t.Errorf("renderPrefsPanel() = %q, want no \"on\" state when everything is off", got)
	}
}

func TestRenderPrefsPanelAllOn(t *testing.T) {
	prefs := &ipn.Prefs{
		RunSSH:          true,
		ShieldsUp:       true,
		RouteAll:        true,
		CorpDNS:         true,
		AdvertiseRoutes: []netip.Prefix{tsaddr.AllIPv4(), tsaddr.AllIPv6()},
	}
	got := renderPrefsPanel(prefs, 0, false, nil, 60)
	if strings.Contains(got, "off") {
		t.Errorf("renderPrefsPanel() = %q, want no \"off\" state when everything is on", got)
	}
}

func TestRenderPrefsPanelHighlightsCursorRow(t *testing.T) {
	prefs := &ipn.Prefs{}
	got0 := renderPrefsPanel(prefs, 0, false, nil, 60)
	got4 := renderPrefsPanel(prefs, 4, false, nil, 60)
	if got0 == got4 {
		t.Error("renderPrefsPanel() output identical for cursor=0 and cursor=4, want the highlighted row to differ")
	}
}

func TestRenderPrefsPanelShowsLoadingState(t *testing.T) {
	got := renderPrefsPanel(nil, 0, true, nil, 60)
	if !strings.Contains(got, "loading") {
		t.Errorf("renderPrefsPanel() = %q, want a loading message", got)
	}
}

func TestRenderPrefsPanelShowsErrorBeforeFirstFetch(t *testing.T) {
	got := renderPrefsPanel(nil, 0, false, errors.New("get prefs failed"), 60)
	if !strings.Contains(got, "get prefs failed") {
		t.Errorf("renderPrefsPanel() = %q, want the error message", got)
	}
}

func TestRenderPrefsPanelShowsErrorAlongsideLoadedPrefs(t *testing.T) {
	prefs := &ipn.Prefs{RunSSH: true}
	got := renderPrefsPanel(prefs, 0, false, errors.New("edit failed"), 60)
	if !strings.Contains(got, "SSH server") || !strings.Contains(got, "edit failed") {
		t.Errorf("renderPrefsPanel() = %q, want both the row list and the error", got)
	}
}
