# Connection & Account Lifecycle Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Let a user bring the Tailscale connection up/down (`c`) and switch between already-authenticated accounts (`a`) without leaving ts-hud.

**Architecture:** `tsnet.Fetcher` gains three thin wrappers around already-vendored `*local.Client` calls: `SetWantRunning` (EditPrefs), `ListProfiles` (ProfileStatus), `SwitchProfile` (SwitchProfile). `Fetch` grows a return value so the daemon's `BackendState` reaches the header. `internal/ui` gains a header suffix for abnormal backend states, a `c`-triggered confirm-then-toggle flow that reuses the peer table as its body (footer-only prompt, no new overlay), and an `a`-triggered account-switch overlay structured exactly like the exit-node picker and preferences panel (cursor-navigable list, open→fetch→loading pattern).

**Tech Stack:** Go, Bubble Tea/lipgloss (existing), `tailscale.com/client/local`, `tailscale.com/ipn`.

## Global Constraints

- Scope is **up / down / switch only** — `login`/`logout` are out of scope for this plan.
- No new dependencies — `EditPrefs`, `ProfileStatus`, `SwitchProfile` are already-vendored parts of the `tailscale.com` module already in `go.mod`.
- No new lipgloss styles — reuse `headerStyle`, `rowStyle`, `selectedRowStyle`, `errorStyle`, `helpStyle` from `internal/ui/styles.go`.
- `Fetch`'s signature change (`internal/tsnet/client.go`) has exactly one call site (`fetchCmd` in `internal/ui/model.go`) and no direct unit test of its own — the change must land in the same commit as its call-site update so the repo stays buildable at every commit.
- No bespoke IPN-bus watch loop: after `SetWantRunning`, state updates arrive on the next periodic 5s refresh (`tickCmd`), not an immediate re-fetch. The one deliberate exception is the accounts panel, which explicitly re-fetches `ListProfiles` after a successful `SwitchProfile` to refresh its own `(current)` marker — that is a panel-local refresh, not a global state-watch mechanism.
- `up`/`switch` are applied immediately (no confirmation) — only `down` (`Running` → toggle off) asks for confirmation, since it interrupts existing connectivity.
- `SetWantRunning(false)` against a live daemon actually drops the Tailscale connection. If this machine is reached over Tailscale/SSH, running it can cut the very session used to test it — the manual-verification step (Task 6) must be run by a human on a machine where that's safe, never by an autonomous agent.

---

### Task 1: `tsnet` connection & account data layer

**Files:**
- Create: `internal/tsnet/connection.go`
- Modify: `internal/tsnet/client.go`
- Modify: `internal/tsnet/client_test.go`
- Create: `internal/tsnet/connection_test.go`

**Interfaces:**
- Produces: `func (f *Fetcher) SetWantRunning(ctx context.Context, running bool) (*ipn.Prefs, error)`, `func (f *Fetcher) ListProfiles(ctx context.Context) (current ipn.LoginProfile, all []ipn.LoginProfile, err error)`, `func (f *Fetcher) SwitchProfile(ctx context.Context, id ipn.ProfileID) error`.

- [ ] **Step 1: Write the failing test**

Create `internal/tsnet/connection_test.go`:

```go
package tsnet

import (
	"context"
	"errors"
	"testing"

	"tailscale.com/ipn"
)

func TestSetWantRunningTrue(t *testing.T) {
	fake := &fakeLocalClient{}
	f := &Fetcher{lc: fake}

	got, err := f.SetWantRunning(context.Background(), true)
	if err != nil {
		t.Fatalf("SetWantRunning(true) error = %v, want nil", err)
	}
	if !fake.gotMasked.WantRunningSet || !fake.gotMasked.Prefs.WantRunning {
		t.Errorf("gotMasked = %+v, want WantRunningSet=true, WantRunning=true", fake.gotMasked)
	}
	if got == nil || !got.WantRunning {
		t.Errorf("SetWantRunning(true) = %+v, want WantRunning=true", got)
	}
}

func TestSetWantRunningFalse(t *testing.T) {
	fake := &fakeLocalClient{}
	f := &Fetcher{lc: fake}

	_, err := f.SetWantRunning(context.Background(), false)
	if err != nil {
		t.Fatalf("SetWantRunning(false) error = %v, want nil", err)
	}
	if !fake.gotMasked.WantRunningSet || fake.gotMasked.Prefs.WantRunning {
		t.Errorf("gotMasked = %+v, want WantRunningSet=true, WantRunning=false", fake.gotMasked)
	}
}

func TestSetWantRunningPropagatesError(t *testing.T) {
	fake := &fakeLocalClient{editErr: errors.New("boom")}
	f := &Fetcher{lc: fake}

	if _, err := f.SetWantRunning(context.Background(), true); err == nil {
		t.Fatal("SetWantRunning() error = nil, want non-nil")
	}
}

func TestListProfiles(t *testing.T) {
	current := ipn.LoginProfile{ID: "1ab3", Name: "alice@example.com"}
	all := []ipn.LoginProfile{current, {ID: "9f2c", Name: "bob@example.com"}}
	fake := &fakeLocalClient{profileStatusCurrent: current, profileStatusAll: all}
	f := &Fetcher{lc: fake}

	gotCurrent, gotAll, err := f.ListProfiles(context.Background())
	if err != nil {
		t.Fatalf("ListProfiles() error = %v, want nil", err)
	}
	if gotCurrent.ID != current.ID {
		t.Errorf("ListProfiles() current = %+v, want %+v", gotCurrent, current)
	}
	if len(gotAll) != 2 {
		t.Errorf("ListProfiles() all = %+v, want 2 profiles", gotAll)
	}
}

func TestListProfilesPropagatesError(t *testing.T) {
	fake := &fakeLocalClient{profileStatusErr: errors.New("boom")}
	f := &Fetcher{lc: fake}

	if _, _, err := f.ListProfiles(context.Background()); err == nil {
		t.Fatal("ListProfiles() error = nil, want non-nil")
	}
}

func TestSwitchProfile(t *testing.T) {
	fake := &fakeLocalClient{}
	f := &Fetcher{lc: fake}
	id := ipn.ProfileID("9f2c")

	if err := f.SwitchProfile(context.Background(), id); err != nil {
		t.Fatalf("SwitchProfile() error = %v, want nil", err)
	}
	if fake.switchProfileGotID != id {
		t.Errorf("switchProfileGotID = %q, want %q", fake.switchProfileGotID, id)
	}
	if fake.switchProfileCallCount != 1 {
		t.Errorf("switchProfileCallCount = %d, want 1", fake.switchProfileCallCount)
	}
}

func TestSwitchProfilePropagatesError(t *testing.T) {
	fake := &fakeLocalClient{switchProfileErr: errors.New("boom")}
	f := &Fetcher{lc: fake}

	if err := f.SwitchProfile(context.Background(), "1ab3"); err == nil {
		t.Fatal("SwitchProfile() error = nil, want non-nil")
	}
}
```

