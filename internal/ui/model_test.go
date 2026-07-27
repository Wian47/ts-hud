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
	"tailscale.com/ipn"

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

func TestPeerDetailOpensForSelectedPeerIncludingOffline(t *testing.T) {
	m := newTestModel()
	m.cursor = 2 // charlie, offline

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("i")})
	m = updated.(Model)

	if !m.viewingPeerDetail {
		t.Fatal("viewingPeerDetail = false after 'i' on an offline peer, want true")
	}
	if m.peerDetailTarget.HostName != "charlie" {
		t.Errorf("peerDetailTarget.HostName = %q, want %q", m.peerDetailTarget.HostName, "charlie")
	}
	if !m.peerDetailLoading {
		t.Fatal("peerDetailLoading = false immediately after 'i', want true")
	}
	if cmd == nil {
		t.Fatal("Update('i') returned nil cmd, want a peer-detail probe command")
	}
}

func TestPeerDetailEscCloses(t *testing.T) {
	m := newTestModel()
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("i")})
	m = updated.(Model)

	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = updated.(Model)

	if m.viewingPeerDetail {
		t.Fatal("viewingPeerDetail = true after esc, want false")
	}
}

func TestPeerDetailReportMsgClearsLoading(t *testing.T) {
	m := newTestModel()
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("i")})
	m = updated.(Model)

	result := tsnet.PeerDetail{Owner: "Alice Smith", Tags: []string{"tag:server"}}
	updated, _ = m.Update(peerDetailReportMsg{result: result})
	m = updated.(Model)

	if m.peerDetailLoading {
		t.Fatal("peerDetailLoading = true after peerDetailReportMsg, want false")
	}
	view := m.View()
	if !contains(view, "Alice Smith") {
		t.Errorf("View() missing owner\n---\n%s", view)
	}
}

func TestPeerDetailTargetSurvivesBackgroundPeerRefresh(t *testing.T) {
	m := newTestModel()
	m.cursor = 0 // bravo
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("i")})
	m = updated.(Model)

	// Simulate the background auto-refresh reordering the peer list while
	// the detail view is open.
	updated, _ = m.Update(peersMsg{peers: []tsnet.Peer{
		{HostName: "alpha", OS: "linux", Online: true, IPs: []netip.Addr{netip.MustParseAddr("100.64.0.1")}},
		{HostName: "bravo", OS: "linux", Online: false, IPs: []netip.Addr{netip.MustParseAddr("100.64.0.2")}},
	}})
	m = updated.(Model)

	if m.peerDetailTarget.HostName != "bravo" {
		t.Errorf("peerDetailTarget.HostName = %q after background refresh, want %q (should stay pinned to the peer the view was opened for)", m.peerDetailTarget.HostName, "bravo")
	}
}

func TestPeerDetailRefreshRetriggersProbe(t *testing.T) {
	m := newTestModel()
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("i")})
	m = updated.(Model)
	updated, _ = m.Update(peerDetailReportMsg{result: tsnet.PeerDetail{Owner: "Alice"}})
	m = updated.(Model)

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("r")})
	m = updated.(Model)

	if !m.peerDetailLoading {
		t.Fatal("peerDetailLoading = false after 'r' in peer detail view, want true")
	}
	if cmd == nil {
		t.Fatal("Update('r') in peer detail view returned nil cmd, want a probe command")
	}
}

func TestPrefsPanelOpensAndFetches(t *testing.T) {
	m := newTestModel()

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("p")})
	m = updated.(Model)

	if !m.viewingPrefs {
		t.Fatal("viewingPrefs = false after 'p', want true")
	}
	if !m.prefsLoading {
		t.Fatal("prefsLoading = false immediately after 'p', want true")
	}
	if cmd == nil {
		t.Fatal("Update('p') returned nil cmd, want a prefs-fetch command")
	}
}

func TestPrefsPanelEscCloses(t *testing.T) {
	m := newTestModel()
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("p")})
	m = updated.(Model)

	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = updated.(Model)

	if m.viewingPrefs {
		t.Fatal("viewingPrefs = true after esc, want false")
	}
}

func TestPrefsCursorMovesAndClamps(t *testing.T) {
	m := newTestModel()
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("p")})
	m = updated.(Model)

	for i := 0; i < 10; i++ {
		updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
		m = updated.(Model)
	}
	if m.prefsCursor != numPrefRows-1 {
		t.Errorf("prefsCursor = %d after 10x 'j', want clamped to %d", m.prefsCursor, numPrefRows-1)
	}

	for i := 0; i < 10; i++ {
		updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("k")})
		m = updated.(Model)
	}
	if m.prefsCursor != 0 {
		t.Errorf("prefsCursor = %d after 10x 'k', want clamped to 0", m.prefsCursor)
	}
}

