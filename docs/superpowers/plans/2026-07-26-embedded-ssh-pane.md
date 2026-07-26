# Embedded SSH Pane Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the current full-terminal `tea.ExecProcess` ssh handoff with an ssh session that runs live inside the ts-hud bordered frame — chrome stays visible, peer list hides while connected, `ctrl+q` detaches.

**Architecture:** `ssh` is spawned attached to a real OS pty (`github.com/creack/pty`). A background goroutine streams the pty's output into a `github.com/charmbracelet/x/vt` `SafeEmulator`, whose `Render()` becomes the frame body every `View()` call. Keystrokes bypass Bubble Tea's normal key dispatch and get re-encoded to raw bytes written straight to the pty. One session at a time; detaching or the remote exiting always kills/reaps the process and returns to the peer table.

**Tech Stack:** Go 1.26.4, Bubble Tea v1.3.10, `github.com/creack/pty` (new), `github.com/charmbracelet/x/vt` (new).

## Global Constraints

- Go 1.26.4 floor (per `go.mod`) — do not use anything newer.
- Follow the existing DI-for-testability convention: narrow interfaces (`localClient` in `internal/tsnet/client.go` is the precedent) so tests substitute fakes instead of spawning real processes/networks.
- TDD: write the failing test before the implementation for every code task.
- No new abstractions beyond what's specified here — YAGNI. No mouse/bracketed-paste support, no session backgrounding/reattach, no split-pane layout.
- This targets version **v0.5.0** (MINOR bump — new capability, no breaking change), per the project's strict-semver convention.
- Manual verification over tmux against the real tailnet is required before this is considered done, matching how the DERP latency matrix and exit-node picker were verified.

---

### Task 1: Key-to-bytes encoder

**Files:**
- Create: `internal/ui/sshkeys.go`
- Test: `internal/ui/sshkeys_test.go`

**Interfaces:**
- Produces: `func keyMsgToBytes(msg tea.KeyMsg) []byte` — used by Task 4's `updateSSHPane`.

- [ ] **Step 1: Write the failing test**

```go
// internal/ui/sshkeys_test.go
package ui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestKeyMsgToBytes(t *testing.T) {
	tests := []struct {
		name string
		msg  tea.KeyMsg
		want []byte
	}{
		{"lowercase rune", tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a")}, []byte("a")},
		{"unicode rune", tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("é")}, []byte("é")},
		{"space", tea.KeyMsg{Type: tea.KeySpace}, []byte(" ")},
		{"enter", tea.KeyMsg{Type: tea.KeyEnter}, []byte("\r")},
		{"tab", tea.KeyMsg{Type: tea.KeyTab}, []byte("\t")},
		{"backspace", tea.KeyMsg{Type: tea.KeyBackspace}, []byte{0x7f}},
		{"esc", tea.KeyMsg{Type: tea.KeyEsc}, []byte{0x1b}},
		{"up", tea.KeyMsg{Type: tea.KeyUp}, []byte("\x1b[A")},
		{"down", tea.KeyMsg{Type: tea.KeyDown}, []byte("\x1b[B")},
		{"left", tea.KeyMsg{Type: tea.KeyLeft}, []byte("\x1b[D")},
		{"right", tea.KeyMsg{Type: tea.KeyRight}, []byte("\x1b[C")},
		{"home", tea.KeyMsg{Type: tea.KeyHome}, []byte("\x1b[H")},
		{"end", tea.KeyMsg{Type: tea.KeyEnd}, []byte("\x1b[F")},
		{"pgup", tea.KeyMsg{Type: tea.KeyPgUp}, []byte("\x1b[5~")},
		{"pgdown", tea.KeyMsg{Type: tea.KeyPgDown}, []byte("\x1b[6~")},
		{"delete", tea.KeyMsg{Type: tea.KeyDelete}, []byte("\x1b[3~")},
		{"ctrl+a", tea.KeyMsg{Type: tea.KeyCtrlA}, []byte{0x01}},
		{"ctrl+c", tea.KeyMsg{Type: tea.KeyCtrlC}, []byte{0x03}},
		{"ctrl+z", tea.KeyMsg{Type: tea.KeyCtrlZ}, []byte{0x1a}},
		{"alt+rune", tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("b"), Alt: true}, []byte{0x1b, 'b'}},
		{"unsupported", tea.KeyMsg{Type: tea.KeyF1}, nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := keyMsgToBytes(tt.msg)
			if string(got) != string(tt.want) {
				t.Errorf("keyMsgToBytes(%+v) = %v, want %v", tt.msg, got, tt.want)
			}
		})
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/ui/... -run TestKeyMsgToBytes -v`
Expected: FAIL with `undefined: keyMsgToBytes`

