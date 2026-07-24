package ui

import "github.com/charmbracelet/lipgloss"

var (
	headerStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("15"))

	rowStyle         = lipgloss.NewStyle()
	selectedRowStyle = lipgloss.NewStyle().Bold(true).Background(lipgloss.Color("237"))

	onlineStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("42"))
	offlineStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("241"))

	connDirectStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("39"))
	connDERPStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("214"))
	connPeerRelayStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("170"))
	connUnknownStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("241"))

	searchPromptStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("214"))
	errorStyle        = lipgloss.NewStyle().Foreground(lipgloss.Color("203"))
	helpStyle         = lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
)
