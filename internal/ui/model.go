package ui

import (
	"context"
	"fmt"
	"strings"
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

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
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

	case sshFinishedMsg:
		if msg.err != nil {
			m.err = msg.err
		}
		return m, fetchCmd(m.fetcher)

	case tea.KeyMsg:
		if m.searching {
			return m.updateSearch(msg)
		}
		return m.updateNormal(msg)
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
			return m, sshCmd(peer)
		}
	case "r":
		return m, fetchCmd(m.fetcher)
	}
	m.clampCursor()
	return m, nil
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
	var b strings.Builder

	b.WriteString(m.renderHeader())
	b.WriteString("\n\n")
	b.WriteString(renderTable(m.filtered, m.cursor))
	b.WriteString("\n\n")

	switch {
	case m.searching:
		b.WriteString(searchPromptStyle.Render("search: ") + m.searchInput.View())
	case m.err != nil:
		b.WriteString(errorStyle.Render("error: " + m.err.Error()))
	default:
		b.WriteString(helpStyle.Render("j/k move  g/G top/bottom  / search  enter ssh  r refresh  q quit"))
	}

	return b.String()
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
	return fmt.Sprintf("%s  self: %s (%s)  peers: %d", title, m.self.HostName, ip, len(m.peers))
}