- [ ] **Step 3: Write the implementation**

```go
// internal/ui/sshkeys.go
package ui

import tea "github.com/charmbracelet/bubbletea"

// keyMsgToBytes encodes a subset of tea.KeyMsg values into the raw bytes an
// ssh session's pty expects on stdin. It is best-effort — the common set
// used by shells, vim, htop, and similar full-screen programs — not a
// byte-perfect terminal input encoder. Mouse events and bracketed paste are
// out of scope.
func keyMsgToBytes(msg tea.KeyMsg) []byte {
	switch msg.Type {
	case tea.KeyRunes:
		return withAltPrefix(msg.Alt, []byte(string(msg.Runes)))
	case tea.KeySpace:
		return withAltPrefix(msg.Alt, []byte(" "))
	case tea.KeyEnter:
		return withAltPrefix(msg.Alt, []byte("\r"))
	case tea.KeyTab:
		return withAltPrefix(msg.Alt, []byte("\t"))
	case tea.KeyBackspace:
		return withAltPrefix(msg.Alt, []byte{0x7f})
	case tea.KeyEsc:
		return []byte{0x1b}
	case tea.KeyUp:
		return []byte("\x1b[A")
	case tea.KeyDown:
		return []byte("\x1b[B")
	case tea.KeyRight:
		return []byte("\x1b[C")
	case tea.KeyLeft:
		return []byte("\x1b[D")
	case tea.KeyHome:
		return []byte("\x1b[H")
	case tea.KeyEnd:
		return []byte("\x1b[F")
	case tea.KeyPgUp:
		return []byte("\x1b[5~")
	case tea.KeyPgDown:
		return []byte("\x1b[6~")
	case tea.KeyDelete:
		return []byte("\x1b[3~")
	}
	if msg.Type >= tea.KeyCtrlA && msg.Type <= tea.KeyCtrlZ {
		return []byte{byte(msg.Type)}
	}
	return nil
}

// withAltPrefix prepends the ESC byte that terminals use to signal an
// alt-modified key, when alt was held.
func withAltPrefix(alt bool, b []byte) []byte {
	if !alt {
		return b
	}
	return append([]byte{0x1b}, b...)
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/ui/... -run TestKeyMsgToBytes -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/ui/sshkeys.go internal/ui/sshkeys_test.go
git commit -m "feat: add key-to-bytes encoder for embedded ssh pane input"
```

---

### Task 2: Body-height sizing helper

**Files:**
- Modify: `internal/ui/frame.go`
- Create: `internal/ui/frame_test.go`

**Interfaces:**
- Consumes: `defaultTermHeight` (existing const in `frame.go`).
- Produces: `func contentHeight(termHeight int) int` — used by Task 4/6 to size the pty and the `vt.SafeEmulator` to exactly fill the frame body.

- [ ] **Step 1: Write the failing test**

```go
// internal/ui/frame_test.go
package ui

import "testing"

func TestContentHeight(t *testing.T) {
	tests := []struct {
		name string
		h    int
		want int
	}{
		{"typical 24-row terminal", 24, 18},
		{"tiny terminal clamps to 1", 3, 1},
		{"zero falls back to default height", 0, contentHeight(defaultTermHeight)},
		{"negative falls back to default height", -5, contentHeight(defaultTermHeight)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := contentHeight(tt.h); got != tt.want {
				t.Errorf("contentHeight(%d) = %d, want %d", tt.h, got, tt.want)
			}
		})
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/ui/... -run TestContentHeight -v`
Expected: FAIL with `undefined: contentHeight`

- [ ] **Step 3: Add the implementation**

Add to `internal/ui/frame.go`, directly below the existing `contentWidth` function (after line 35):

