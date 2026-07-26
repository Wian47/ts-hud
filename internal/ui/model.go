package ui

import (
	"context"
	"fmt"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/Wian47/ts-hud/internal/tsnet"
)

type peersMsg struct {
	peers []tsnet.Peer
	self  *tsnet.Peer
}

type errMsg struct{ err error }

type tickMsg struct{}

type exitNodeResultMsg struct{ err error }

type derpReportMsg struct {
	regions []tsnet.DERPRegion
	err     error
}

// Model is the root Bubble Tea model for ts-hud.
type Model struct {
	fetcher         *tsnet.Fetcher
	refreshInterval time.Duration

	peers    []tsnet.Peer
	filtered []tsnet.Peer
	self     *tsnet.Peer
	cursor   int
	err      error

	searching   bool
	searchInput textinput.Model

	pickingExitNode bool
	exitNodeCursor  int
	allowLANAccess  bool

	viewingDERP bool
	derpRegions []tsnet.DERPRegion
	derpLoading bool
	derpErr     error

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
		peers, self, err := fetcher.Fetch(ctx)
		if err != nil {
			return errMsg{err: err}
		}
		return peersMsg{peers: peers, self: self}
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
		regions, err := fetcher.NetCheck(ctx)
		return derpReportMsg{regions: regions, err: err}
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
		m.derpRegions = msg.regions
		m.derpErr = msg.err
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
		m.sshPane = msg.pane
		return m, waitForPTYOutput(m.sshPane)

	case sshOutputMsg:
		if m.sshPane != nil {
			_, _ = m.sshPane.term.Write(msg.data)
			return m, waitForPTYOutput(m.sshPane)
		}
		return m, nil

	case sshClosedMsg:
		if m.sshPane != nil {
			m.sshPane.close()
		}
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
		case m.viewingSSH:
			return m.updateSSHPane(msg)
		default:
			return m.updateNormal(msg)
		}
	}

	return m, nil
}

func (m Model) updateNormal(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
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
		body = renderDERPTable(m.derpRegions, m.derpLoading, m.derpErr, width)
		footer = helpStyle.Render("r refresh  esc/d back")
	default:
		body = renderTable(m.filtered, m.cursor, width)
		switch {
		case m.searching:
			footer = searchPromptStyle.Render("search: ") + m.searchInput.View()
		case m.err != nil:
			footer = errorStyle.Render("error: " + m.err.Error())
		default:
			footer = helpStyle.Render("j/k move  g/G top/bottom  / search  enter ssh  x exit-node  d derp  r refresh  q quit")
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
	return fmt.Sprintf("%s  self: %s (%s)  peers: %d", title, m.self.DisplayName(), ip, len(m.peers))
}
