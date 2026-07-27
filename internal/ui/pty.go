package ui

import (
	"io"
	"os"
	"os/exec"

	"github.com/creack/pty"
)

// ptySpawner starts a command attached to a real pseudo-terminal. It's
// narrowed to an interface — mirroring the localClient pattern in
// internal/tsnet/client.go — so tests can substitute a fake instead of
// spawning real processes.
type ptySpawner interface {
	Start(cmd *exec.Cmd) (ptySession, error)
}

// ptySession is a running pty-attached process: its master side for
// reading/writing terminal I/O, plus resize support.
type ptySession interface {
	io.ReadWriteCloser
	Setsize(rows, cols int) error
}

// realPTYSpawner spawns real OS pseudo-terminals via github.com/creack/pty.
type realPTYSpawner struct{}

func (realPTYSpawner) Start(cmd *exec.Cmd) (ptySession, error) {
	f, err := pty.Start(cmd)
	if err != nil {
		return nil, err
	}
	return &realPTY{f: f, cmd: cmd}, nil
}

// realPTY adapts creack/pty's *os.File-plus-*exec.Cmd pair to ptySession.
type realPTY struct {
	f   *os.File
	cmd *exec.Cmd
}

func (r *realPTY) Read(p []byte) (int, error)  { return r.f.Read(p) }
func (r *realPTY) Write(p []byte) (int, error) { return r.f.Write(p) }

// Close kills the process, closes the pty master, and reaps the process so
// it doesn't linger as a zombie. Safe to call more than once.
func (r *realPTY) Close() error {
	if r.cmd.Process != nil {
		_ = r.cmd.Process.Kill()
	}
	closeErr := r.f.Close()
	_ = r.cmd.Wait()
	return closeErr
}

func (r *realPTY) Setsize(rows, cols int) error {
	return pty.Setsize(r.f, &pty.Winsize{Rows: uint16(rows), Cols: uint16(cols)})
}
