package ui

import (
	"context"
	"fmt"
	"net/netip"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"tailscale.com/ipn"

	"github.com/Wian47/ts-hud/internal/tsnet"
)

type peersMsg struct {
	peers        []tsnet.Peer
	self         *tsnet.Peer
	backendState string
}

type errMsg struct{ err error }

type tickMsg struct{}

type exitNodeResultMsg struct{ err error }

type derpReportMsg struct {
	result tsnet.NetCheckResult
	err    error
}

type peerDetailReportMsg struct {
	result tsnet.PeerDetail
}

type prefsMsg struct {
	prefs *ipn.Prefs
	err   error
}

type connResultMsg struct{ err error }

// Model is the root Bubble Tea model for ts-hud.
type Model struct {
	fetcher         *tsnet.Fetcher
	refreshInterval time.Duration

	peers        []tsnet.Peer
	filtered     []tsnet.Peer
	self         *tsnet.Peer
	backendState string
	cursor       int
	err          error

	searching   bool
	searchInput textinput.Model

	pickingExitNode bool
	exitNodeCursor  int
	allowLANAccess  bool

	viewingDERP  bool
	derpNetCheck tsnet.NetCheckResult
	derpLoading  bool
	derpErr      error

	viewingPeerDetail bool
	peerDetailTarget  tsnet.Peer
	peerDetailResult  tsnet.PeerDetail
	peerDetailLoading bool

	viewingPrefs bool
	prefsCursor  int
	prefs        *ipn.Prefs
	prefsLoading bool
	prefsErr     error

	confirmingDown bool
	connLoading    bool
	connErr        error

	viewingSSH bool
	sshPane    *sshPane
	spawner    ptySpawner

	width  int
	height int
}

func NewModel(fetcher *tsnet.Fetcher, refreshInterval time.Duration) Model {
	input := textinput.New()
	input.Prompt = "/"
	input.CharLimit = 64

	return Model{
		fetcher:         fetcher,
		refreshInterval: refreshInterval,
		searchInput:     input,
		spawner:         realPTYSpawner{},
	}
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(fetchCmd(m.fetcher), tickCmd(m.refreshInterval))
}

func fetchCmd(fetcher *tsnet.Fetcher) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		peers, self, backendState, err := fetcher.Fetch(ctx)
		if err != nil {
			return errMsg{err: err}
		}
		return peersMsg{peers: peers, self: self, backendState: backendState}
	}
}

func tickCmd(interval time.Duration) tea.Cmd {
	return tea.Tick(interval, func(time.Time) tea.Msg {
		return tickMsg{}
	})
}

func setExitNodeCmd(fetcher *tsnet.Fetcher, peer tsnet.Peer, allowLAN bool) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := fetcher.SetExitNode(ctx, peer); err != nil {
			return exitNodeResultMsg{err: err}
		}
		if err := fetcher.SetExitNodeAllowLANAccess(ctx, allowLAN); err != nil {
			return exitNodeResultMsg{err: err}
		}
		return exitNodeResultMsg{}
	}
}

func clearExitNodeCmd(fetcher *tsnet.Fetcher) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return exitNodeResultMsg{err: fetcher.ClearExitNode(ctx)}
	}
}

// derpCheckCmd runs a live network check, sending real STUN probes to every
// DERP region. That's slower than the other commands here (bounded by
// netcheck.ReportTimeout, 5s), hence the longer context deadline.
func derpCheckCmd(fetcher *tsnet.Fetcher) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		result, err := fetcher.NetCheck(ctx)
		return derpReportMsg{result: result, err: err}
	}
}

// peerDetailCmd runs a live ping + whois probe against one peer's primary
// Tailscale IP.
func peerDetailCmd(fetcher *tsnet.Fetcher, ip netip.Addr) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return peerDetailReportMsg{result: fetcher.PeerDetail(ctx, ip)}
	}
}

func prefsFetchCmd(fetcher *tsnet.Fetcher) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		prefs, err := fetcher.GetPrefs(ctx)
		return prefsMsg{prefs: prefs, err: err}
	}
}

func setRunSSHCmd(fetcher *tsnet.Fetcher, run bool) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		prefs, err := fetcher.SetRunSSH(ctx, run)
		return prefsMsg{prefs: prefs, err: err}
	}
}

func setShieldsUpCmd(fetcher *tsnet.Fetcher, up bool) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		prefs, err := fetcher.SetShieldsUp(ctx, up)
		return prefsMsg{prefs: prefs, err: err}
	}
}

func setAcceptRoutesCmd(fetcher *tsnet.Fetcher, accept bool) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		prefs, err := fetcher.SetAcceptRoutes(ctx, accept)
		return prefsMsg{prefs: prefs, err: err}
	}
}