- [ ] **Step 2: Run the test, verify it fails**

Run: `go test ./internal/tsnet/... -run "TestSetWantRunning|TestListProfiles|TestSwitchProfile" -v`
Expected: FAIL to compile — `fakeLocalClient` has no `profileStatusCurrent`/`profileStatusAll`/`profileStatusErr`/`switchProfileErr`/`switchProfileGotID`/`switchProfileCallCount` fields or `ProfileStatus`/`SwitchProfile` methods yet, and none of the `Fetcher` methods under test exist.

- [ ] **Step 3: Implement**

Create `internal/tsnet/connection.go`:

```go
package tsnet

import (
	"context"

	"tailscale.com/ipn"
)

// SetWantRunning brings the Tailscale connection up (running=true) or down
// (running=false).
func (f *Fetcher) SetWantRunning(ctx context.Context, running bool) (*ipn.Prefs, error) {
	return f.lc.EditPrefs(ctx, &ipn.MaskedPrefs{
		Prefs:          ipn.Prefs{WantRunning: running},
		WantRunningSet: true,
	})
}

// ListProfiles returns the currently active login profile and every
// profile already authenticated on this device.
func (f *Fetcher) ListProfiles(ctx context.Context) (current ipn.LoginProfile, all []ipn.LoginProfile, err error) {
	return f.lc.ProfileStatus(ctx)
}

// SwitchProfile switches the daemon to an already-authenticated profile.
func (f *Fetcher) SwitchProfile(ctx context.Context, id ipn.ProfileID) error {
	return f.lc.SwitchProfile(ctx, id)
}
```

In `internal/tsnet/client.go`, widen the `localClient` interface:

```go
type localClient interface {
	Status(ctx context.Context) (*ipnstate.Status, error)
	EditPrefs(ctx context.Context, mp *ipn.MaskedPrefs) (*ipn.Prefs, error)
	GetPrefs(ctx context.Context) (*ipn.Prefs, error)
	CurrentDERPMap(ctx context.Context) (*tailcfg.DERPMap, error)
	Ping(ctx context.Context, ip netip.Addr, pingtype tailcfg.PingType) (*ipnstate.PingResult, error)
	WhoIs(ctx context.Context, remoteAddr string) (*apitype.WhoIsResponse, error)
	ProfileStatus(ctx context.Context) (current ipn.LoginProfile, all []ipn.LoginProfile, err error)
	SwitchProfile(ctx context.Context, profile ipn.ProfileID) error
}
```

(`*local.Client` already implements both — pure additive interface surface, no other change to `client.go` in this task.)

In `internal/tsnet/client_test.go`, add fields to `fakeLocalClient` and two methods:

```go
type fakeLocalClient struct {
	editErr       error
	gotMasked     *ipn.MaskedPrefs
	editCallCount int
	derpMap       *tailcfg.DERPMap
	derpMapErr    error

	getPrefsResult *ipn.Prefs
	getPrefsErr    error

	pingResult  *ipnstate.PingResult
	pingErr     error
	whoIsResult *apitype.WhoIsResponse
	whoIsErr    error

	profileStatusCurrent ipn.LoginProfile
	profileStatusAll     []ipn.LoginProfile
	profileStatusErr     error

	switchProfileErr       error
	switchProfileGotID     ipn.ProfileID
	switchProfileCallCount int
}
```

```go
func (f *fakeLocalClient) ProfileStatus(ctx context.Context) (ipn.LoginProfile, []ipn.LoginProfile, error) {
	return f.profileStatusCurrent, f.profileStatusAll, f.profileStatusErr
}

func (f *fakeLocalClient) SwitchProfile(ctx context.Context, profile ipn.ProfileID) error {
	f.switchProfileGotID = profile
	f.switchProfileCallCount++
	return f.switchProfileErr
}
```

- [ ] **Step 4: Run the test, verify it passes**

Run: `go test ./internal/tsnet/... -v`
Expected: PASS, including all pre-existing `internal/tsnet` tests.

- [ ] **Step 5: Commit**

```bash
git add internal/tsnet/connection.go internal/tsnet/connection_test.go internal/tsnet/client.go internal/tsnet/client_test.go
git commit -m "feat: add Fetcher connection & account methods"
```

---

### Task 2: Surface backend state through `Fetch` and the header

**Files:**
- Modify: `internal/tsnet/client.go`
- Modify: `internal/ui/model.go`
- Modify: `internal/ui/model_test.go`

**Interfaces:**
- Consumes: nothing from Task 1.
- Produces: `func (f *Fetcher) Fetch(ctx context.Context) (peers []Peer, self *Peer, backendState string, err error)` (signature change — was `(peers []Peer, self *Peer, err error)`); `Model.backendState string` field; `peersMsg.backendState string` field.

