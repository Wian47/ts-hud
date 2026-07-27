package ui

import (
	"errors"
	"io"
	"net/netip"
	"os/exec"
	"strings"
	"sync"
	"testing"
	"time"

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

// fakePTYSession is written to from the Bubble Tea update goroutine *and*
// from the pane's background reply-drain goroutine, so every field it
// records is mutex-guarded and read back through accessors.
type fakePTYSession struct {
	readCh  chan []byte
	closeCh chan struct{}

	mu     sync.Mutex
	closed bool
	writes [][]byte
	sizes  [][2]int
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
	f.mu.Lock()
	defer f.mu.Unlock()
	f.writes = append(f.writes, cp)
	return len(p), nil
}

func (f *fakePTYSession) Close() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if !f.closed {
		f.closed = true
		close(f.closeCh)
	}
	return nil
}

func (f *fakePTYSession) Setsize(rows, cols int) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sizes = append(f.sizes, [2]int{rows, cols})
	return nil
}

func (f *fakePTYSession) isClosed() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.closed
}

// writtenChunks returns a copy of every chunk written to the session, in
// order.
func (f *fakePTYSession) writtenChunks() [][]byte {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([][]byte, len(f.writes))
	copy(out, f.writes)
	return out
}

// written returns every byte written to the session, concatenated.
func (f *fakePTYSession) written() string {
	var b strings.Builder
	for _, chunk := range f.writtenChunks() {
		b.Write(chunk)
	}
	return b.String()
}

func (f *fakePTYSession) sizeCalls() [][2]int {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([][2]int, len(f.sizes))
	copy(out, f.sizes)
	return out
}

// newTestSSHPane builds a pane the way the production code does, minus the
// background goroutines, for tests that drive Update directly.
func newTestSSHPane(sess ptySession) *sshPane {
	return &sshPane{
		sess:   sess,
		term:   vt.NewSafeEmulator(80, 24),
		output: make(chan []byte),
		done:   make(chan struct{}),
	}
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
	if sizes := sess.sizeCalls(); len(sizes) != 1 || sizes[0] != [2]int{24, 80} {
		t.Errorf("Setsize calls = %v, want [[24 80]]", sizes)
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
	pane := newTestSSHPane(newFakePTYSession())

	m := newTestModel()
	m.viewingSSH = true
	m.sshPane = pane

	updated, cmd := m.Update(sshOutputMsg{pane: pane, data: []byte("hello")})
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
	pane := newTestSSHPane(sess)

	m := newTestModel()
	m.viewingSSH = true
	m.sshPane = pane

	updated, cmd := m.Update(sshClosedMsg{pane: pane})
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
	if !sess.isClosed() {
		t.Error("session not closed after sshClosedMsg")
	}
}

func TestCtrlQDetachesAndClosesSession(t *testing.T) {
	sess := newFakePTYSession()
	pane := newTestSSHPane(sess)

	m := newTestModel()
	m.viewingSSH = true
	m.sshPane = pane

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlQ})
	m = updated.(Model)

	if m.viewingSSH || m.sshPane != nil {
		t.Errorf("after ctrl+q: viewingSSH=%v sshPane=%v, want false/nil", m.viewingSSH, m.sshPane)
	}
	if !sess.isClosed() {
		t.Error("session not closed after ctrl+q")
	}
	if cmd == nil {
		t.Fatal("Update(ctrl+q) returned nil cmd, want fetchCmd")
	}
}

func TestSSHPaneForwardsOtherKeysToSession(t *testing.T) {
	sess := newFakePTYSession()
	pane := newTestSSHPane(sess)

	m := newTestModel()
	m.viewingSSH = true
	m.sshPane = pane

	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("l")})
	m.Update(tea.KeyMsg{Type: tea.KeyEnter})

	if writes := sess.writtenChunks(); len(writes) != 2 || string(writes[0]) != "l" || string(writes[1]) != "\r" {
		t.Errorf("writes = %v, want [[l] [\\r]]", writes)
	}
}

