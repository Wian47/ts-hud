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