This task changes an existing exported-package-internal signature and its one call site together so the build never breaks mid-task.

- [ ] **Step 1: Write the failing test**

In `internal/ui/model_test.go`, add:

```go
func TestRenderHeaderOmitsSuffixWhenRunning(t *testing.T) {
	m := newTestModel()
	self := tsnet.Peer{HostName: "laptop", IPs: []netip.Addr{netip.MustParseAddr("100.64.0.9")}}
	m.self = &self
	m.backendState = "Running"

	header := m.renderHeader()
	if contains(header, "RUNNING") {
		t.Errorf("renderHeader() = %q, want no state suffix when Running", header)
	}
}

func TestRenderHeaderOmitsSuffixWhenUnset(t *testing.T) {
	m := newTestModel()
	self := tsnet.Peer{HostName: "laptop", IPs: []netip.Addr{netip.MustParseAddr("100.64.0.9")}}
	m.self = &self

	header := m.renderHeader()
	if contains(header, "[") {
		t.Errorf("renderHeader() = %q, want no state suffix before the first fetch populates backendState", header)
	}
}

func TestRenderHeaderShowsSuffixWhenStopped(t *testing.T) {
	m := newTestModel()
	self := tsnet.Peer{HostName: "laptop", IPs: []netip.Addr{netip.MustParseAddr("100.64.0.9")}}
	m.self = &self
	m.backendState = "Stopped"

	header := m.renderHeader()
	if !contains(header, "[STOPPED]") {
		t.Errorf("renderHeader() = %q, want it to contain %q", header, "[STOPPED]")
	}
}

func TestPeersMsgPopulatesBackendState(t *testing.T) {
	m := newTestModel()
	updated, _ := m.Update(peersMsg{peers: testPeers(), backendState: "Stopped"})
	m = updated.(Model)

	if m.backendState != "Stopped" {
		t.Errorf("backendState = %q after peersMsg, want %q", m.backendState, "Stopped")
	}
}
```

- [ ] **Step 2: Run the test, verify it fails**

Run: `go test ./internal/ui/... -run "TestRenderHeader|TestPeersMsgPopulatesBackendState" -v`
Expected: FAIL to compile — `Model` has no `backendState` field and `peersMsg` has no `backendState` field yet.

- [ ] **Step 3: Implement**

In `internal/tsnet/client.go`, change `Fetch`:

```go
// Fetch returns the current peer list, the local node's own status, and the
// daemon's backend state (one of "NoState", "NeedsLogin",
// "NeedsMachineAuth", "Stopped", "Starting", "Running").
func (f *Fetcher) Fetch(ctx context.Context) ([]Peer, *Peer, string, error) {
	status, err := f.lc.Status(ctx)
	if err != nil {
		status, err = statusFromCLI(ctx)
		if err != nil {
			return nil, nil, "", fmt.Errorf("fetch tailscale status: %w", err)
		}
	}
	peers, self, err := peersFromStatus(status)
	if err != nil {
		return nil, nil, "", err
	}
	return peers, self, status.BackendState, nil
}
```

In `internal/ui/model.go`, add `"strings"` to the imports, and add a `backendState` field to `peersMsg`:

```go
type peersMsg struct {
	peers        []tsnet.Peer
	self         *tsnet.Peer
	backendState string
}
```

Add a `backendState string` field to `Model`, next to `self`:

```go
	peers        []tsnet.Peer
	filtered     []tsnet.Peer
	self         *tsnet.Peer
	backendState string
	cursor       int
	err          error
```

Update `fetchCmd`:

```go
func fetchCmd(fetcher *tsnet.Fetcher) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		peers, self, backendState, err := fetcher.Fetch(ctx)
		if err != nil {
			return errMsg{err: err}
		}
		return peersMsg{peers: peers, self: self, backendState: backendState}
	}
}
```

Update the `peersMsg` case in `Update`:

```go
	case peersMsg:
		m.err = nil
		m.peers = msg.peers
		m.self = msg.self
		m.backendState = msg.backendState
		m.applyFilter()
		return m, nil
```

Update `renderHeader`:

```go
func (m Model) renderHeader() string {
	title := headerStyle.Render("ts-hud")
	if m.self == nil {
		return title
	}
	ip := ""
	if len(m.self.IPs) > 0 {
		ip = m.self.IPs[0].String()
	}
	header := fmt.Sprintf("%s  self: %s (%s)  peers: %d", title, m.self.DisplayName(), ip, len(m.peers))
	if m.backendState != "" && m.backendState != "Running" {
		header += "  " + errorStyle.Render("["+strings.ToUpper(m.backendState)+"]")
	}
	return header
}
```

(`m.backendState == ""` is the pre-first-fetch zero value — no state is known yet, so no suffix is shown, matching the "Nothing is appended... in the common case" framing for the one state we can positively confirm is fine.)

- [ ] **Step 4: Run the test, verify it passes**

Run: `go test ./... -v`
Expected: PASS, including all pre-existing tests across both packages.

Then:

Run: `go build ./... && go vet ./... && gofmt -l .`
Expected: builds cleanly, `go vet` silent, `gofmt -l .` prints nothing.

- [ ] **Step 5: Commit**

```bash
git add internal/tsnet/client.go internal/ui/model.go internal/ui/model_test.go
git commit -m "feat: surface backend state through Fetch and the header"
```

---

### Task 3: Connection toggle (`c`)

**Files:**
- Modify: `internal/ui/model.go`
- Modify: `internal/ui/model_test.go`

**Interfaces:**
- Consumes: `tsnet.Fetcher.SetWantRunning` from Task 1; `Model.backendState` from Task 2.
- Produces: `Model.confirmingDown bool`, `Model.connLoading bool`, `Model.connErr error`; `connResultMsg{err error}`; `func setWantRunningCmd(fetcher *tsnet.Fetcher, running bool) tea.Cmd`; `func (m Model) updateConnConfirm(msg tea.KeyMsg) (tea.Model, tea.Cmd)`.