```go
// contentHeight returns how many terminal rows are available for body
// content inside a frame of the given terminal height, assuming a
// single-line header and a single-line footer (true for every caller in
// this package). Callers that need to size body content exactly — like a
// live-rendered ssh pane — use this instead of letting renderFrame pad
// with blank lines.
func contentHeight(termHeight int) int {
	if termHeight <= 0 {
		termHeight = defaultTermHeight
	}
	// 2 border rows + 2 divider rows + 1 header row + 1 footer row.
	h := termHeight - 6
	if h < 1 {
		h = 1
	}
	return h
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/ui/... -run TestContentHeight -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/ui/frame.go internal/ui/frame_test.go
git commit -m "feat: add contentHeight frame-sizing helper"
```

---

### Task 3: PTY spawning abstraction

**Files:**
- Create: `internal/ui/pty.go`
- Test: `internal/ui/pty_test.go`

**Interfaces:**
- Produces:
  ```go
  type ptySpawner interface {
      Start(cmd *exec.Cmd) (ptySession, error)
  }
  type ptySession interface {
      io.ReadWriteCloser
      Setsize(rows, cols int) error
  }
  type realPTYSpawner struct{}
  ```
  `realPTYSpawner{}` is the production implementation; Task 4 injects it as `Model`'s default and tests substitute a fake.

- [ ] **Step 1: Add the dependency**

Run: `go get github.com/creack/pty@v1.1.24 && go mod tidy`

- [ ] **Step 2: Write the failing test**

```go
// internal/ui/pty_test.go
package ui

import (
	"bufio"
	"os/exec"
	"testing"
)

func TestRealPTYSpawnerRunsCommandAndCapturesOutput(t *testing.T) {
	spawner := realPTYSpawner{}
	sess, err := spawner.Start(exec.Command("echo", "hello-pty"))
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	defer sess.Close()

	scanner := bufio.NewScanner(sess)
	if !scanner.Scan() {
		t.Fatalf("expected an output line, got none (scanner err: %v)", scanner.Err())
	}
	if got := scanner.Text(); got != "hello-pty" {
		t.Errorf("output line = %q, want %q", got, "hello-pty")
	}
}

func TestRealPTYSpawnerSetsize(t *testing.T) {
	spawner := realPTYSpawner{}
	sess, err := spawner.Start(exec.Command("cat"))
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	defer sess.Close()

	if err := sess.Setsize(40, 100); err != nil {
		t.Errorf("Setsize() error = %v, want nil", err)
	}
}
```

- [ ] **Step 3: Run test to verify it fails**

Run: `go test ./internal/ui/... -run TestRealPTYSpawner -v`
Expected: FAIL with `undefined: realPTYSpawner`

- [ ] **Step 4: Write the implementation**

```go
// internal/ui/pty.go
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
```

- [ ] **Step 5: Run test to verify it passes**

Run: `go test ./internal/ui/... -run TestRealPTYSpawner -v`
Expected: PASS

- [ ] **Step 6: Full package build check**

Run: `go build ./... && go vet ./...`
Expected: no errors

- [ ] **Step 7: Commit**

```bash
git add go.mod go.sum internal/ui/pty.go internal/ui/pty_test.go
git commit -m "feat: add real pty spawner for embedded ssh sessions"
```

---

### Task 4: SSH pane lifecycle and Model wiring

This is the core task: it replaces the old full-takeover ssh flow entirely (deletes `internal/ui/ssh.go`) with the pty-backed pane, wired into `Model`'s update loop.

**Files:**
- Create: `internal/ui/sshpane.go`
- Test: `internal/ui/sshpane_test.go`
- Modify: `internal/ui/model.go`
- Modify: `internal/ui/model_test.go`
- Delete: `internal/ui/ssh.go`

**Interfaces:**
- Consumes: `ptySpawner`/`ptySession` (Task 3), `keyMsgToBytes` (Task 1), `tsnet.Peer.SSHTarget()` (existing).
- Produces:
  ```go
  type sshPane struct { /* unexported fields */ }
  func (p *sshPane) close()
  type sshStartedMsg struct{ pane *sshPane; err error }
  type sshOutputMsg struct{ data []byte }
  type sshClosedMsg struct{}
  func startSSHPaneCmd(spawner ptySpawner, peer tsnet.Peer, cols, rows int) tea.Cmd
  func waitForPTYOutput(pane *sshPane) tea.Cmd
  func buildSSHCommand(peer tsnet.Peer) *exec.Cmd  // moved from ssh.go, unchanged
  ```
  `Model` gains fields `spawner ptySpawner`, `viewingSSH bool`, `sshPane *sshPane`, and method `updateSSHPane`. Task 5 (View rendering) and Task 6 (resize) build on these.

