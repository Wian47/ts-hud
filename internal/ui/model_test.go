package ui

import (
	"errors"
	"net/netip"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/vt"

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

func TestRenderHeaderUsesSelfDisplayName(t *testing.T) {
	m := newTestModel()
	self := tsnet.Peer{
		HostName: "fedora",
		DNSName:  "acer-swift.tail865ddd.ts.net.",
		IPs:      []netip.Addr{netip.MustParseAddr("100.64.0.9")},
	}
	m.self = &self

	header := m.renderHeader()
	if !contains(header, "acer-swift") {
		t.Errorf("renderHeader() = %q, want it to contain self display name %q", header, "acer-swift")
	}
	if contains(header, "fedora") {
		t.Errorf("renderHeader() = %q, should not show raw OS hostname %q", header, "fedora")
	}
}

func exitNodeTestPeers() []tsnet.Peer {
	return []tsnet.Peer{
		{HostName: "alpha", DNSName: "alpha.tailnet-1234.ts.net.", Online: true, CanBeExitNode: true, ID: "node-alpha"},
		{HostName: "bravo", DNSName: "bravo.tailnet-1234.ts.net.", Online: true, CanBeExitNode: true, IsExitNode: true, ID: "node-bravo"},
		{HostName: "charlie", Online: true},
	}
}

func TestEnterExitNodePickerFiltersToEligiblePeersAndSelectsActive(t *testing.T) {
	m := newTestModel()
	m.peers = exitNodeTestPeers()
	m.applyFilter()

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("x")})
	m = updated.(Model)

	if !m.pickingExitNode {
		t.Fatal("pickingExitNode = false after 'x', want true")
	}
	if candidates := m.exitNodeCandidates(); len(candidates) != 2 {
		t.Fatalf("exitNodeCandidates() len = %d, want 2 (charlie excluded)", len(candidates))
	}
	// bravo (IsExitNode=true) is candidates[1], so cursor should default to 2.
	if m.exitNodeCursor != 2 {
		t.Fatalf("exitNodeCursor = %d, want 2 (pointing at active exit node)", m.exitNodeCursor)
	}
}

func TestExitNodePickerEscCancelsWithoutChanging(t *testing.T) {
	m := newTestModel()
	m.peers = exitNodeTestPeers()
	m.applyFilter()

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("x")})
	m = updated.(Model)

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = updated.(Model)
	if m.pickingExitNode {
		t.Fatal("pickingExitNode = true after esc, want false")
	}
	if cmd != nil {
		t.Fatal("Update(esc) in exit node picker returned non-nil cmd, want nil")
	}
}

func TestExitNodePickerEnterSelectsAndReturnsCmd(t *testing.T) {
	m := newTestModel()
	m.peers = exitNodeTestPeers()
	m.applyFilter()

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("x")})
	m = updated.(Model)

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(Model)
	if m.pickingExitNode {
		t.Fatal("pickingExitNode = true after enter, want false")
	}
	if cmd == nil {
		t.Fatal("Update(enter) in exit node picker returned nil cmd, want non-nil")
	}
}

func TestExitNodePickerNoneEntryClearsExitNode(t *testing.T) {
	m := newTestModel()
	m.peers = exitNodeTestPeers()
	m.applyFilter()

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("x")})
	m = updated.(Model)
	m.exitNodeCursor = 0 // the synthetic "none" entry

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(Model)
	if m.pickingExitNode {
		t.Fatal("pickingExitNode = true after selecting none, want false")
	}
	if cmd == nil {
		t.Fatal("Update(enter) on none entry returned nil cmd, want non-nil")
	}
}

func TestExitNodePickerTogglesAllowLANAccess(t *testing.T) {
	m := newTestModel()
	m.peers = exitNodeTestPeers()
	m.applyFilter()

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("x")})
	m = updated.(Model)

	if m.allowLANAccess {
		t.Fatal("allowLANAccess = true initially, want false")
	}
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("l")})
	m = updated.(Model)
	if !m.allowLANAccess {
		t.Fatal("allowLANAccess = false after 'l', want true")
	}
}

func TestViewRendersExitNodePicker(t *testing.T) {
	m := newTestModel()
	m.peers = exitNodeTestPeers()
	m.applyFilter()

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("x")})
	m = updated.(Model)

	view := m.View()
	for _, want := range []string{"alpha", "bravo", "Select exit node"} {
		if !contains(view, want) {
			t.Errorf("View() missing %q\n---\n%s", want, view)
		}
	}
	if contains(view, "charlie") {
		t.Errorf("View() should not list charlie (not exit-node eligible)\n---\n%s", view)
	}
}

