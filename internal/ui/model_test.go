package ui

import (
	"errors"
	"net/netip"
	"testing"
	"time"

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

func TestFilterNarrowsListAndClampsCursor(t *testing.T) {
	m := newTestModel()

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("G")})
	m = updated.(Model)
	if m.cursor != 2 {
		t.Fatalf("cursor after G = %d, want 2", m.cursor)
	}

	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/")})
	m = updated.(Model)
	if !m.searching {
		t.Fatal("searching = false after '/', want true")
	}

	for _, r := range "windows" {
		updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		m = updated.(Model)
	}

	if len(m.filtered) != 1 || m.filtered[0].HostName != "charlie" {
		t.Fatalf("filtered = %+v, want only charlie", m.filtered)
	}
	if m.cursor != 0 {
		t.Fatalf("cursor after filter shrank list = %d, want 0", m.cursor)
	}

	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = updated.(Model)
	if m.searching {
		t.Fatal("searching = true after esc, want false")
	}
	if len(m.filtered) != len(m.peers) {
		t.Fatalf("filtered len = %d after esc, want %d (query cleared)", len(m.filtered), len(m.peers))
	}
}

func TestEnterOnOfflinePeerIsNoop(t *testing.T) {
	m := newTestModel()
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("G")})
	m = updated.(Model)

	if peer, ok := m.selectedPeer(); !ok || peer.HostName != "charlie" || peer.Online {
		t.Fatalf("selectedPeer() = %+v, %v, want offline charlie", peer, ok)
	}

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd != nil {
		t.Fatal("Update(enter) on offline peer returned non-nil cmd, want nil")
	}
}

func TestBuildSSHCommandPrefersDNSName(t *testing.T) {
	peer := tsnet.Peer{
		HostName: "bravo",
		DNSName:  "bravo.tailnet-1234.ts.net.",
		IPs:      []netip.Addr{netip.MustParseAddr("100.64.0.2")},
	}
	c := buildSSHCommand(peer)
	if len(c.Args) != 2 || c.Args[0] != "ssh" || c.Args[1] != "bravo.tailnet-1234.ts.net" {
		t.Fatalf("buildSSHCommand args = %v, want [ssh bravo.tailnet-1234.ts.net]", c.Args)
	}
}

func TestViewRendersErrorWithoutPanicking(t *testing.T) {
	m := NewModel(tsnet.NewFetcher(), time.Second)
	updated, _ := m.Update(errMsg{err: errors.New("tailscaled: not running")})
	m = updated.(Model)

	view := m.View()
	if !contains(view, "error: tailscaled: not running") {
		t.Fatalf("View() missing error text\n---\n%s", view)
	}
	if !contains(view, "no peers match") {
		t.Fatalf("View() missing empty-table message\n---\n%s", view)
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