**Note on the design deviation from the spec:** the spec said mid-session errors surface as a footer error. In practice, closing a pty out from under a blocked `Read()` on Linux typically returns `EIO`, not a clean `io.EOF` — that's the *normal* exit path, not a real error, and any actual ssh-level error text (e.g. "connection refused") already scrolled by inside the pane itself before the process exited. So `sshClosedMsg` carries no error field and never shows a footer error; only a **spawn**-time failure (`pty.Start` itself failing — e.g. `ssh` missing from `$PATH`) surfaces via `sshStartedMsg.err`.

- [ ] **Step 1: Add the dependency**

Run: `go get github.com/charmbracelet/x/vt@latest && go mod tidy`

- [ ] **Step 2: Write the failing tests**

```go
// internal/ui/sshpane_test.go
package ui

import (
	"errors"
	"io"
	"net/netip"
	"os/exec"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/vt"

	"github.com/Wian47/ts-hud/internal/tsnet"
)

// fakePTYSpawner and fakePTYSession let pane-lifecycle tests run without
// spawning real processes, mirroring fakeLocalClient in
// internal/tsnet/client_test.go.

type fakePTYSpawner struct {
	sess ptySession
	err  error
}

func (f fakePTYSpawner) Start(cmd *exec.Cmd) (ptySession, error) { return f.sess, f.err }

type fakePTYSession struct {
	readCh  chan []byte
	closeCh chan struct{}
	closed  bool
	writes  [][]byte
	sizes   [][2]int
}

func newFakePTYSession() *fakePTYSession {
	return &fakePTYSession{readCh: make(chan []byte, 8), closeCh: make(chan struct{})}
}

func (f *fakePTYSession) Read(p []byte) (int, error) {
	select {
	case chunk, ok := <-f.readCh:
		if !ok {
			return 0, io.EOF
		}
		return copy(p, chunk), nil
	case <-f.closeCh:
		return 0, io.ErrClosedPipe
	}
}

func (f *fakePTYSession) Write(p []byte) (int, error) {
	cp := make([]byte, len(p))
	copy(cp, p)
	f.writes = append(f.writes, cp)
	return len(p), nil
}

func (f *fakePTYSession) Close() error {
	if !f.closed {
		f.closed = true
		close(f.closeCh)
	}
	return nil
}

func (f *fakePTYSession) Setsize(rows, cols int) error {
	f.sizes = append(f.sizes, [2]int{rows, cols})
	return nil
}

func TestStartSSHPaneCmdReturnsRunningPaneOnSuccess(t *testing.T) {
	sess := newFakePTYSession()
	spawner := fakePTYSpawner{sess: sess}
	peer := tsnet.Peer{HostName: "bravo", IPs: []netip.Addr{netip.MustParseAddr("100.64.0.2")}}

	msg := startSSHPaneCmd(spawner, peer, 80, 24)()

	started, ok := msg.(sshStartedMsg)
	if !ok {
		t.Fatalf("cmd() = %T, want sshStartedMsg", msg)
	}
	if started.err != nil {
		t.Fatalf("sshStartedMsg.err = %v, want nil", started.err)
	}
	if started.pane == nil {
		t.Fatal("sshStartedMsg.pane = nil, want a pane")
	}
	if len(sess.sizes) != 1 || sess.sizes[0] != [2]int{24, 80} {
		t.Errorf("Setsize calls = %v, want [[24 80]]", sess.sizes)
	}
	started.pane.close()
}

func TestStartSSHPaneCmdSurfacesSpawnError(t *testing.T) {
	spawner := fakePTYSpawner{err: errors.New("boom")}
	peer := tsnet.Peer{HostName: "bravo"}

	msg := startSSHPaneCmd(spawner, peer, 80, 24)()

	started, ok := msg.(sshStartedMsg)
	if !ok {
		t.Fatalf("cmd() = %T, want sshStartedMsg", msg)
	}
	if started.err == nil || started.pane != nil {
		t.Fatalf("sshStartedMsg = %+v, want non-nil err and nil pane", started)
	}
}

func TestSSHOutputMsgWritesIntoEmulator(t *testing.T) {
	pane := &sshPane{sess: newFakePTYSession(), term: vt.NewSafeEmulator(80, 24), output: make(chan []byte), done: make(chan struct{})}

	m := newTestModel()
	m.viewingSSH = true
	m.sshPane = pane

	updated, cmd := m.Update(sshOutputMsg{data: []byte("hello")})
	m = updated.(Model)
	if cmd == nil {
		t.Fatal("Update(sshOutputMsg) returned nil cmd, want waitForPTYOutput")
	}
	if !strings.Contains(m.sshPane.term.Render(), "hello") {
		t.Errorf("emulator content = %q, want it to contain %q", m.sshPane.term.Render(), "hello")
	}
}

func TestSSHClosedMsgReturnsToPeerTableAndClosesSession(t *testing.T) {
	sess := newFakePTYSession()
	pane := &sshPane{sess: sess, term: vt.NewSafeEmulator(80, 24), output: make(chan []byte), done: make(chan struct{})}

	m := newTestModel()
	m.viewingSSH = true
	m.sshPane = pane

	updated, cmd := m.Update(sshClosedMsg{})
	m = updated.(Model)

	if m.viewingSSH {
		t.Error("viewingSSH = true after sshClosedMsg, want false")
	}
	if m.sshPane != nil {
		t.Error("sshPane != nil after sshClosedMsg, want nil")
	}
	if cmd == nil {
		t.Fatal("Update(sshClosedMsg) returned nil cmd, want fetchCmd")
	}
	if !sess.closed {
		t.Error("session not closed after sshClosedMsg")
	}
}

func TestCtrlQDetachesAndClosesSession(t *testing.T) {
	sess := newFakePTYSession()
	pane := &sshPane{sess: sess, term: vt.NewSafeEmulator(80, 24), output: make(chan []byte), done: make(chan struct{})}

	m := newTestModel()
	m.viewingSSH = true
	m.sshPane = pane

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlQ})
	m = updated.(Model)

	if m.viewingSSH || m.sshPane != nil {
		t.Errorf("after ctrl+q: viewingSSH=%v sshPane=%v, want false/nil", m.viewingSSH, m.sshPane)
	}
	if !sess.closed {
		t.Error("session not closed after ctrl+q")
	}
	if cmd == nil {
		t.Fatal("Update(ctrl+q) returned nil cmd, want fetchCmd")
	}
}

func TestSSHPaneForwardsOtherKeysToSession(t *testing.T) {
	sess := newFakePTYSession()
	pane := &sshPane{sess: sess, term: vt.NewSafeEmulator(80, 24), output: make(chan []byte), done: make(chan struct{})}

	m := newTestModel()
	m.viewingSSH = true
	m.sshPane = pane

	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("l")})
	m.Update(tea.KeyMsg{Type: tea.KeyEnter})

	if len(sess.writes) != 2 || string(sess.writes[0]) != "l" || string(sess.writes[1]) != "\r" {
		t.Errorf("writes = %v, want [[l] [\\r]]", sess.writes)
	}
}

func TestSSHStartedMsgIgnoredIfDetachedBeforeSpawnCompleted(t *testing.T) {
	sess := newFakePTYSession()
	m := newTestModel()
	m.viewingSSH = false // already detached by the time this arrives

	updated, cmd := m.Update(sshStartedMsg{pane: &sshPane{sess: sess, term: vt.NewSafeEmulator(80, 24), output: make(chan []byte), done: make(chan struct{})}})
	m = updated.(Model)

	if m.sshPane != nil {
		t.Error("sshPane set after being ignored, want nil")
	}
	if cmd != nil {
		t.Error("cmd != nil, want nil")
	}
	if !sess.closed {
		t.Error("orphaned pane's session was not closed")
	}
}

func TestEnterOnOnlinePeerStartsSSHPane(t *testing.T) {
	m := newTestModel()
	m.spawner = fakePTYSpawner{sess: newFakePTYSession()}

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(Model)

	if !m.viewingSSH {
		t.Error("viewingSSH = false after enter on online peer, want true")
	}
	if cmd == nil {
		t.Fatal("Update(enter) returned nil cmd, want startSSHPaneCmd")
	}
}
```