func TestSSHStartedMsgIgnoredIfDetachedBeforeSpawnCompleted(t *testing.T) {
	sess := newFakePTYSession()
	m := newTestModel()
	m.viewingSSH = false // already detached by the time this arrives

	updated, cmd := m.Update(sshStartedMsg{pane: newTestSSHPane(sess)})
	m = updated.(Model)

	if m.sshPane != nil {
		t.Error("sshPane set after being ignored, want nil")
	}
	if cmd != nil {
		t.Error("cmd != nil, want nil")
	}
	if !sess.isClosed() {
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

func TestWindowResizeWhileSSHActiveResizesPaneAndSession(t *testing.T) {
	sess := newFakePTYSession()
	pane := newTestSSHPane(sess)

	m := newTestModel()
	m.viewingSSH = true
	m.sshPane = pane

	updated, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	m = updated.(Model)

	wantCols, wantRows := contentWidth(100), contentHeight(40)
	if m.sshPane.term.Width() != wantCols || m.sshPane.term.Height() != wantRows {
		t.Errorf("emulator size = %dx%d, want %dx%d", m.sshPane.term.Width(), m.sshPane.term.Height(), wantCols, wantRows)
	}
	if sizes := sess.sizeCalls(); len(sizes) != 1 || sizes[0] != [2]int{wantRows, wantCols} {
		t.Errorf("Setsize calls = %v, want [[%d %d]]", sizes, wantRows, wantCols)
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

// TestSSHOutputWithDeviceQueryDoesNotDeadlock guards the emulator's reply
// pipe. vt writes terminal replies (DA1/DA2/CPR/OSC answers) into an
// unbuffered io.Pipe from inside Write(), so if nobody drains that pipe the
// first query sequence a remote program emits blocks Write() forever — on
// the Bubble Tea update goroutine, while holding the SafeEmulator's write
// lock, which freezes View() too. The whole TUI would hang with no way out.
// The bounded timeout keeps a regression from hanging `go test` itself.
func TestSSHOutputWithDeviceQueryDoesNotDeadlock(t *testing.T) {
	sess := newFakePTYSession()
	msg := startSSHPaneCmd(fakePTYSpawner{sess: sess}, tsnet.Peer{HostName: "bravo"}, 80, 24)()
	started, ok := msg.(sshStartedMsg)
	if !ok || started.pane == nil {
		t.Fatalf("startSSHPaneCmd() = %#v, want a ready pane", msg)
	}
	pane := started.pane
	defer pane.close()

	m := newTestModel()
	m.viewingSSH = true
	m.sshPane = pane

	returned := make(chan struct{})
	go func() {
		// ESC [ c is DA1, a Primary Device Attributes query. Real programs
		// (vim, shell prompts, capability probes) send these constantly.
		m.Update(sshOutputMsg{pane: pane, data: []byte("\x1b[c")})
		close(returned)
	}()

	select {
	case <-returned:
	case <-time.After(2 * time.Second):
		t.Fatal("Update(sshOutputMsg) with a DA1 query did not return within 2s: the emulator's reply pipe is not being drained, so Write() deadlocked")
	}

	// Not hanging isn't enough: the reply has to reach the remote program
	// that asked for it, i.e. be written back into the pty.
	wantReply := "\x1b[?62;1;6;22c"
	deadline := time.Now().Add(2 * time.Second)
	for {
		if got := sess.written(); strings.Contains(got, wantReply) {
			break
		} else if time.Now().After(deadline) {
			t.Fatalf("pty writes = %q, want them to contain the DA1 reply %q", got, wantReply)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// TestClosingPaneStopsReplyDrain checks close() unblocks the reply-drain
// goroutine (via the emulator's Close) instead of leaking it for the
// lifetime of the process.
func TestClosingPaneStopsReplyDrain(t *testing.T) {
	sess := newFakePTYSession()
	msg := startSSHPaneCmd(fakePTYSpawner{sess: sess}, tsnet.Peer{HostName: "bravo"}, 80, 24)()
	pane := msg.(sshStartedMsg).pane

	pane.close()

	// After close, the emulator's reply side is closed, so a Read returns an
	// error rather than blocking forever — which is exactly what lets the
	// io.Copy in startSSHPaneCmd return.
	done := make(chan error, 1)
	go func() {
		_, err := pane.term.Read(make([]byte, 8))
		done <- err
	}()
	select {
	case err := <-done:
		if err == nil {
			t.Error("term.Read after close() succeeded, want an error")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("term.Read still blocking 2s after close(): the reply-drain goroutine would leak")
	}
}

func TestStaleSSHOutputMsgFromReplacedPaneIsIgnored(t *testing.T) {
	current := newTestSSHPane(newFakePTYSession())
	stale := newTestSSHPane(newFakePTYSession())

	m := newTestModel()
	m.viewingSSH = true
	m.sshPane = current

	updated, cmd := m.Update(sshOutputMsg{pane: stale, data: []byte("hello")})
	m = updated.(Model)

	if cmd != nil {
		t.Error("cmd != nil for a stale sshOutputMsg, want nil (no re-arming waitForPTYOutput on a dead pane)")
	}
	if m.sshPane != current {
		t.Error("m.sshPane changed on a stale sshOutputMsg, want it untouched")
	}
	if got := current.term.Render(); strings.Contains(got, "hello") {
		t.Errorf("current pane emulator = %q, want the stale pane's output kept out of it", got)
	}
}

func TestStaleSSHClosedMsgFromReplacedPaneIsIgnored(t *testing.T) {
	sess := newFakePTYSession()
	current := newTestSSHPane(sess)
	stale := newTestSSHPane(newFakePTYSession())

	m := newTestModel()
	m.viewingSSH = true
	m.sshPane = current

	updated, cmd := m.Update(sshClosedMsg{pane: stale})
	m = updated.(Model)

	if cmd != nil {
		t.Error("cmd != nil for a stale sshClosedMsg, want nil")
	}
	if !m.viewingSSH {
		t.Error("viewingSSH = false after a stale sshClosedMsg, want the live session still shown")
	}
	if m.sshPane != current {
		t.Error("m.sshPane cleared by a stale sshClosedMsg, want it untouched")
	}
	if sess.isClosed() {
		t.Error("live session closed by a stale sshClosedMsg")
	}
}

func TestSSHStartedMsgClosesSpawnWhenAPaneIsAlreadyActive(t *testing.T) {
	liveSess := newFakePTYSession()
	live := newTestSSHPane(liveSess)
	lateSess := newFakePTYSession()

	m := newTestModel()
	m.viewingSSH = true
	m.sshPane = live

	updated, cmd := m.Update(sshStartedMsg{pane: newTestSSHPane(lateSess)})
	m = updated.(Model)

	if m.sshPane != live {
		t.Error("m.sshPane overwritten by a second spawn result, want the already-active pane kept")
	}
	if cmd != nil {
		t.Error("cmd != nil, want nil")
	}
	if !lateSess.isClosed() {
		t.Error("the redundant spawn's session was not closed, leaking a process and pty")
	}
	if liveSess.isClosed() {
		t.Error("the already-active session was closed, want it left running")
	}
}