func TestPrefsMsgPopulatesPrefsAndClearsLoading(t *testing.T) {
	m := newTestModel()
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("p")})
	m = updated.(Model)

	updated, _ = m.Update(prefsMsg{prefs: &ipn.Prefs{RunSSH: true}})
	m = updated.(Model)

	if m.prefsLoading {
		t.Fatal("prefsLoading = true after prefsMsg, want false")
	}
	view := m.View()
	if !contains(view, "SSH server") {
		t.Errorf("View() missing preferences panel\n---\n%s", view)
	}
}

func TestPrefsEnterWithNilPrefsIsNoop(t *testing.T) {
	m := newTestModel()
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("p")})
	m = updated.(Model)

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(Model)

	if cmd != nil {
		t.Error("Update(enter) with prefs still nil returned a non-nil cmd, want nil")
	}
	if !m.prefsLoading {
		t.Error("prefsLoading = false after a no-op enter, want it to remain true (fetch still in flight)")
	}
}

func TestPrefsEnterTogglesAndReturnsCmd(t *testing.T) {
	m := newTestModel()
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("p")})
	m = updated.(Model)
	updated, _ = m.Update(prefsMsg{prefs: &ipn.Prefs{RunSSH: false}})
	m = updated.(Model)

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(Model)

	if !m.prefsLoading {
		t.Fatal("prefsLoading = false after enter on a loaded panel, want true")
	}
	if cmd == nil {
		t.Fatal("Update(enter) in prefs panel returned nil cmd, want a toggle command")
	}
}

func TestPrefsMsgErrorKeepsExistingPrefs(t *testing.T) {
	m := newTestModel()
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("p")})
	m = updated.(Model)
	updated, _ = m.Update(prefsMsg{prefs: &ipn.Prefs{RunSSH: true}})
	m = updated.(Model)

	updated, _ = m.Update(prefsMsg{err: errors.New("boom")})
	m = updated.(Model)

	if m.prefs == nil || !m.prefs.RunSSH {
		t.Fatalf("prefs = %+v after a failed toggle, want the last-known-good RunSSH=true prefs preserved", m.prefs)
	}
	if m.prefsErr == nil {
		t.Fatal("prefsErr = nil after prefsMsg with an error, want it set")
	}
}

func TestRenderHeaderOmitsSuffixWhenRunning(t *testing.T) {
	m := newTestModel()
	self := tsnet.Peer{HostName: "laptop", IPs: []netip.Addr{netip.MustParseAddr("100.64.0.9")}}
	m.self = &self
	m.backendState = "Running"

	header := m.renderHeader()
	if contains(header, "RUNNING") {
		t.Errorf("renderHeader() = %q, want no state suffix when Running", header)
	}
}

func TestRenderHeaderOmitsSuffixWhenUnset(t *testing.T) {
	m := newTestModel()
	self := tsnet.Peer{HostName: "laptop", IPs: []netip.Addr{netip.MustParseAddr("100.64.0.9")}}
	m.self = &self

	header := m.renderHeader()
	if contains(header, "[") {
		t.Errorf("renderHeader() = %q, want no state suffix before the first fetch populates backendState", header)
	}
}

func TestRenderHeaderShowsSuffixWhenStopped(t *testing.T) {
	m := newTestModel()
	self := tsnet.Peer{HostName: "laptop", IPs: []netip.Addr{netip.MustParseAddr("100.64.0.9")}}
	m.self = &self
	m.backendState = "Stopped"

	header := m.renderHeader()
	if !contains(header, "[STOPPED]") {
		t.Errorf("renderHeader() = %q, want it to contain %q", header, "[STOPPED]")
	}
}

func TestPeersMsgPopulatesBackendState(t *testing.T) {
	m := newTestModel()
	updated, _ := m.Update(peersMsg{peers: testPeers(), backendState: "Stopped"})
	m = updated.(Model)

	if m.backendState != "Stopped" {
		t.Errorf("backendState = %q after peersMsg, want %q", m.backendState, "Stopped")
	}
}

func TestConnToggleRunningAsksConfirmation(t *testing.T) {
	m := newTestModel()
	m.backendState = "Running"

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("c")})
	m = updated.(Model)

	if !m.confirmingDown {
		t.Fatal("confirmingDown = false after 'c' while Running, want true")
	}
	if cmd != nil {
		t.Error("Update('c') while Running returned a non-nil cmd, want nil until confirmed")
	}
	view := m.View()
	if !contains(view, "Bring Tailscale down?") {
		t.Errorf("View() = %q, want the confirmation prompt", view)
	}
}