- [ ] **Step 3: Run tests to verify they fail**

Run: `go test ./internal/ui/... -run 'TestStartSSHPaneCmd|TestSSHOutputMsg|TestSSHClosedMsg|TestCtrlQ|TestSSHPaneForwards|TestSSHStartedMsgIgnored|TestEnterOnOnlinePeer' -v`
Expected: FAIL to compile (`undefined: sshPane`, etc.)

- [ ] **Step 4: Write `internal/ui/sshpane.go`**

```go
package ui

import (
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

// close kills/reaps the underlying process and unblocks the read pump. Safe
// to call more than once (natural remote exit and manual detach can both
// reach it).
func (p *sshPane) close() {
	p.closeOnce.Do(func() {
		close(p.done)
		_ = p.sess.Close()
	})
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
type sshOutputMsg struct{ data []byte }

// sshClosedMsg is sent once the pty read loop ends, whether because the
// remote process exited or the session was closed locally. It carries no
// error: on Linux, reading a pty out from under an exited/closed process
// typically surfaces as EIO, which is the *normal* exit signal here, not a
// user-facing failure.
type sshClosedMsg struct{}

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

// waitForPTYOutput blocks for the next chunk (or close) from pane's read
// pump, translating it into a Bubble Tea message. Update re-issues this
// after every sshOutputMsg to keep draining the pump.
func waitForPTYOutput(pane *sshPane) tea.Cmd {
	return func() tea.Msg {
		data, ok := <-pane.output
		if !ok {
			return sshClosedMsg{}
		}
		return sshOutputMsg{data: data}
	}
}
```