- [ ] **Step 1: Write the failing test**

In `internal/ui/model_test.go`, add:

```go
func TestConnToggleRunningAsksConfirmation(t *testing.T) {
	m := newTestModel()
	m.backendState = "Running"

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("c")})
	m = updated.(Model)

	if !m.confirmingDown {
		t.Fatal("confirmingDown = false after 'c' while Running, want true")
	}
	if cmd != nil {
		t.Error("Update('c') while Running returned a non-nil cmd, want nil until confirmed")
	}
	view := m.View()
	if !contains(view, "Bring Tailscale down?") {
		t.Errorf("View() = %q, want the confirmation prompt", view)
	}
}

func TestConnConfirmYesToggles(t *testing.T) {
	m := newTestModel()
	m.backendState = "Running"
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("c")})
	m = updated.(Model)

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("y")})
	m = updated.(Model)

	if m.confirmingDown {
		t.Error("confirmingDown = true after 'y', want false")
	}
	if !m.connLoading {
		t.Error("connLoading = false after 'y', want true")
	}
	if cmd == nil {
		t.Fatal("Update('y') while confirming returned nil cmd, want a SetWantRunning command")
	}
}

func TestConnConfirmNoCancels(t *testing.T) {
	m := newTestModel()
	m.backendState = "Running"
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("c")})
	m = updated.(Model)

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("n")})
	m = updated.(Model)

	if m.confirmingDown {
		t.Error("confirmingDown = true after 'n', want false")
	}
	if m.connLoading {
		t.Error("connLoading = true after 'n', want false — nothing should have been triggered")
	}
	if cmd != nil {
		t.Error("Update('n') while confirming returned a non-nil cmd, want nil")
	}
}

func TestConnConfirmEscCancels(t *testing.T) {
	m := newTestModel()
	m.backendState = "Running"
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("c")})
	m = updated.(Model)

	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = updated.(Model)

	if m.confirmingDown {
		t.Error("confirmingDown = true after esc, want false")
	}
}

func TestConnToggleStoppedAppliesImmediately(t *testing.T) {
	m := newTestModel()
	m.backendState = "Stopped"

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("c")})
	m = updated.(Model)

	if m.confirmingDown {
		t.Error("confirmingDown = true after 'c' while Stopped, want false — no confirmation for bringing the connection up")
	}
	if !m.connLoading {
		t.Error("connLoading = false after 'c' while Stopped, want true")
	}
	if cmd == nil {
		t.Fatal("Update('c') while Stopped returned nil cmd, want a SetWantRunning command")
	}
}

func TestConnToggleNeedsLoginSetsInlineError(t *testing.T) {
	m := newTestModel()
	m.backendState = "NeedsLogin"

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("c")})
	m = updated.(Model)

	if m.connErr == nil {
		t.Fatal("connErr = nil after 'c' while NeedsLogin, want an inline error")
	}
	if cmd != nil {
		t.Error("Update('c') while NeedsLogin returned a non-nil cmd, want nil — no EditPrefs call is possible")
	}
	view := m.View()
	if !contains(view, "not logged in") {
		t.Errorf("View() = %q, want the inline error rendered", view)
	}
}

func TestConnErrClearsOnNextKeypress(t *testing.T) {
	m := newTestModel()
	m.backendState = "NeedsLogin"
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("c")})
	m = updated.(Model)
	if m.connErr == nil {
		t.Fatal("connErr = nil after 'c' while NeedsLogin, want it set (test precondition)")
	}

	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	m = updated.(Model)

	if m.connErr != nil {
		t.Errorf("connErr = %v after the next keypress, want nil", m.connErr)
	}
}

func TestConnResultMsgClearsLoadingOnSuccess(t *testing.T) {
	m := newTestModel()
	m.connLoading = true

	updated, cmd := m.Update(connResultMsg{})
	m = updated.(Model)

	if m.connLoading {
		t.Error("connLoading = true after a successful connResultMsg, want false")
	}
	if cmd != nil {
		t.Error("Update(connResultMsg{}) returned a non-nil cmd, want nil — state arrives on the next periodic refresh")
	}
}

func TestConnResultMsgSetsErrorOnFailure(t *testing.T) {
	m := newTestModel()
	m.connLoading = true

	updated, _ := m.Update(connResultMsg{err: errors.New("boom")})
	m = updated.(Model)

	if m.connLoading {
		t.Error("connLoading = true after a failed connResultMsg, want false")
	}
	if m.connErr == nil {
		t.Fatal("connErr = nil after a failed connResultMsg, want it set")
	}
}
```

- [ ] **Step 2: Run the test, verify it fails**

Run: `go test ./internal/ui/... -run "TestConn" -v`
Expected: FAIL to compile — `confirmingDown`, `connLoading`, `connErr`, `connResultMsg` don't exist on `Model` yet.

- [ ] **Step 3: Wire it into Model**

Add a new message type after `prefsMsg`:

```go
type connResultMsg struct{ err error }
```

Add three fields to `Model`, after `prefsErr` and before `viewingSSH`:

```go
	confirmingDown bool
	connLoading    bool
	connErr        error
```

Add a command function after `setAdvertiseExitNodeCmd`:

```go
func setWantRunningCmd(fetcher *tsnet.Fetcher, running bool) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_, err := fetcher.SetWantRunning(ctx, running)
		return connResultMsg{err: err}
	}
}
```

Add a case to `Update`'s message switch, after `case prefsMsg:`'s block:

```go
	case connResultMsg:
		m.connLoading = false
		if msg.err != nil {
			m.connErr = msg.err
		}
		return m, nil
```

Add a case to the `tea.KeyMsg` dispatch switch inside `Update`, alongside `case m.viewingPrefs:`:

```go
		case m.confirmingDown:
			return m.updateConnConfirm(msg)
```