func TestEnterDERPViewStartsLoadingAndReturnsCmd(t *testing.T) {
	m := newTestModel()

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("d")})
	m = updated.(Model)

	if !m.viewingDERP {
		t.Fatal("viewingDERP = false after 'd', want true")
	}
	if !m.derpLoading {
		t.Fatal("derpLoading = false after 'd', want true")
	}
	if cmd == nil {
		t.Fatal("Update('d') returned nil cmd, want a netcheck command")
	}
}

func TestDERPViewEscReturnsToPeerTable(t *testing.T) {
	m := newTestModel()
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("d")})
	m = updated.(Model)

	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = updated.(Model)

	if m.viewingDERP {
		t.Fatal("viewingDERP = true after esc, want false")
	}
	view := m.View()
	if !contains(view, "HOSTNAME") {
		t.Errorf("View() after leaving DERP view missing peer table\n---\n%s", view)
	}
}

func TestDERPReportMsgPopulatesRegionsAndClearsLoading(t *testing.T) {
	m := newTestModel()
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("d")})
	m = updated.(Model)

	regions := []tsnet.DERPRegion{
		{Code: "fra", Name: "Frankfurt", Latency: 20 * time.Millisecond, Available: true, Preferred: true},
		{Code: "syd", Name: "Sydney", Available: false},
	}
	updated, _ = m.Update(derpReportMsg{result: tsnet.NetCheckResult{Regions: regions}})
	m = updated.(Model)

	if m.derpLoading {
		t.Fatal("derpLoading = true after derpReportMsg, want false")
	}

	view := m.View()
	for _, want := range []string{"fra", "Frankfurt", "preferred", "syd", "Sydney"} {
		if !contains(view, want) {
			t.Errorf("View() missing %q\n---\n%s", want, view)
		}
	}
}

func TestDERPViewShowsLoadingState(t *testing.T) {
	m := newTestModel()
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("d")})
	m = updated.(Model)

	view := m.View()
	if !contains(view, "checking DERP latency") {
		t.Errorf("View() missing loading message\n---\n%s", view)
	}
}

func TestDERPViewRefreshRestartsLoading(t *testing.T) {
	m := newTestModel()
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("d")})
	m = updated.(Model)
	updated, _ = m.Update(derpReportMsg{result: tsnet.NetCheckResult{Regions: []tsnet.DERPRegion{{Code: "fra"}}}})
	m = updated.(Model)

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("r")})
	m = updated.(Model)

	if !m.derpLoading {
		t.Fatal("derpLoading = false after 'r' in DERP view, want true")
	}
	if cmd == nil {
		t.Fatal("Update('r') in DERP view returned nil cmd, want a netcheck command")
	}
}

func TestViewIsFramedWithRoundedBorder(t *testing.T) {
	m := newTestModel()
	view := m.View()
	for _, want := range []string{"╭", "╮", "╰", "╯", "├", "┤"} {
		if !contains(view, want) {
			t.Errorf("View() missing border rune %q\n---\n%s", want, view)
		}
	}
}

func TestViewFillsWindowDimensions(t *testing.T) {
	m := newTestModel()
	m.width = 60
	m.height = 20

	view := m.View()
	lines := strings.Split(view, "\n")
	if len(lines) != m.height {
		t.Fatalf("View() produced %d lines, want %d (m.height)", len(lines), m.height)
	}
	for i, line := range lines {
		if w := lipgloss.Width(line); w != m.width {
			t.Fatalf("line %d width = %d, want %d (m.width)\nline: %q", i, w, m.width, line)
		}
	}
	if !strings.HasPrefix(lines[len(lines)-1], "╰") {
		t.Errorf("last line = %q, want it to start with the bottom border corner", lines[len(lines)-1])
	}
}

func TestViewHandlesTinyDimensionsWithoutPanicking(t *testing.T) {
	m := newTestModel()
	m.width = 1
	m.height = 1
	_ = m.View() // must not panic
}

func TestViewRendersSSHPaneConnectingState(t *testing.T) {
	m := newTestModel()
	m.viewingSSH = true

	view := m.View()
	if !contains(view, "connecting") {
		t.Errorf("View() missing connecting indicator\n---\n%s", view)
	}
}

func TestViewRendersSSHPaneOutput(t *testing.T) {
	m := newTestModel()
	m.viewingSSH = true
	m.sshPane = &sshPane{term: vt.NewSafeEmulator(80, 24), output: make(chan []byte), done: make(chan struct{})}
	m.sshPane.term.Write([]byte("remote-shell-prompt$"))

	view := m.View()
	if !contains(view, "remote-shell-prompt$") {
		t.Errorf("View() missing pty output\n---\n%s", view)
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