- [ ] **Step 5: Delete `internal/ui/ssh.go`**

```bash
git rm internal/ui/ssh.go
```

- [ ] **Step 6: Wire `Model`**

In `internal/ui/model.go`:

Remove the now-undefined `sshFinishedMsg` case from `Update` (currently lines 146–150):

```go
	case sshFinishedMsg:
		if msg.err != nil {
			m.err = msg.err
		}
		return m, fetchCmd(m.fetcher)
```

Add fields to the `Model` struct (after the existing `viewingDERP` block, currently ending at line 51):

```go
	viewingSSH bool
	sshPane    *sshPane
	spawner    ptySpawner
```

Set the default spawner in `NewModel` (currently lines 57–67):

```go
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
```

Add three new top-level cases to `Update` (alongside the existing `derpReportMsg` case, currently lines 158–162):

```go
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
```

Add `m.viewingSSH` to the `tea.KeyMsg` dispatch switch (currently lines 164–174), alongside the existing `m.viewingDERP` case:

```go
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
```

Replace the `"enter"` case in `updateNormal` (currently lines 196–199):

```go
	case "enter":
		if peer, ok := m.selectedPeer(); ok && peer.Online {
			m.viewingSSH = true
			m.err = nil
			cols, rows := contentWidth(m.width), contentHeight(m.height)
			return m, startSSHPaneCmd(m.spawner, peer, cols, rows)
		}
```

Add a new `updateSSHPane` method, alongside the existing `updateDERPView` (after it, currently ending at line 227):

```go
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
```

- [ ] **Step 7: Update `internal/ui/model_test.go`**

`TestBuildSSHCommandPrefersDNSName` still passes unchanged (`buildSSHCommand` moved, not renamed). Append the new tests from Step 2 are in `sshpane_test.go`, not here — no further changes needed to `model_test.go` for this task.

- [ ] **Step 8: Run the full test suite**

Run: `go test ./... -v`
Expected: PASS (existing tests plus all new ones from Steps 2 and this task)

- [ ] **Step 9: Build check**

Run: `go build ./... && go vet ./...`
Expected: no errors

- [ ] **Step 10: Commit**

```bash
git add -A
git commit -m "feat: replace full-takeover ssh with an embedded pty-backed pane"
```

---

### Task 5: Render the pane in View()

**Files:**
- Modify: `internal/ui/sshpane.go`
- Modify: `internal/ui/model.go`
- Modify: `internal/ui/model_test.go`

