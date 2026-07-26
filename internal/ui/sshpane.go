package ui

import (
	"io"
	"os/exec"
	"sync"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/vt"

	"github.com/Wian47/ts-hud/internal/tsnet"
)

// sshPane holds the live state of an embedded ssh session: the pty-backed
// process and the terminal emulator rendering its output.
type sshPane struct {
	sess   ptySession
	term   *vt.SafeEmulator
	output chan []byte
	done   chan struct{}

	closeOnce sync.Once
}

// close kills/reaps the underlying process and unblocks both background
// goroutines: the read pump (via done) and the reply drain (via the
// emulator's reply pipe). Safe to call more than once (natural remote exit
// and manual detach can both reach it).
func (p *sshPane) close() {
	p.closeOnce.Do(func() {
		close(p.done)
		_ = p.sess.Close()
		closeTerminalReplyPipe(p.term)
	})
}

// closeTerminalReplyPipe closes the write half of the emulator's reply pipe,
// which makes the pending Read in drainTerminalReplies return io.EOF so that
// goroutine exits instead of leaking for the life of the process.
//
// It deliberately avoids vt's own (*Emulator).Close: that flips an
// unsynchronized `closed` bool which (*Emulator).Read also reads, and
// SafeEmulator guards neither method, so calling it while the drain
// goroutine sits in Read is a data race — `go test -race` flags it. Closing
// the pipe writer has the same effect (pending and subsequent reads end, and
// the emulator's reply writes fail fast instead of blocking) and io.Pipe is
// internally synchronized.
func closeTerminalReplyPipe(term *vt.SafeEmulator) {
	if pw, ok := term.InputPipe().(io.Closer); ok {
		_ = pw.Close()
	}
}

// sshStartedMsg carries the result of spawning an ssh session: either a
// ready pane, or the error that prevented one from starting (e.g. the ssh
// binary is missing). It does not represent ssh-level failures like a
// refused connection — those show up as ordinary output inside the pane,
// followed by the process exiting (sshClosedMsg).
type sshStartedMsg struct {
	pane *sshPane
	err  error
}

// sshOutputMsg carries one chunk of raw bytes read from the pty master.
// pane identifies which session produced the chunk: messages are delivered
// asynchronously, so one can still arrive from a pane that has already been
// detached and replaced. Update drops those instead of writing them into
// whatever pane happens to be current.
type sshOutputMsg struct {
	pane *sshPane
	data []byte
}

// sshClosedMsg is sent once the pty read loop ends, whether because the
// remote process exited or the session was closed locally. It carries no
// error: on Linux, reading a pty out from under an exited/closed process
// typically surfaces as EIO, which is the *normal* exit signal here, not a
// user-facing failure. As with sshOutputMsg, pane identifies the session it
// came from so a late close from an old pane can't tear down a new one.
type sshClosedMsg struct{ pane *sshPane }

// buildSSHCommand returns the command ts-hud runs to ssh into peer.
func buildSSHCommand(peer tsnet.Peer) *exec.Cmd {
	return exec.Command("ssh", peer.SSHTarget())
}

// startSSHPaneCmd spawns ssh into peer attached to a pty sized to cols x
// rows, and starts the background read pump. It returns immediately with
// the spawn result — it does not wait for the ssh handshake to complete.
func startSSHPaneCmd(spawner ptySpawner, peer tsnet.Peer, cols, rows int) tea.Cmd {
	return func() tea.Msg {
		sess, err := spawner.Start(buildSSHCommand(peer))
		if err != nil {
			return sshStartedMsg{err: err}
		}
		if err := sess.Setsize(rows, cols); err != nil {
			_ = sess.Close()
			return sshStartedMsg{err: err}
		}
		pane := &sshPane{
			sess:   sess,
			term:   vt.NewSafeEmulator(cols, rows),
			output: make(chan []byte),
			done:   make(chan struct{}),
		}
		go pumpPTYOutput(pane)
		go drainTerminalReplies(pane)
		return sshStartedMsg{pane: pane}
	}
}

// pumpPTYOutput reads from pane's session until it errors (remote exit,
// locally closed) and forwards each chunk on pane.output, then closes it.
// It selects on pane.done so a local close() unblocks a pending send
// immediately, even if the next Read hasn't returned yet.
func pumpPTYOutput(pane *sshPane) {
	defer close(pane.output)
	buf := make([]byte, 4096)
	for {
		n, err := pane.sess.Read(buf)
		if n > 0 {
			chunk := make([]byte, n)
			copy(chunk, buf[:n])
			select {
			case pane.output <- chunk:
			case <-pane.done:
				return
			}
		}
		if err != nil {
			return
		}
	}
}

// drainTerminalReplies forwards the emulator's answers to terminal queries
// back into the pty, where the remote program that asked is waiting for
// them (DA1/DA2 device attributes, cursor position reports, OSC color
// queries, DECRQM).
//
// This is not optional plumbing: vt writes those replies into an unbuffered
// pipe from inside Emulator.Write, so with nobody reading the pipe the first
// query byte sequence a remote program sends blocks Write forever. That
// Write runs on the Bubble Tea update goroutine while holding the
// SafeEmulator's write lock, so View()'s Render() blocks too — the whole TUI
// freezes with no way to detach. SafeEmulator.Read takes no lock, so this
// cannot contend with Write or Render. It returns once close() closes the
// emulator.
func drainTerminalReplies(pane *sshPane) {
	_, _ = io.Copy(pane.sess, pane.term)
}

// waitForPTYOutput blocks for the next chunk (or close) from pane's read
// pump, translating it into a Bubble Tea message. Update re-issues this
// after every sshOutputMsg to keep draining the pump.
func waitForPTYOutput(pane *sshPane) tea.Cmd {
	return func() tea.Msg {
		data, ok := <-pane.output
		if !ok {
			return sshClosedMsg{pane: pane}
		}
		return sshOutputMsg{pane: pane, data: data}
	}
}

// renderSSHPane returns the frame body for the embedded ssh view: a
// connecting indicator before the pane is ready, otherwise the terminal
// emulator's current screen contents.
func renderSSHPane(pane *sshPane) string {
	if pane == nil {
		return helpStyle.Render("connecting…")
	}
	return pane.term.Render()
}