func setAcceptDNSCmd(fetcher *tsnet.Fetcher, accept bool) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		prefs, err := fetcher.SetAcceptDNS(ctx, accept)
		return prefsMsg{prefs: prefs, err: err}
	}
}

func setAdvertiseExitNodeCmd(fetcher *tsnet.Fetcher, advertise bool) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		prefs, err := fetcher.SetAdvertiseExitNode(ctx, advertise)
		return prefsMsg{prefs: prefs, err: err}
	}
}

func setWantRunningCmd(fetcher *tsnet.Fetcher, running bool) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_, err := fetcher.SetWantRunning(ctx, running)
		return connResultMsg{err: err}
	}
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		if m.viewingSSH && m.sshPane != nil {
			cols, rows := contentWidth(m.width), contentHeight(m.height)
			m.sshPane.term.Resize(cols, rows)
			_ = m.sshPane.sess.Setsize(rows, cols)
		}
		return m, nil

	case peersMsg:
		m.err = nil
		m.peers = msg.peers
		m.self = msg.self
		m.backendState = msg.backendState
		m.applyFilter()
		return m, nil

	case errMsg:
		m.err = msg.err
		return m, nil

	case tickMsg:
		return m, tea.Batch(fetchCmd(m.fetcher), tickCmd(m.refreshInterval))

	case exitNodeResultMsg:
		if msg.err != nil {
			m.err = msg.err
		}
		return m, fetchCmd(m.fetcher)

	case derpReportMsg:
		m.derpLoading = false
		m.derpNetCheck = msg.result
		m.derpErr = msg.err
		return m, nil

	case peerDetailReportMsg:
		m.peerDetailLoading = false
		m.peerDetailResult = msg.result
		return m, nil

	case prefsMsg:
		m.prefsLoading = false
		if msg.err != nil {
			m.prefsErr = msg.err
			return m, nil
		}
		m.prefs = msg.prefs
		m.prefsErr = nil
		return m, nil

	case connResultMsg:
		m.connLoading = false
		if msg.err != nil {
			m.connErr = msg.err
		}
		return m, nil

	case sshStartedMsg:
		if !m.viewingSSH {
			// Detached before the spawn finished; don't leave an orphaned
			// session running with nobody driving it.
			if msg.pane != nil {
				msg.pane.close()
			}
			return m, nil
		}
		if msg.err != nil {
			m.viewingSSH = false
			m.err = msg.err
			return m, nil
		}
		if m.sshPane != nil {
			// A session is already live, so this spawn is redundant.
			// updateNormal/updateSSHPane shouldn't let it happen, but never
			// overwrite a live pane — that would leak its process, pty and
			// goroutines with nothing left holding a reference to close them.
			msg.pane.close()
			return m, nil
		}
		m.sshPane = msg.pane
		return m, waitForPTYOutput(m.sshPane)

	case sshOutputMsg:
		// Stale chunk from a pane that has already been detached or
		// replaced: dropping it keeps a dead session's output out of the
		// current pane's emulator. Its own pump is independent, so there is
		// nothing to re-arm here either.
		if msg.pane == nil || msg.pane != m.sshPane {
			return m, nil
		}
		_, _ = m.sshPane.term.Write(msg.data)
		return m, waitForPTYOutput(m.sshPane)

	case sshClosedMsg:
		// Likewise: a late close from an old pane must not tear down the
		// session the user is currently looking at.
		if msg.pane == nil || msg.pane != m.sshPane {
			return m, nil
		}
		m.sshPane.close()
		m.viewingSSH = false
		m.sshPane = nil
		return m, fetchCmd(m.fetcher)

	case tea.KeyMsg:
		switch {
		case m.searching:
			return m.updateSearch(msg)
		case m.pickingExitNode:
			return m.updateExitNodePicker(msg)
		case m.viewingDERP:
			return m.updateDERPView(msg)
		case m.viewingPeerDetail:
			return m.updatePeerDetailView(msg)
		case m.viewingPrefs:
			return m.updatePrefsView(msg)
		case m.confirmingDown:
			return m.updateConnConfirm(msg)
		case m.viewingSSH:
			return m.updateSSHPane(msg)
		default:
			return m.updateNormal(msg)
		}
	}

	return m, nil
}