In `updateNormal`, clear `connErr` at the very top so any keypress dismisses a stale inline error (the same keypress that sets a new one runs after this line, so it isn't wiped out):

```go
func (m Model) updateNormal(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	m.connErr = nil
	switch msg.String() {
```

Add a case after `case "p":`'s block, before the closing of the switch:

```go
	case "c":
		if m.connLoading {
			return m, nil
		}
		switch m.backendState {
		case "Running":
			m.confirmingDown = true
		case "Stopped", "Starting":
			m.connLoading = true
			return m, setWantRunningCmd(m.fetcher, true)
		case "NeedsLogin", "NeedsMachineAuth", "NoState":
			m.connErr = fmt.Errorf("not logged in — run tailscale login")
		}
		return m, nil
```

(The zero-value `""` backend state — before the first fetch resolves — falls through with no case matched, i.e. a silent no-op; there is no safe action to take without knowing the real state yet.)

Add a new method after `updatePrefsView`:

```go
func (m Model) updateConnConfirm(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "y":
		m.confirmingDown = false
		m.connLoading = true
		return m, setWantRunningCmd(m.fetcher, false)
	case "n", "esc":
		m.confirmingDown = false
	}
	return m, nil
}
```

In `View()`, inside the `default:` branch's inner footer switch, add a case before `case m.searching:` so the confirmation prompt takes priority over the normal footer while the peer table stays as the body underneath:

```go
		switch {
		case m.confirmingDown:
			footer = errorStyle.Render("Bring Tailscale down? y confirm  n/esc cancel")
		case m.searching:
			footer = searchPromptStyle.Render("search: ") + m.searchInput.View()
		case m.err != nil:
			footer = errorStyle.Render("error: " + m.err.Error())
		case m.connErr != nil:
			footer = errorStyle.Render(m.connErr.Error())
		default:
			footer = helpStyle.Render("j/k move  g/G top/bottom  / search  enter ssh  x exit-node  d derp  i info  p prefs  c connection  r refresh  q quit")
		}
```

- [ ] **Step 4: Run the test, verify it passes**

Run: `go test ./... -v`
Expected: PASS, including all pre-existing tests.

Run: `go build ./... && go vet ./... && gofmt -l .`
Expected: builds cleanly, `go vet` silent, `gofmt -l .` prints nothing.

- [ ] **Step 5: Commit**

```bash
git add internal/ui/model.go internal/ui/model_test.go
git commit -m "feat: wire the connection up/down toggle into Model"
```

---

### Task 4: Account-switch overlay rendering

**Files:**
- Create: `internal/ui/accounts.go`
- Create: `internal/ui/accounts_test.go`

**Interfaces:**
- Produces: `func renderAccountsPanel(current ipn.LoginProfile, all []ipn.LoginProfile, cursor int, loading bool, accountsErr error, width int) string`.

- [ ] **Step 1: Write the failing test**

Create `internal/ui/accounts_test.go`:

```go
package ui

import (
	"errors"
	"strings"
	"testing"

	"tailscale.com/ipn"
)

func TestRenderAccountsPanelEmpty(t *testing.T) {
	got := renderAccountsPanel(ipn.LoginProfile{}, nil, 0, false, nil, 60)
	if !strings.Contains(got, "no accounts") {
		t.Errorf("renderAccountsPanel() = %q, want the empty-state message", got)
	}
}

func TestRenderAccountsPanelSingleProfile(t *testing.T) {
	p := ipn.LoginProfile{ID: "1ab3", Name: "alice@example.com"}
	got := renderAccountsPanel(p, []ipn.LoginProfile{p}, 0, false, nil, 60)
	if !strings.Contains(got, "alice@example.com") {
		t.Errorf("renderAccountsPanel() = %q, want it to contain the profile name", got)
	}
	if !strings.Contains(got, "(current)") {
		t.Errorf("renderAccountsPanel() = %q, want the current profile marked", got)
	}
}

func TestRenderAccountsPanelMultipleProfilesMarksCurrent(t *testing.T) {
	current := ipn.LoginProfile{ID: "1ab3", Name: "alice@example.com"}
	other := ipn.LoginProfile{ID: "9f2c", Name: "bob@example.com"}
	got := renderAccountsPanel(current, []ipn.LoginProfile{current, other}, 0, false, nil, 60)

	if !strings.Contains(got, "alice@example.com") || !strings.Contains(got, "bob@example.com") {
		t.Errorf("renderAccountsPanel() = %q, want both profile names", got)
	}
	aliceLine := lineContaining(got, "alice@example.com")
	bobLine := lineContaining(got, "bob@example.com")
	if !strings.Contains(aliceLine, "(current)") {
		t.Errorf("alice line = %q, want it marked (current)", aliceLine)
	}
	if strings.Contains(bobLine, "(current)") {
		t.Errorf("bob line = %q, want it NOT marked (current)", bobLine)
	}
}

func TestRenderAccountsPanelFallsBackToDisplayName(t *testing.T) {
	p := ipn.LoginProfile{ID: "1ab3", NetworkProfile: ipn.NetworkProfile{DisplayName: "Acme Tailnet"}}
	got := renderAccountsPanel(p, []ipn.LoginProfile{p}, 0, false, nil, 60)
	if !strings.Contains(got, "Acme Tailnet") {
		t.Errorf("renderAccountsPanel() = %q, want the NetworkProfile display name when Name is empty", got)
	}
}

func TestRenderAccountsPanelHighlightsCursorRow(t *testing.T) {
	all := []ipn.LoginProfile{{ID: "1ab3", Name: "alice"}, {ID: "9f2c", Name: "bob"}}
	got0 := renderAccountsPanel(all[0], all, 0, false, nil, 60)
	got1 := renderAccountsPanel(all[0], all, 1, false, nil, 60)
	if got0 == got1 {
		t.Error("renderAccountsPanel() output identical for cursor=0 and cursor=1, want the highlighted row to differ")
	}
}

func TestRenderAccountsPanelShowsLoadingState(t *testing.T) {
	got := renderAccountsPanel(ipn.LoginProfile{}, nil, 0, true, nil, 60)
	if !strings.Contains(got, "loading") {
		t.Errorf("renderAccountsPanel() = %q, want a loading message", got)
	}
}

func TestRenderAccountsPanelShowsErrorBeforeFirstFetch(t *testing.T) {
	got := renderAccountsPanel(ipn.LoginProfile{}, nil, 0, false, errors.New("list profiles failed"), 60)
	if !strings.Contains(got, "list profiles failed") {
		t.Errorf("renderAccountsPanel() = %q, want the error message", got)
	}
	if strings.Contains(got, "no accounts") {
		t.Errorf("renderAccountsPanel() = %q, want the error to replace the empty-state message, not both", got)
	}
}

func TestRenderAccountsPanelShowsErrorAlongsideLoadedList(t *testing.T) {
	p := ipn.LoginProfile{ID: "1ab3", Name: "alice@example.com"}
	got := renderAccountsPanel(p, []ipn.LoginProfile{p}, 0, false, errors.New("switch failed"), 60)
	if !strings.Contains(got, "alice@example.com") || !strings.Contains(got, "switch failed") {
		t.Errorf("renderAccountsPanel() = %q, want both the row list and the error", got)
	}
}

func lineContaining(s, substr string) string {
	for _, line := range strings.Split(s, "\n") {
		if strings.Contains(line, substr) {
			return line
		}
	}
	return ""
}
```

- [ ] **Step 2: Run the test, verify it fails**

Run: `go test ./internal/ui/... -run TestRenderAccountsPanel -v`
Expected: FAIL to compile — `renderAccountsPanel` doesn't exist yet.

- [ ] **Step 3: Implement**

Create `internal/ui/accounts.go`:

```go
package ui

import (
	"strings"

	"tailscale.com/ipn"
)

// profileName returns a profile's display label: its login name, falling
// back to the tailnet's display name when Name is empty (matching how
// `tailscale switch` lists profiles without a resolved login name yet).
func profileName(p ipn.LoginProfile) string {
	if p.Name != "" {
		return p.Name
	}
	return p.NetworkProfile.DisplayNameOrDefault()
}

// renderAccountsPanel shows the account-switch overlay opened via 'a': a
// cursor-navigable list of already-authenticated profiles, styled like the
// exit-node picker and preferences panel. Mirrors renderPrefsPanel's
// structure: an empty/no-baseline state (no profiles loaded yet, or the
// initial fetch failed) replaces the list entirely; once a list is loaded,
// a later error (e.g. a failed switch) is appended below it instead of
// blanking what's already on screen.
func renderAccountsPanel(current ipn.LoginProfile, all []ipn.LoginProfile, cursor int, loading bool, accountsErr error, width int) string {
	var b strings.Builder

	if loading {
		b.WriteString(helpStyle.Render("  loading accounts…"))
		return b.String()
	}
	if len(all) == 0 {
		msg := "no accounts — run tailscale login"
		style := helpStyle
		if accountsErr != nil {
			msg = accountsErr.Error()
			style = errorStyle
		}
		b.WriteString(style.Render("  " + msg))
		return b.String()
	}

	b.WriteString(headerStyle.Render("Switch account"))
	b.WriteString("\n")

	for i, p := range all {
		label := profileName(p)
		if p.ID == current.ID {
			label += "  (current)"
		}
		row := "  " + label
		style := rowStyle
		if i == cursor {
			row = fitWidth(row, width)
			style = selectedRowStyle
		}
		b.WriteString(style.Render(row))
		b.WriteString("\n")
	}

	if accountsErr != nil {
		b.WriteString("\n")
		b.WriteString(errorStyle.Render("  " + accountsErr.Error()))
	}

	return strings.TrimRight(b.String(), "\n")
}
```

- [ ] **Step 4: Run the test, verify it passes**

Run: `go test ./internal/ui/... -run TestRenderAccountsPanel -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/ui/accounts.go internal/ui/accounts_test.go
git commit -m "feat: render the account-switch overlay"
```

---

### Task 5: Account-switch overlay wiring

**Files:**
- Modify: `internal/ui/model.go`
- Modify: `internal/ui/model_test.go`

**Interfaces:**
- Consumes: `tsnet.Fetcher.ListProfiles`, `SwitchProfile` from Task 1; `renderAccountsPanel` from Task 4.
- Produces: `Model.viewingAccounts bool`, `accountsCursor int`, `accountsCurrent ipn.LoginProfile`, `accountsAll []ipn.LoginProfile`, `accountsLoading bool`, `accountsErr error`; `accountsMsg{current ipn.LoginProfile; all []ipn.LoginProfile; err error}`, `switchProfileMsg{err error}`; `func accountsFetchCmd(fetcher *tsnet.Fetcher) tea.Cmd`, `func switchProfileCmd(fetcher *tsnet.Fetcher, id ipn.ProfileID) tea.Cmd`.

- [ ] **Step 1: Write the failing test**

In `internal/ui/model_test.go`, add:

```go
func TestAccountsOpensAndFetches(t *testing.T) {
	m := newTestModel()

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a")})
	m = updated.(Model)

	if !m.viewingAccounts {
		t.Fatal("viewingAccounts = false after 'a', want true")
	}
	if !m.accountsLoading {
		t.Fatal("accountsLoading = false immediately after 'a', want true")
	}
	if cmd == nil {
		t.Fatal("Update('a') returned nil cmd, want an accounts-fetch command")
	}
}

func TestAccountsEscCloses(t *testing.T) {
	m := newTestModel()
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a")})
	m = updated.(Model)

	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = updated.(Model)

	if m.viewingAccounts {
		t.Fatal("viewingAccounts = true after esc, want false")
	}
}

func TestAccountsCursorMovesAndClamps(t *testing.T) {
	m := newTestModel()
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a")})
	m = updated.(Model)
	updated, _ = m.Update(accountsMsg{all: []ipn.LoginProfile{{ID: "1"}, {ID: "2"}}})
	m = updated.(Model)

	for i := 0; i < 5; i++ {
		updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
		m = updated.(Model)
	}
	if m.accountsCursor != 1 {
		t.Errorf("accountsCursor = %d after 5x 'j' over 2 profiles, want clamped to 1", m.accountsCursor)
	}

	for i := 0; i < 5; i++ {
		updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("k")})
		m = updated.(Model)
	}
	if m.accountsCursor != 0 {
		t.Errorf("accountsCursor = %d after 5x 'k', want clamped to 0", m.accountsCursor)
	}
}

func TestAccountsMsgPopulatesListAndClearsLoading(t *testing.T) {
	m := newTestModel()
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a")})
	m = updated.(Model)

	current := ipn.LoginProfile{ID: "1ab3", Name: "alice@example.com"}
	updated, _ = m.Update(accountsMsg{current: current, all: []ipn.LoginProfile{current}})
	m = updated.(Model)

	if m.accountsLoading {
		t.Fatal("accountsLoading = true after accountsMsg, want false")
	}
	view := m.View()
	if !contains(view, "alice@example.com") {
		t.Errorf("View() missing accounts panel\n---\n%s", view)
	}
}

func TestAccountsMsgErrorKeepsEmptyStateAndSetsError(t *testing.T) {
	m := newTestModel()
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a")})
	m = updated.(Model)

	updated, _ = m.Update(accountsMsg{err: errors.New("boom")})
	m = updated.(Model)

	if m.accountsErr == nil {
		t.Fatal("accountsErr = nil after accountsMsg with an error, want it set")
	}
	if len(m.accountsAll) != 0 {
		t.Errorf("accountsAll = %+v after a failed fetch, want empty", m.accountsAll)
	}
}

func TestAccountsEnterSwitchesProfile(t *testing.T) {
	m := newTestModel()
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a")})
	m = updated.(Model)
	all := []ipn.LoginProfile{{ID: "1ab3", Name: "alice"}, {ID: "9f2c", Name: "bob"}}
	updated, _ = m.Update(accountsMsg{current: all[0], all: all})
	m = updated.(Model)
	m.accountsCursor = 1

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(Model)

	if !m.accountsLoading {
		t.Fatal("accountsLoading = false after enter on a profile row, want true")
	}
	if cmd == nil {
		t.Fatal("Update(enter) in accounts view returned nil cmd, want a switch command")
	}
}

func TestSwitchProfileMsgSuccessRefetches(t *testing.T) {
	m := newTestModel()
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a")})
	m = updated.(Model)

	updated, cmd := m.Update(switchProfileMsg{})
	m = updated.(Model)

	if m.accountsErr != nil {
		t.Errorf("accountsErr = %v after a successful switch, want nil", m.accountsErr)
	}
	if !m.accountsLoading {
		t.Error("accountsLoading = false after a successful switch, want true — a follow-up fetch is in flight")
	}
	if cmd == nil {
		t.Fatal("Update(switchProfileMsg{}) returned nil cmd, want a follow-up accounts-fetch command")
	}
}

func TestSwitchProfileMsgFailureKeepsListAndSetsError(t *testing.T) {
	m := newTestModel()
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a")})
	m = updated.(Model)
	all := []ipn.LoginProfile{{ID: "1ab3", Name: "alice"}}
	updated, _ = m.Update(accountsMsg{current: all[0], all: all})
	m = updated.(Model)

	updated, cmd := m.Update(switchProfileMsg{err: errors.New("boom")})
	m = updated.(Model)

	if m.accountsErr == nil {
		t.Fatal("accountsErr = nil after a failed switch, want it set")
	}
	if len(m.accountsAll) != 1 {
		t.Errorf("accountsAll = %+v after a failed switch, want the last-loaded list preserved", m.accountsAll)
	}
	if cmd != nil {
		t.Error("Update(switchProfileMsg{err}) returned a non-nil cmd, want nil — no re-fetch after a failed switch")
	}
}
```

- [ ] **Step 2: Run the test, verify it fails**

Run: `go test ./internal/ui/... -run "TestAccounts|TestSwitchProfileMsg" -v`
Expected: FAIL to compile — `viewingAccounts`, `accountsCursor`, `accountsAll`, `accountsLoading`, `accountsErr`, `accountsMsg`, `switchProfileMsg` don't exist on `Model` yet.

- [ ] **Step 3: Wire it into Model**

Add two new message types after `connResultMsg`:

```go
type accountsMsg struct {
	current ipn.LoginProfile
	all     []ipn.LoginProfile
	err     error
}

type switchProfileMsg struct{ err error }
```

Add six fields to `Model`, after `connErr` and before `viewingSSH`:

```go
	viewingAccounts bool
	accountsCursor  int
	accountsCurrent ipn.LoginProfile
	accountsAll     []ipn.LoginProfile
	accountsLoading bool
	accountsErr     error
```

Add two command functions after `setWantRunningCmd`:

```go
func accountsFetchCmd(fetcher *tsnet.Fetcher) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		current, all, err := fetcher.ListProfiles(ctx)
		return accountsMsg{current: current, all: all, err: err}
	}
}

func switchProfileCmd(fetcher *tsnet.Fetcher, id ipn.ProfileID) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return switchProfileMsg{err: fetcher.SwitchProfile(ctx, id)}
	}
}
```

Add two cases to `Update`'s message switch, after `case connResultMsg:`'s block:

```go
	case accountsMsg:
		m.accountsLoading = false
		if msg.err != nil {
			m.accountsErr = msg.err
			return m, nil
		}
		m.accountsCurrent = msg.current
		m.accountsAll = msg.all
		m.accountsErr = nil
		return m, nil

	case switchProfileMsg:
		m.accountsLoading = false
		if msg.err != nil {
			m.accountsErr = msg.err
			return m, nil
		}
		m.accountsErr = nil
		m.accountsLoading = true
		return m, accountsFetchCmd(m.fetcher)
```

Add a case to the `tea.KeyMsg` dispatch switch inside `Update`, alongside `case m.confirmingDown:`:

```go
		case m.viewingAccounts:
			return m.updateAccountsView(msg)
```

In `updateNormal`, add a case after the `"c"` case's block, before the closing of the switch:

```go
	case "a":
		m.viewingAccounts = true
		m.accountsCursor = 0
		m.accountsLoading = true
		m.accountsErr = nil
		return m, accountsFetchCmd(m.fetcher)
```

Add two new methods after `updateConnConfirm`:

```go
func (m Model) updateAccountsView(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "a":
		m.viewingAccounts = false
		return m, nil
	case "j", "down":
		m.accountsCursor++
		m.clampAccountsCursor()
	case "k", "up":
		m.accountsCursor--
		m.clampAccountsCursor()
	case "enter":
		if len(m.accountsAll) == 0 {
			return m, nil
		}
		id := m.accountsAll[m.accountsCursor].ID
		m.accountsLoading = true
		return m, switchProfileCmd(m.fetcher, id)
	}
	return m, nil
}

func (m *Model) clampAccountsCursor() {
	if m.accountsCursor < 0 {
		m.accountsCursor = 0
	}
	if len(m.accountsAll) == 0 {
		m.accountsCursor = 0
		return
	}
	if m.accountsCursor > len(m.accountsAll)-1 {
		m.accountsCursor = len(m.accountsAll) - 1
	}
}
```

In `View()`, add a case alongside `case m.viewingPrefs:`:

```go
	case m.viewingAccounts:
		body = renderAccountsPanel(m.accountsCurrent, m.accountsAll, m.accountsCursor, m.accountsLoading, m.accountsErr, width)
		footer = helpStyle.Render("j/k move  enter switch  esc/a back")
```

Update the default footer's help text (set in Task 3) to also mention `a`:

```go
			footer = helpStyle.Render("j/k move  g/G top/bottom  / search  enter ssh  x exit-node  d derp  i info  p prefs  c connection  a accounts  r refresh  q quit")
```

- [ ] **Step 4: Run the test, verify it passes**

Run: `go test ./... -v`
Expected: PASS, including all pre-existing tests.

Run: `go build ./... && go vet ./... && gofmt -l .`
Expected: builds cleanly, `go vet` silent, `gofmt -l .` prints nothing.

- [ ] **Step 5: Commit**

```bash
git add internal/ui/model.go internal/ui/model_test.go
git commit -m "feat: wire the account-switch overlay into Model"
```

---

### Task 6: Docs and manual verification

**Files:**
- Modify: `README.md`

- [ ] **Step 1: Update the keybinding table**

In `README.md`, add two rows to the main keybinding table, after the `p` row:

```markdown
| `c` | Bring the Tailscale connection up, or down (with confirmation) |
| `a` | Open the account-switch overlay |
```

- [ ] **Step 2: Add subsections**

Add two new subsections after "### Preferences panel" and before "## Flags":

```markdown
### Connection toggle

Press `c` to bring the Tailscale connection up or down. Bringing it up
(from `Stopped`/`Starting`) applies immediately, matching the preferences
panel's immediate-apply behavior. Bringing it down (from `Running`) asks
for confirmation first, since it interrupts existing connectivity. If the
daemon isn't logged in (`NeedsLogin`/`NeedsMachineAuth`/`NoState`), `c`
shows an inline error instead — there's no node key to bring up.

When the connection isn't `Running`, the header shows the backend state in
brackets, e.g. `[STOPPED]`, so you can tell the peer list below might be
stale.

| Key | Action |
|---|---|
| `y` | Confirm bringing the connection down |
| `n` / `esc` | Cancel |

### Account switch

Press `a` to switch between Tailscale accounts already authenticated on
this device (via a prior `tailscale login`). Adding a brand-new account
isn't supported here — see `tailscale login` for that.

| Key | Action |
|---|---|
| `j` / `k` | Move down / up |
| `enter` | Switch to the highlighted account |
| `esc` / `a` | Back to the peer table |
```

- [ ] **Step 3: Commit**

```bash
git add README.md
git commit -m "docs: document the connection toggle and account switch"
```

- [ ] **Step 4: Manual verification (human only — do not run this step as an autonomous agent)**

`SetWantRunning(false)` actually drops the Tailscale connection on the
machine it runs on. If this machine is reached over Tailscale/SSH, that
can cut the very session used to test it. A human should run this on a
machine where losing connectivity briefly is safe (e.g. sitting at the
physical console), not delegate it.

```bash
go build -o /tmp/ts-hud-verify .
/tmp/ts-hud-verify
```

Test `a` first (fully reversible): press `a`, confirm the profile list
shows real accounts with `(current)` on the right one, move the cursor,
press `esc` to back out without switching (or press `enter` on a
different profile only if you're prepared to be logged into that account
afterward).

Then test `c`: note the current header (should show no `[STATE]` suffix
if `Running`). Press `c` — expect the confirmation footer prompt. Press
`n` to cancel and confirm the header/state is unchanged. Only then, if
you're comfortable briefly losing connectivity, press `c` then `y` to
actually bring it down, confirm the header now shows `[STOPPED]` within
~5s, then press `c` again to bring it back up and confirm `[STOPPED]`
disappears once `Running` is reported again.

- [ ] **Step 5: Report back**

Summarize what was observed for both `a` (profile list contents, whether
`(current)` was correctly marked) and `c` (confirmation prompt text,
header state transitions), and confirm no unintended account switch or
lasting disconnection was left in place.