func TestConnConfirmYesToggles(t *testing.T) {
	m := newTestModel()
	m.backendState = "Running"
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("c")})
	m = updated.(Model)

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("y")})
	m = updated.(Model)

	if m.confirmingDown {
		t.Error("confirmingDown = true after 'y', want false")
	}
	if !m.connLoading {
		t.Error("connLoading = false after 'y', want true")
	}
	if cmd == nil {
		t.Fatal("Update('y') while confirming returned nil cmd, want a SetWantRunning command")
	}
}

func TestConnConfirmNoCancels(t *testing.T) {
	m := newTestModel()
	m.backendState = "Running"
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("c")})
	m = updated.(Model)

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("n")})
	m = updated.(Model)

	if m.confirmingDown {
		t.Error("confirmingDown = true after 'n', want false")
	}
	if m.connLoading {
		t.Error("connLoading = true after 'n', want false — nothing should have been triggered")
	}
	if cmd != nil {
		t.Error("Update('n') while confirming returned a non-nil cmd, want nil")
	}
}

func TestConnConfirmEscCancels(t *testing.T) {
	m := newTestModel()
	m.backendState = "Running"
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("c")})
	m = updated.(Model)

	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = updated.(Model)

	if m.confirmingDown {
		t.Error("confirmingDown = true after esc, want false")
	}
}

func TestConnToggleStoppedAppliesImmediately(t *testing.T) {
	m := newTestModel()
	m.backendState = "Stopped"

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("c")})
	m = updated.(Model)

	if m.confirmingDown {
		t.Error("confirmingDown = true after 'c' while Stopped, want false — no confirmation for bringing the connection up")
	}
	if !m.connLoading {
		t.Error("connLoading = false after 'c' while Stopped, want true")
	}
	if cmd == nil {
		t.Fatal("Update('c') while Stopped returned nil cmd, want a SetWantRunning command")
	}
}

func TestConnToggleNeedsLoginSetsInlineError(t *testing.T) {
	m := newTestModel()
	m.backendState = "NeedsLogin"

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("c")})
	m = updated.(Model)

	if m.connErr == nil {
		t.Fatal("connErr = nil after 'c' while NeedsLogin, want an inline error")
	}
	if cmd != nil {
		t.Error("Update('c') while NeedsLogin returned a non-nil cmd, want nil — no EditPrefs call is possible")
	}
	view := m.View()
	if !contains(view, "not logged in") {
		t.Errorf("View() = %q, want the inline error rendered", view)
	}
}

func TestConnErrClearsOnNextKeypress(t *testing.T) {
	m := newTestModel()
	m.backendState = "NeedsLogin"
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("c")})
	m = updated.(Model)
	if m.connErr == nil {
		t.Fatal("connErr = nil after 'c' while NeedsLogin, want it set (test precondition)")
	}

	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	m = updated.(Model)

	if m.connErr != nil {
		t.Errorf("connErr = %v after the next keypress, want nil", m.connErr)
	}
}

func TestConnResultMsgClearsLoadingOnSuccess(t *testing.T) {
	m := newTestModel()
	m.connLoading = true

	updated, cmd := m.Update(connResultMsg{})
	m = updated.(Model)

	if m.connLoading {
		t.Error("connLoading = true after a successful connResultMsg, want false")
	}
	if cmd != nil {
		t.Error("Update(connResultMsg{}) returned a non-nil cmd, want nil — state arrives on the next periodic refresh")
	}
}