func (m Model) updateNormal(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	m.connErr = nil
	switch msg.String() {
	case "q", "ctrl+c":
		return m, tea.Quit
	case "j", "down":
		m.moveCursor(1)
	case "k", "up":
		m.moveCursor(-1)
	case "g":
		m.cursor = 0
	case "G":
		m.cursor = len(m.filtered) - 1
	case "/":
		m.searching = true
		m.searchInput.Focus()
		return m, nil
	case "enter":
		if peer, ok := m.selectedPeer(); ok && peer.Online {
			m.viewingSSH = true
			m.err = nil
			cols, rows := contentWidth(m.width), contentHeight(m.height)
			return m, startSSHPaneCmd(m.spawner, peer, cols, rows)
		}
	case "r":
		return m, fetchCmd(m.fetcher)
	case "x":
		m.pickingExitNode = true
		m.exitNodeCursor = m.initialExitNodeCursor()
		return m, nil
	case "d":
		m.viewingDERP = true
		m.derpLoading = true
		m.derpErr = nil
		return m, derpCheckCmd(m.fetcher)
	case "i":
		if peer, ok := m.selectedPeer(); ok && len(peer.IPs) > 0 {
			m.viewingPeerDetail = true
			m.peerDetailTarget = peer
			m.peerDetailLoading = true
			m.peerDetailResult = tsnet.PeerDetail{}
			return m, peerDetailCmd(m.fetcher, peer.IPs[0])
		}
	case "p":
		m.viewingPrefs = true
		m.prefsCursor = 0
		m.prefsLoading = true
		m.prefsErr = nil
		return m, prefsFetchCmd(m.fetcher)
	case "c":
		if m.connLoading {
			return m, nil
		}
		switch m.backendState {
		case "Running":
			m.confirmingDown = true
		case "Stopped", "Starting":
			m.connLoading = true
			return m, setWantRunningCmd(m.fetcher, true)
		case "NeedsLogin", "NeedsMachineAuth", "NoState":
			m.connErr = fmt.Errorf("not logged in — run tailscale login")
		}
		return m, nil
	}
	m.clampCursor()
	return m, nil
}

func (m Model) updateDERPView(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "d":
		m.viewingDERP = false
		return m, nil
	case "r":
		m.derpLoading = true
		m.derpErr = nil
		return m, derpCheckCmd(m.fetcher)
	}
	return m, nil
}

func (m Model) updatePeerDetailView(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "i":
		m.viewingPeerDetail = false
		return m, nil
	case "r":
		m.peerDetailLoading = true
		return m, peerDetailCmd(m.fetcher, m.peerDetailTarget.IPs[0])
	}
	return m, nil
}

func (m Model) updatePrefsView(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "p":
		m.viewingPrefs = false
		return m, nil
	case "j", "down":
		m.prefsCursor++
		m.clampPrefsCursor()
	case "k", "up":
		m.prefsCursor--
		m.clampPrefsCursor()
	case "enter", " ":
		if m.prefs == nil {
			return m, nil
		}
		next := !prefRows(m.prefs)[m.prefsCursor].on
		m.prefsLoading = true
		switch m.prefsCursor {
		case 0:
			return m, setRunSSHCmd(m.fetcher, next)
		case 1:
			return m, setShieldsUpCmd(m.fetcher, next)
		case 2:
			return m, setAcceptRoutesCmd(m.fetcher, next)
		case 3:
			return m, setAcceptDNSCmd(m.fetcher, next)
		case 4:
			return m, setAdvertiseExitNodeCmd(m.fetcher, next)
		}
	}
	return m, nil
}

func (m Model) updateConnConfirm(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "y":
		m.confirmingDown = false
		m.connLoading = true
		return m, setWantRunningCmd(m.fetcher, false)
	case "n", "esc":
		m.confirmingDown = false
	}
	return m, nil
}

func (m *Model) clampPrefsCursor() {
	if m.prefsCursor < 0 {
		m.prefsCursor = 0
	}
	if m.prefsCursor > numPrefRows-1 {
		m.prefsCursor = numPrefRows - 1
	}
}

func (m Model) updateSSHPane(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if msg.Type == tea.KeyCtrlQ {
		if m.sshPane != nil {
			m.sshPane.close()
		}
		m.viewingSSH = false
		m.sshPane = nil
		return m, fetchCmd(m.fetcher)
	}
	if m.sshPane != nil {
		if b := keyMsgToBytes(msg); len(b) > 0 {
			_, _ = m.sshPane.sess.Write(b)
		}
	}
	return m, nil
}

func (m Model) updateExitNodePicker(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "x":
		m.pickingExitNode = false
		return m, nil
	case "j", "down":
		m.exitNodeCursor++
		m.clampExitNodeCursor()
	case "k", "up":
		m.exitNodeCursor--
		m.clampExitNodeCursor()
	case "l":
		m.allowLANAccess = !m.allowLANAccess
	case "enter":
		candidates := m.exitNodeCandidates()
		m.pickingExitNode = false
		if m.exitNodeCursor == 0 {
			return m, clearExitNodeCmd(m.fetcher)
		}
		return m, setExitNodeCmd(m.fetcher, candidates[m.exitNodeCursor-1], m.allowLANAccess)
	}
	return m, nil
}

