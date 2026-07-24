package ui

import (
	"os/exec"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/Wian47/ts-hud/internal/tsnet"
)

// sshFinishedMsg is sent once the suspended ssh subprocess exits and the
// TUI resumes control of the terminal.
type sshFinishedMsg struct{ err error }

func buildSSHCommand(peer tsnet.Peer) *exec.Cmd {
	return exec.Command("ssh", peer.SSHTarget())
}

func sshCmd(peer tsnet.Peer) tea.Cmd {
	c := buildSSHCommand(peer)
	return tea.ExecProcess(c, func(err error) tea.Msg {
		return sshFinishedMsg{err: err}
	})
}
