package ui

import (
	"context"
	"fmt"
	"strings"
	"time"

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

	width  int
	height int
}

func NewModel(fetcher *tsnet.Fetcher, refreshInterval time.Duration) Model {
	return Model{
		fetcher:         fetcher,
		refreshInterval: refreshInterval,
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

	case tea.KeyMsg:
		return m.updateNormal(msg)
	}

	return m, nil
}

func (m Model) updateNormal(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q", "ctrl+c":
		return m, tea.Quit
	case "r":
		return m, fetchCmd(m.fetcher)
	}
	return m, nil
}

// applyFilter recomputes the visible peer list. Until Task 5 adds live
// search it simply mirrors peers, keeping the cursor in bounds.
func (m *Model) applyFilter() {
	m.filtered = m.peers
	if m.cursor > len(m.filtered)-1 {
		m.cursor = len(m.filtered) - 1
	}
	if m.cursor < 0 {
		m.cursor = 0
	}
}

func (m Model) View() string {
	var b strings.Builder

	b.WriteString(m.renderHeader())
	b.WriteString("\n\n")
	b.WriteString(renderTable(m.filtered, m.cursor))
	b.WriteString("\n\n")

	if m.err != nil {
		b.WriteString(errorStyle.Render("error: " + m.err.Error()))
	} else {
		b.WriteString(helpStyle.Render("r refresh  q quit"))
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