// exitNodeCandidates returns the peers eligible to be selected as an exit
// node. Index i in the picker's cursor corresponds to candidates[i-1];
// cursor 0 is the synthetic "none" entry that clears the exit node.
func (m Model) exitNodeCandidates() []tsnet.Peer {
	candidates := make([]tsnet.Peer, 0, len(m.peers))
	for _, p := range m.peers {
		if p.CanBeExitNode {
			candidates = append(candidates, p)
		}
	}
	return candidates
}

// initialExitNodeCursor points the picker at the currently active exit
// node, or at the "none" entry if no peer is currently selected.
func (m Model) initialExitNodeCursor() int {
	for i, p := range m.exitNodeCandidates() {
		if p.IsExitNode {
			return i + 1
		}
	}
	return 0
}

func (m *Model) clampExitNodeCursor() {
	max := len(m.exitNodeCandidates())
	if m.exitNodeCursor < 0 {
		m.exitNodeCursor = 0
	}
	if m.exitNodeCursor > max {
		m.exitNodeCursor = max
	}
}

func (m Model) updateSearch(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.searching = false
		m.searchInput.Blur()
		m.searchInput.SetValue("")
		m.applyFilter()
		return m, nil
	case "enter":
		m.searching = false
		m.searchInput.Blur()
		return m, nil
	}

	var cmd tea.Cmd
	m.searchInput, cmd = m.searchInput.Update(msg)
	m.applyFilter()
	return m, cmd
}

func (m *Model) clampCursor() {
	if len(m.filtered) == 0 {
		m.cursor = 0
		return
	}
	if m.cursor < 0 {
		m.cursor = 0
	}
	if m.cursor > len(m.filtered)-1 {
		m.cursor = len(m.filtered) - 1
	}
}

func (m *Model) moveCursor(delta int) {
	m.cursor += delta
	m.clampCursor()
}

func (m Model) selectedPeer() (tsnet.Peer, bool) {
	if len(m.filtered) == 0 || m.cursor < 0 || m.cursor >= len(m.filtered) {
		return tsnet.Peer{}, false
	}
	return m.filtered[m.cursor], true
}

func (m *Model) applyFilter() {
	query := m.searchInput.Value()
	filtered := make([]tsnet.Peer, 0, len(m.peers))
	for _, p := range m.peers {
		if p.MatchesQuery(query) {
			filtered = append(filtered, p)
		}
	}
	m.filtered = filtered
	m.clampCursor()
}

func (m Model) View() string {
	width := contentWidth(m.width)
	header := m.renderHeader()

	var body, footer string
	switch {
	case m.viewingSSH:
		body = renderSSHPane(m.sshPane)
		footer = helpStyle.Render("ctrl+q detach")
	case m.pickingExitNode:
		body = renderExitNodePicker(m.exitNodeCandidates(), m.exitNodeCursor, m.allowLANAccess, width)
		footer = helpStyle.Render("j/k move  enter select  l toggle LAN access  esc cancel")
	case m.viewingDERP:
		body = renderDERPTable(m.derpNetCheck, m.derpLoading, m.derpErr, width)
		footer = helpStyle.Render("r refresh  esc/d back")
	case m.viewingPeerDetail:
		body = renderPeerDetail(m.peerDetailTarget, m.peerDetailResult, m.peerDetailLoading)
		footer = helpStyle.Render("r refresh  esc/i back")
	case m.viewingPrefs:
		body = renderPrefsPanel(m.prefs, m.prefsCursor, m.prefsLoading, m.prefsErr, width)
		footer = helpStyle.Render("j/k move  enter toggle  esc/p back")
	default:
		body = renderTable(m.filtered, m.cursor, width)
		switch {
		case m.confirmingDown:
			footer = errorStyle.Render("Bring Tailscale down? y confirm  n/esc cancel")
		case m.searching:
			footer = searchPromptStyle.Render("search: ") + m.searchInput.View()
		case m.err != nil:
			footer = errorStyle.Render("error: " + m.err.Error())
		case m.connErr != nil:
			footer = errorStyle.Render(m.connErr.Error())
		default:
			footer = helpStyle.Render("j/k move  g/G top/bottom  / search  enter ssh  x exit-node  d derp  i info  p prefs  c connection  r refresh  q quit")
		}
	}

	return renderFrame(m.width, m.height, header, body, footer)
}

func (m Model) renderHeader() string {
	title := headerStyle.Render("ts-hud")
	if m.self == nil {
		return title
	}
	ip := ""
	if len(m.self.IPs) > 0 {
		ip = m.self.IPs[0].String()
	}
	header := fmt.Sprintf("%s  self: %s (%s)  peers: %d", title, m.self.DisplayName(), ip, len(m.peers))
	if m.backendState != "" && m.backendState != "Running" {
		header += "  " + errorStyle.Render("["+strings.ToUpper(m.backendState)+"]")
	}
	return header
}
