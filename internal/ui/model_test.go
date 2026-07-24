package ui

import (
	"net/netip"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/Wian47/ts-hud/internal/tsnet"
)

func testPeers() []tsnet.Peer {
	return []tsnet.Peer{
		{HostName: "bravo", OS: "linux", Online: true, ConnType: tsnet.ConnDirect, IPs: []netip.Addr{netip.MustParseAddr("100.64.0.2")}},
		{HostName: "alpha", OS: "linux", Online: true, ConnType: tsnet.ConnDERP, IPs: []netip.Addr{netip.MustParseAddr("100.64.0.1")}},
		{HostName: "charlie", OS: "windows", Online: false, ConnType: tsnet.ConnUnknown, IPs: []netip.Addr{netip.MustParseAddr("100.64.0.3")}},
	}
}

func newTestModel() Model {
	m := NewModel(tsnet.NewFetcher(), 0)
	m.peers = testPeers()
	m.applyFilter()
	return m
}

func TestUpdateQuit(t *testing.T) {
	m := newTestModel()
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")})
	if cmd == nil {
		t.Fatal("Update(q) returned nil cmd, want tea.Quit")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Fatalf("Update(q) cmd produced %T, want tea.QuitMsg", cmd())
	}
}

func TestViewRendersPeerTable(t *testing.T) {
	m := newTestModel()
	view := m.View()
	for _, want := range []string{"alpha", "bravo", "charlie", "HOSTNAME"} {
		if !contains(view, want) {
			t.Errorf("View() missing %q\n---\n%s", want, view)
		}
	}
}

func TestNavigationBounds(t *testing.T) {
	m := newTestModel()

	if m.cursor != 0 {
		t.Fatalf("initial cursor = %d, want 0", m.cursor)
	}

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("k")})
	m = updated.(Model)
	if m.cursor != 0 {
		t.Fatalf("cursor after k at top = %d, want 0 (clamped)", m.cursor)
	}

	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("G")})
	m = updated.(Model)
	if want := len(m.filtered) - 1; m.cursor != want {
		t.Fatalf("cursor after G = %d, want %d", m.cursor, want)
	}

	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	m = updated.(Model)
	if want := len(m.filtered) - 1; m.cursor != want {
		t.Fatalf("cursor after j at bottom = %d, want %d (clamped)", m.cursor, want)
	}

	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("g")})
	m = updated.(Model)
	if m.cursor != 0 {
		t.Fatalf("cursor after g = %d, want 0", m.cursor)
	}
}

func contains(haystack, needle string) bool {
	return len(haystack) >= len(needle) && (func() bool {
		for i := 0; i+len(needle) <= len(haystack); i++ {
			if haystack[i:i+len(needle)] == needle {
				return true
			}
		}
		return false
	})()
}