func TestConnResultMsgSetsErrorOnFailure(t *testing.T) {
	m := newTestModel()
	m.connLoading = true

	updated, _ := m.Update(connResultMsg{err: errors.New("boom")})
	m = updated.(Model)

	if m.connLoading {
		t.Error("connLoading = true after a failed connResultMsg, want false")
	}
	if m.connErr == nil {
		t.Fatal("connErr = nil after a failed connResultMsg, want it set")
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

func TestAccountsOpensAndFetches(t *testing.T) {
	m := newTestModel()

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a")})
	m = updated.(Model)

	if !m.viewingAccounts {
		t.Fatal("viewingAccounts = false after 'a', want true")
	}
	if !m.accountsLoading {
		t.Fatal("accountsLoading = false immediately after 'a', want true")
	}
	if cmd == nil {
		t.Fatal("Update('a') returned nil cmd, want an accounts-fetch command")
	}
}

func TestAccountsEscCloses(t *testing.T) {
	m := newTestModel()
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a")})
	m = updated.(Model)

	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = updated.(Model)

	if m.viewingAccounts {
		t.Fatal("viewingAccounts = true after esc, want false")
	}
}

func TestAccountsCursorMovesAndClamps(t *testing.T) {
	m := newTestModel()
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a")})
	m = updated.(Model)
	updated, _ = m.Update(accountsMsg{all: []ipn.LoginProfile{{ID: "1"}, {ID: "2"}}})
	m = updated.(Model)

	for i := 0; i < 5; i++ {
		updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
		m = updated.(Model)
	}
	if m.accountsCursor != 1 {
		t.Errorf("accountsCursor = %d after 5x 'j' over 2 profiles, want clamped to 1", m.accountsCursor)
	}

	for i := 0; i < 5; i++ {
		updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("k")})
		m = updated.(Model)
	}
	if m.accountsCursor != 0 {
		t.Errorf("accountsCursor = %d after 5x 'k', want clamped to 0", m.accountsCursor)
	}
}

func TestAccountsMsgPopulatesListAndClearsLoading(t *testing.T) {
	m := newTestModel()
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a")})
	m = updated.(Model)

	current := ipn.LoginProfile{ID: "1ab3", Name: "alice@example.com"}
	updated, _ = m.Update(accountsMsg{current: current, all: []ipn.LoginProfile{current}})
	m = updated.(Model)

	if m.accountsLoading {
		t.Fatal("accountsLoading = true after accountsMsg, want false")
	}
	view := m.View()
	if !contains(view, "alice@example.com") {
		t.Errorf("View() missing accounts panel\n---\n%s", view)
	}
}

func TestAccountsMsgErrorKeepsEmptyStateAndSetsError(t *testing.T) {
	m := newTestModel()
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a")})
	m = updated.(Model)

	updated, _ = m.Update(accountsMsg{err: errors.New("boom")})
	m = updated.(Model)

	if m.accountsErr == nil {
		t.Fatal("accountsErr = nil after accountsMsg with an error, want it set")
	}
	if len(m.accountsAll) != 0 {
		t.Errorf("accountsAll = %+v after a failed fetch, want empty", m.accountsAll)
	}
}

func TestAccountsEnterSwitchesProfile(t *testing.T) {
	m := newTestModel()
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a")})
	m = updated.(Model)
	all := []ipn.LoginProfile{{ID: "1ab3", Name: "alice"}, {ID: "9f2c", Name: "bob"}}
	updated, _ = m.Update(accountsMsg{current: all[0], all: all})
	m = updated.(Model)
	m.accountsCursor = 1

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(Model)

	if !m.accountsLoading {
		t.Fatal("accountsLoading = false after enter on a profile row, want true")
	}
	if cmd == nil {
		t.Fatal("Update(enter) in accounts view returned nil cmd, want a switch command")
	}
}

func TestSwitchProfileMsgSuccessRefetches(t *testing.T) {
	m := newTestModel()
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a")})
	m = updated.(Model)

	updated, cmd := m.Update(switchProfileMsg{})
	m = updated.(Model)

	if m.accountsErr != nil {
		t.Errorf("accountsErr = %v after a successful switch, want nil", m.accountsErr)
	}
	if !m.accountsLoading {
		t.Error("accountsLoading = false after a successful switch, want true — a follow-up fetch is in flight")
	}
	if cmd == nil {
		t.Fatal("Update(switchProfileMsg{}) returned nil cmd, want a follow-up accounts-fetch command")
	}
}

func TestSwitchProfileMsgFailureKeepsListAndSetsError(t *testing.T) {
	m := newTestModel()
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a")})
	m = updated.(Model)
	all := []ipn.LoginProfile{{ID: "1ab3", Name: "alice"}}
	updated, _ = m.Update(accountsMsg{current: all[0], all: all})
	m = updated.(Model)

	updated, cmd := m.Update(switchProfileMsg{err: errors.New("boom")})
	m = updated.(Model)

	if m.accountsErr == nil {
		t.Fatal("accountsErr = nil after a failed switch, want it set")
	}
	if len(m.accountsAll) != 1 {
		t.Errorf("accountsAll = %+v after a failed switch, want the last-loaded list preserved", m.accountsAll)
	}
	if cmd != nil {
		t.Error("Update(switchProfileMsg{err}) returned a non-nil cmd, want nil — no re-fetch after a failed switch")
	}
}