**Interfaces:**
- Consumes: `sshPane.term.Render()` (Task 4), `helpStyle` (existing, `styles.go`).
- Produces: `func renderSSHPane(pane *sshPane) string`.

- [ ] **Step 1: Write the failing tests**

Add to `internal/ui/model_test.go`:

```go
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
```

Add `"github.com/charmbracelet/x/vt"` to `model_test.go`'s import block.

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/ui/... -run TestViewRendersSSHPane -v`
Expected: FAIL — `View()` still shows the peer table (`contains` assertions fail)

- [ ] **Step 3: Add `renderSSHPane` to `internal/ui/sshpane.go`**

```go
// renderSSHPane returns the frame body for the embedded ssh view: a
// connecting indicator before the pane is ready, otherwise the terminal
// emulator's current screen contents.
func renderSSHPane(pane *sshPane) string {
	if pane == nil {
		return helpStyle.Render("connecting…")
	}
	return pane.term.Render()
}
```

- [ ] **Step 4: Wire it into `View()`**

In `internal/ui/model.go`, add a case to `View`'s switch (currently lines 349–366), before `case m.pickingExitNode:`:

```go
	switch {
	case m.viewingSSH:
		body = renderSSHPane(m.sshPane)
		footer = helpStyle.Render("ctrl+q detach")
	case m.pickingExitNode:
		body = renderExitNodePicker(m.exitNodeCandidates(), m.exitNodeCursor, m.allowLANAccess, width)
		footer = helpStyle.Render("j/k move  enter select  l toggle LAN access  esc cancel")
```

(leave the rest of the switch as-is)

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test ./internal/ui/... -v`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add internal/ui/sshpane.go internal/ui/model.go internal/ui/model_test.go
git commit -m "feat: render the embedded ssh pane in View()"
```

---

### Task 6: Resize forwarding

**Files:**
- Modify: `internal/ui/model.go`
- Modify: `internal/ui/sshpane_test.go`

**Interfaces:**
- Consumes: `contentWidth`/`contentHeight` (existing/Task 2), `sshPane.term.Resize` and `sshPane.sess.Setsize` (Task 4).

- [ ] **Step 1: Write the failing test**

Add to `internal/ui/sshpane_test.go`:

```go
func TestWindowResizeWhileSSHActiveResizesPaneAndSession(t *testing.T) {
	sess := newFakePTYSession()
	pane := &sshPane{sess: sess, term: vt.NewSafeEmulator(80, 24), output: make(chan []byte), done: make(chan struct{})}

	m := newTestModel()
	m.viewingSSH = true
	m.sshPane = pane

	updated, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	m = updated.(Model)

	wantCols, wantRows := contentWidth(100), contentHeight(40)
	if m.sshPane.term.Width() != wantCols || m.sshPane.term.Height() != wantRows {
		t.Errorf("emulator size = %dx%d, want %dx%d", m.sshPane.term.Width(), m.sshPane.term.Height(), wantCols, wantRows)
	}
	if len(sess.sizes) != 1 || sess.sizes[0] != [2]int{wantRows, wantCols} {
		t.Errorf("Setsize calls = %v, want [[%d %d]]", sess.sizes, wantRows, wantCols)
	}
}

func TestWindowResizeWithNoActiveSSHPaneIsNoop(t *testing.T) {
	m := newTestModel()

	updated, cmd := m.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	m = updated.(Model)

	if m.width != 100 || m.height != 40 {
		t.Errorf("m.width/height = %d/%d, want 100/40", m.width, m.height)
	}
	if cmd != nil {
		t.Error("cmd != nil, want nil")
	}
}
```

- [ ] **Step 2: Run tests to verify the resize assertion fails**

Run: `go test ./internal/ui/... -run TestWindowResize -v`
Expected: `TestWindowResizeWhileSSHActiveResizesPaneAndSession` FAILs (emulator still 80x24, no `Setsize` calls); `TestWindowResizeWithNoActiveSSHPaneIsNoop` already passes (documents current behavior).

- [ ] **Step 3: Update the `tea.WindowSizeMsg` case in `internal/ui/model.go`**

Replace the existing case (currently lines 127–130):

```go
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		if m.viewingSSH && m.sshPane != nil {
			cols, rows := contentWidth(m.width), contentHeight(m.height)
			m.sshPane.term.Resize(cols, rows)
			_ = m.sshPane.sess.Setsize(rows, cols)
		}
		return m, nil
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/ui/... -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/ui/model.go internal/ui/sshpane_test.go
git commit -m "feat: forward terminal resizes to the active ssh pane"
```

---

### Task 7: Docs and manual verification

**Files:**
- Modify: `README.md`
- Modify: `ROADMAP.md`

- [ ] **Step 1: Update `README.md`**

Replace the `enter` row's description and add a short subsection. In the keybinding table:

```markdown
| `enter` | SSH into the selected online peer, embedded in the TUI |
```

Add a new subsection after "### Exit node picker" and before "### DERP latency matrix":

```markdown
### Embedded ssh session

Press `enter` on an online peer to ssh into it — the session runs live
inside the ts-hud frame (header/footer chrome stays put), not as a
separate full-terminal handoff.

| Key | Action |
|---|---|
| `ctrl+q` | Detach — ends the ssh session and returns to the peer table |

Detaching (or the remote session ending on its own) always terminates that
ssh connection; there's no background/reattach. Resizing your terminal
while connected resizes the remote session too.
```

- [ ] **Step 2: Update `ROADMAP.md`**

In the Phase 1 section, change:

```markdown
* **Seamless SSH Integration:** Execute SSH sessions directly into peers with automatic port/user detection.
```

to:

```markdown
* **Seamless SSH Integration:** Execute SSH sessions directly into peers with automatic port/user detection, running embedded live inside the TUI frame (not a full-terminal handoff).
```

- [ ] **Step 3: Commit docs**

```bash
git add README.md ROADMAP.md
git commit -m "docs: document the embedded ssh pane"
```

- [ ] **Step 4: Manual verification over tmux against the real tailnet**

This exercises the real `ssh` binary, a real pty, and a real remote shell — not mockable, and matches how the DERP latency matrix and exit-node picker were verified earlier in this project.

```bash
go build -o /tmp/ts-hud-verify ./...
tmux new-session -d -s sshpane -x 100 -y 30 "/tmp/ts-hud-verify"
sleep 1
tmux capture-pane -t sshpane -p   # confirm peer table renders inside the bordered frame
```

Then, from the pane's peer list, move to an online peer and connect:

```bash
tmux send-keys -t sshpane "j" ""      # or whatever navigation reaches an online peer
tmux send-keys -t sshpane "Enter"
sleep 2
tmux capture-pane -t sshpane -p       # expect: frame chrome intact, remote shell prompt visible in the body
```

Verify a round-trip through the remote shell:

```bash
tmux send-keys -t sshpane "echo hi-from-pane" "Enter"
sleep 1
tmux capture-pane -t sshpane -p       # expect: "hi-from-pane" visible inside the frame
```

Verify resize forwarding against a full-screen remote program:

```bash
tmux send-keys -t sshpane "htop" "Enter"   # or vim/less — anything that redraws on resize
sleep 1
tmux resize-window -t sshpane -x 130 -y 40
sleep 1
tmux capture-pane -t sshpane -p       # expect: htop's own UI has reflowed to the new size, no corruption
tmux send-keys -t sshpane "q"              # quit htop
```

Verify detach:

```bash
tmux send-keys -t sshpane C-q
sleep 1
tmux capture-pane -t sshpane -p       # expect: back at the peer table, frame intact
```

Verify natural remote exit also returns cleanly (connect again, then `exit` the remote shell instead of ctrl+q):

```bash
tmux send-keys -t sshpane "Enter"
sleep 2
tmux send-keys -t sshpane "exit" "Enter"
sleep 1
tmux capture-pane -t sshpane -p       # expect: back at the peer table, no error in the footer
```

Clean up:

```bash
tmux kill-session -t sshpane
```

If any step shows visual corruption (broken border, style bleeding from ssh output into the frame edges), style bleed is the known risk flagged in the design spec — capture the `-e -p` (ANSI-preserving) output and inspect with `cat -A` to diagnose before considering this task done.

- [ ] **Step 5: Report back**

Summarize what was verified (or any issue found) before moving on to release tagging.
