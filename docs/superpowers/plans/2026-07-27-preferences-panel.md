# Preferences Panel Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Let a user view and flip the 5 `tailscale set` preferences they use day-to-day (SSH server, shields up, accept routes, accept DNS, advertise as exit node) from a `p` overlay, without leaving ts-hud.

**Architecture:** `tsnet.Fetcher` gains `GetPrefs` plus 5 typed setters, all thin wrappers around the already-vendored `*local.Client.EditPrefs`/`GetPrefs` — pure additive interface surface, no existing signature changes. `internal/ui` gains a new overlay, `viewingPrefs`, styled and structured like the exit-node picker (cursor-navigable list, `rowStyle`/`selectedRowStyle` highlighting) with the DERP/peer-detail views' open→fetch→loading pattern.

**Tech Stack:** Go, Bubble Tea/lipgloss (existing), `tailscale.com/client/local`, `tailscale.com/ipn`, `tailscale.com/net/tsaddr`, `tailscale.com/types/views`.

## Global Constraints

- No new dependencies — `GetPrefs`/`EditPrefs` and the `tsaddr`/`views` helpers are already-vendored parts of the `tailscale.com` module already in `go.mod`.
- No new lipgloss styles — reuse `headerStyle`, `rowStyle`, `selectedRowStyle`, `onlineStyle`, `offlineStyle`, `errorStyle`, `helpStyle` from `internal/ui/styles.go`.
- Every toggle applies immediately via `EditPrefs` — no staged/save step, matching the exit-node picker's existing behavior.
- The panel always renders whatever `*ipn.Prefs` the daemon actually returned from `EditPrefs`/`GetPrefs`, never a locally-guessed post-toggle state.
- A failed toggle must not blank an already-loaded panel: on a `prefsMsg` with a non-nil `err`, only `prefsErr` is set — `m.prefs` is left untouched.
- Row order is fixed and defined in exactly one place, `prefRows()`: SSH server, Shields up, Accept routes, Accept DNS, Advertise as exit node (indices 0-4). `updatePrefsView`'s cursor→setter dispatch must stay in that same order.

---

### Task 1: `tsnet` preferences data layer

**Files:**
- Create: `internal/tsnet/prefs.go`
- Modify: `internal/tsnet/client.go`
- Modify: `internal/tsnet/client_test.go`
- Create: `internal/tsnet/prefs_test.go`

**Interfaces:**
- Produces: `func (f *Fetcher) GetPrefs(ctx context.Context) (*ipn.Prefs, error)`, `func (f *Fetcher) SetRunSSH(ctx context.Context, run bool) (*ipn.Prefs, error)`, `func (f *Fetcher) SetShieldsUp(ctx context.Context, up bool) (*ipn.Prefs, error)`, `func (f *Fetcher) SetAcceptRoutes(ctx context.Context, accept bool) (*ipn.Prefs, error)`, `func (f *Fetcher) SetAcceptDNS(ctx context.Context, accept bool) (*ipn.Prefs, error)`, `func (f *Fetcher) SetAdvertiseExitNode(ctx context.Context, advertise bool) (*ipn.Prefs, error)`.

- [ ] **Step 1: Write the failing test**

Create `internal/tsnet/prefs_test.go`:

```go
package tsnet

import (
	"context"
	"errors"
	"net/netip"
	"testing"

	"tailscale.com/ipn"
	"tailscale.com/net/tsaddr"
)

func TestGetPrefs(t *testing.T) {
	want := &ipn.Prefs{RunSSH: true}
	fake := &fakeLocalClient{getPrefsResult: want}
	f := &Fetcher{lc: fake}

	got, err := f.GetPrefs(context.Background())
	if err != nil {
		t.Fatalf("GetPrefs() error = %v, want nil", err)
	}
	if got != want {
		t.Errorf("GetPrefs() = %p, want %p", got, want)
	}
}

func TestSetRunSSH(t *testing.T) {
	fake := &fakeLocalClient{}
	f := &Fetcher{lc: fake}

	got, err := f.SetRunSSH(context.Background(), true)
	if err != nil {
		t.Fatalf("SetRunSSH() error = %v, want nil", err)
	}
	if !fake.gotMasked.RunSSHSet || !fake.gotMasked.Prefs.RunSSH {
		t.Errorf("gotMasked = %+v, want RunSSHSet=true, RunSSH=true", fake.gotMasked)
	}
	if got == nil || !got.RunSSH {
		t.Errorf("SetRunSSH() = %+v, want RunSSH=true", got)
	}
}

func TestSetShieldsUp(t *testing.T) {
	fake := &fakeLocalClient{}
	f := &Fetcher{lc: fake}

	_, err := f.SetShieldsUp(context.Background(), true)
	if err != nil {
		t.Fatalf("SetShieldsUp() error = %v, want nil", err)
	}
	if !fake.gotMasked.ShieldsUpSet || !fake.gotMasked.Prefs.ShieldsUp {
		t.Errorf("gotMasked = %+v, want ShieldsUpSet=true, ShieldsUp=true", fake.gotMasked)
	}
}

func TestSetAcceptRoutes(t *testing.T) {
	fake := &fakeLocalClient{}
	f := &Fetcher{lc: fake}

	_, err := f.SetAcceptRoutes(context.Background(), true)
	if err != nil {
		t.Fatalf("SetAcceptRoutes() error = %v, want nil", err)
	}
	if !fake.gotMasked.RouteAllSet || !fake.gotMasked.Prefs.RouteAll {
		t.Errorf("gotMasked = %+v, want RouteAllSet=true, RouteAll=true", fake.gotMasked)
	}
}

func TestSetAcceptDNS(t *testing.T) {
	fake := &fakeLocalClient{}
	f := &Fetcher{lc: fake}

	_, err := f.SetAcceptDNS(context.Background(), false)
	if err != nil {
		t.Fatalf("SetAcceptDNS() error = %v, want nil", err)
	}
	if !fake.gotMasked.CorpDNSSet || fake.gotMasked.Prefs.CorpDNS {
		t.Errorf("gotMasked = %+v, want CorpDNSSet=true, CorpDNS=false", fake.gotMasked)
	}
}

func TestSetAdvertiseExitNodeTurnsOnFromEmpty(t *testing.T) {
	fake := &fakeLocalClient{getPrefsResult: &ipn.Prefs{}}
	f := &Fetcher{lc: fake}

	got, err := f.SetAdvertiseExitNode(context.Background(), true)
	if err != nil {
		t.Fatalf("SetAdvertiseExitNode() error = %v, want nil", err)
	}
	if !fake.gotMasked.AdvertiseRoutesSet {
		t.Fatal("gotMasked.AdvertiseRoutesSet = false, want true")
	}
	if len(fake.gotMasked.Prefs.AdvertiseRoutes) != 2 {
		t.Fatalf("AdvertiseRoutes = %v, want 2 routes (all-v4, all-v6)", fake.gotMasked.Prefs.AdvertiseRoutes)
	}
	if got == nil || !got.AdvertisesExitNode() {
		t.Errorf("SetAdvertiseExitNode() = %+v, want AdvertisesExitNode() true", got)
	}
}

func TestSetAdvertiseExitNodePreservesSubnetRoutesWhenTurningOn(t *testing.T) {
	subnet := netip.MustParsePrefix("10.0.0.0/8")
	fake := &fakeLocalClient{getPrefsResult: &ipn.Prefs{AdvertiseRoutes: []netip.Prefix{subnet}}}
	f := &Fetcher{lc: fake}

	_, err := f.SetAdvertiseExitNode(context.Background(), true)
	if err != nil {
		t.Fatalf("SetAdvertiseExitNode() error = %v, want nil", err)
	}
	routes := fake.gotMasked.Prefs.AdvertiseRoutes
	if len(routes) != 3 {
		t.Fatalf("AdvertiseRoutes = %v, want 3 routes (subnet + all-v4 + all-v6)", routes)
	}
	found := false
	for _, r := range routes {
		if r == subnet {
			found = true
		}
	}
	if !found {
		t.Errorf("AdvertiseRoutes = %v, want to still contain %v", routes, subnet)
	}
}

func TestSetAdvertiseExitNodeTurnsOff(t *testing.T) {
	subnet := netip.MustParsePrefix("10.0.0.0/8")
	fake := &fakeLocalClient{getPrefsResult: &ipn.Prefs{
		AdvertiseRoutes: []netip.Prefix{subnet, tsaddr.AllIPv4(), tsaddr.AllIPv6()},
	}}
	f := &Fetcher{lc: fake}

	_, err := f.SetAdvertiseExitNode(context.Background(), false)
	if err != nil {
		t.Fatalf("SetAdvertiseExitNode() error = %v, want nil", err)
	}
	routes := fake.gotMasked.Prefs.AdvertiseRoutes
	if len(routes) != 1 || routes[0] != subnet {
		t.Errorf("AdvertiseRoutes = %v, want only [%v]", routes, subnet)
	}
}

func TestSetAdvertiseExitNodeNoopWhenAlreadyDesiredState(t *testing.T) {
	fake := &fakeLocalClient{getPrefsResult: &ipn.Prefs{}}
	f := &Fetcher{lc: fake}

	got, err := f.SetAdvertiseExitNode(context.Background(), false)
	if err != nil {
		t.Fatalf("SetAdvertiseExitNode() error = %v, want nil", err)
	}
	if fake.editCallCount != 0 {
		t.Errorf("editCallCount = %d, want 0 (already false, should not call EditPrefs)", fake.editCallCount)
	}
	if got != fake.getPrefsResult {
		t.Errorf("SetAdvertiseExitNode() = %p, want the same *ipn.Prefs returned by GetPrefs (%p)", got, fake.getPrefsResult)
	}
}

func TestSetAdvertiseExitNodePropagatesGetPrefsError(t *testing.T) {
	fake := &fakeLocalClient{getPrefsErr: errors.New("get prefs failed")}
	f := &Fetcher{lc: fake}

	_, err := f.SetAdvertiseExitNode(context.Background(), true)
	if err == nil {
		t.Fatal("SetAdvertiseExitNode() error = nil, want an error")
	}
}
```

- [ ] **Step 2: Run the test, verify it fails**

Run: `go test ./internal/tsnet/... -run "TestGetPrefs|TestSetRunSSH|TestSetShieldsUp|TestSetAcceptRoutes|TestSetAcceptDNS|TestSetAdvertiseExitNode" -v`
Expected: FAIL to compile — `fakeLocalClient` has no `getPrefsResult`/`getPrefsErr`/`editCallCount` fields or `GetPrefs` method yet, and none of the `Fetcher` methods under test exist.

- [ ] **Step 3: Implement**

Create `internal/tsnet/prefs.go`:

```go
package tsnet

import (
	"context"
	"net/netip"

	"tailscale.com/ipn"
	"tailscale.com/net/tsaddr"
	"tailscale.com/types/views"
)

// GetPrefs returns the daemon's current preferences.
func (f *Fetcher) GetPrefs(ctx context.Context) (*ipn.Prefs, error) {
	return f.lc.GetPrefs(ctx)
}

// SetRunSSH toggles whether this node runs a Tailscale SSH server.
func (f *Fetcher) SetRunSSH(ctx context.Context, run bool) (*ipn.Prefs, error) {
	return f.lc.EditPrefs(ctx, &ipn.MaskedPrefs{
		Prefs:     ipn.Prefs{RunSSH: run},
		RunSSHSet: true,
	})
}

// SetShieldsUp toggles whether incoming connections are blocked.
func (f *Fetcher) SetShieldsUp(ctx context.Context, up bool) (*ipn.Prefs, error) {
	return f.lc.EditPrefs(ctx, &ipn.MaskedPrefs{
		Prefs:        ipn.Prefs{ShieldsUp: up},
		ShieldsUpSet: true,
	})
}

// SetAcceptRoutes toggles whether subnet routes advertised by other nodes
// are accepted.
func (f *Fetcher) SetAcceptRoutes(ctx context.Context, accept bool) (*ipn.Prefs, error) {
	return f.lc.EditPrefs(ctx, &ipn.MaskedPrefs{
		Prefs:       ipn.Prefs{RouteAll: accept},
		RouteAllSet: true,
	})
}

// SetAcceptDNS toggles whether DNS configuration from the admin panel is
// accepted.
func (f *Fetcher) SetAcceptDNS(ctx context.Context, accept bool) (*ipn.Prefs, error) {
	return f.lc.EditPrefs(ctx, &ipn.MaskedPrefs{
		Prefs:      ipn.Prefs{CorpDNS: accept},
		CorpDNSSet: true,
	})
}

// SetAdvertiseExitNode toggles whether this node advertises the two /0
// "exit node" routes, without disturbing any other subnet routes already
// present in AdvertiseRoutes. "Advertise as exit node" isn't its own pref
// field — tailscale set implements it the same way, via
// Prefs.AdvertisesExitNode() and the two all-IPv4/all-IPv6 /0 prefixes.
func (f *Fetcher) SetAdvertiseExitNode(ctx context.Context, advertise bool) (*ipn.Prefs, error) {
	cur, err := f.lc.GetPrefs(ctx)
	if err != nil {
		return nil, err
	}
	if cur.AdvertisesExitNode() == advertise {
		return cur, nil
	}
	routes := tsaddr.FilterPrefixesCopy(views.SliceOf(cur.AdvertiseRoutes), func(p netip.Prefix) bool {
		return p.Bits() != 0
	})
	if advertise {
		routes = append(routes, tsaddr.AllIPv4(), tsaddr.AllIPv6())
	}
	return f.lc.EditPrefs(ctx, &ipn.MaskedPrefs{
		Prefs:              ipn.Prefs{AdvertiseRoutes: routes},
		AdvertiseRoutesSet: true,
	})
}
```

In `internal/tsnet/client.go`, widen the `localClient` interface (it already imports `context`, `ipn`, and `ipnstate`):

```go
type localClient interface {
	Status(ctx context.Context) (*ipnstate.Status, error)
	EditPrefs(ctx context.Context, mp *ipn.MaskedPrefs) (*ipn.Prefs, error)
	GetPrefs(ctx context.Context) (*ipn.Prefs, error)
	CurrentDERPMap(ctx context.Context) (*tailcfg.DERPMap, error)
	Ping(ctx context.Context, ip netip.Addr, pingtype tailcfg.PingType) (*ipnstate.PingResult, error)
	WhoIs(ctx context.Context, remoteAddr string) (*apitype.WhoIsResponse, error)
}
```

(`*local.Client` already implements `GetPrefs` — pure additive interface surface, no other change to `client.go`.)

In `internal/tsnet/client_test.go`, add three fields to `fakeLocalClient`, add a `GetPrefs` method, and make `EditPrefs` count its calls:

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
}
```

```go
func (f *fakeLocalClient) EditPrefs(ctx context.Context, mp *ipn.MaskedPrefs) (*ipn.Prefs, error) {
	f.gotMasked = mp
	f.editCallCount++
	if f.editErr != nil {
		return nil, f.editErr
	}
	return &mp.Prefs, nil
}

func (f *fakeLocalClient) GetPrefs(ctx context.Context) (*ipn.Prefs, error) {
	return f.getPrefsResult, f.getPrefsErr
}
```

(`EditPrefs` above replaces the existing method — only the added
`f.editCallCount++` line and the `editCallCount` field are new; its
existing body and every other method are unchanged.)

- [ ] **Step 4: Run the test, verify it passes**

Run: `go test ./internal/tsnet/... -v`
Expected: PASS, including all pre-existing `internal/tsnet` tests (the `editCallCount` addition is purely additive — no existing test reads or depends on that field).

- [ ] **Step 5: Commit**

```bash
git add internal/tsnet/prefs.go internal/tsnet/prefs_test.go internal/tsnet/client.go internal/tsnet/client_test.go
git commit -m "feat: add Fetcher preferences methods (get + 5 toggles)"
```

---

### Task 2: Preferences overlay UI

**Files:**
- Create: `internal/ui/prefs.go`
- Modify: `internal/ui/model.go`
- Modify: `internal/ui/model_test.go`
- Create: `internal/ui/prefs_test.go`
- Modify: `README.md`

**Interfaces:**
- Consumes: `tsnet.Fetcher.GetPrefs`, `SetRunSSH`, `SetShieldsUp`, `SetAcceptRoutes`, `SetAcceptDNS`, `SetAdvertiseExitNode` from Task 1, all `(ctx context.Context, ...) (*ipn.Prefs, error)`.
- Produces: `func renderPrefsPanel(prefs *ipn.Prefs, cursor int, loading bool, prefsErr error, width int) string`, `func prefRows(prefs *ipn.Prefs) []prefRow` (row order: SSH server, Shields up, Accept routes, Accept DNS, Advertise as exit node — indices 0-4).

- [ ] **Step 1: Write the failing test for rendering**

Create `internal/ui/prefs_test.go`:

```go
package ui

import (
	"errors"
	"net/netip"
	"strings"
	"testing"

	"tailscale.com/ipn"
	"tailscale.com/net/tsaddr"
)

func TestRenderPrefsPanelAllOff(t *testing.T) {
	prefs := &ipn.Prefs{}
	got := renderPrefsPanel(prefs, 0, false, nil, 60)
	for _, want := range []string{"SSH server", "Shields up", "Accept routes", "Accept DNS", "Advertise as exit node"} {
		if !strings.Contains(got, want) {
			t.Errorf("renderPrefsPanel() = %q, want to contain %q", got, want)
		}
	}
	if strings.Contains(got, "on") {
		t.Errorf("renderPrefsPanel() = %q, want no \"on\" state when everything is off", got)
	}
}

func TestRenderPrefsPanelAllOn(t *testing.T) {
	prefs := &ipn.Prefs{
		RunSSH:          true,
		ShieldsUp:       true,
		RouteAll:        true,
		CorpDNS:         true,
		AdvertiseRoutes: []netip.Prefix{tsaddr.AllIPv4(), tsaddr.AllIPv6()},
	}
	got := renderPrefsPanel(prefs, 0, false, nil, 60)
	if strings.Contains(got, "off") {
		t.Errorf("renderPrefsPanel() = %q, want no \"off\" state when everything is on", got)
	}
}

func TestRenderPrefsPanelHighlightsCursorRow(t *testing.T) {
	prefs := &ipn.Prefs{}
	got0 := renderPrefsPanel(prefs, 0, false, nil, 60)
	got4 := renderPrefsPanel(prefs, 4, false, nil, 60)
	if got0 == got4 {
		t.Error("renderPrefsPanel() output identical for cursor=0 and cursor=4, want the highlighted row to differ")
	}
}

func TestRenderPrefsPanelShowsLoadingState(t *testing.T) {
	got := renderPrefsPanel(nil, 0, true, nil, 60)
	if !strings.Contains(got, "loading") {
		t.Errorf("renderPrefsPanel() = %q, want a loading message", got)
	}
}

func TestRenderPrefsPanelShowsErrorBeforeFirstFetch(t *testing.T) {
	got := renderPrefsPanel(nil, 0, false, errors.New("get prefs failed"), 60)
	if !strings.Contains(got, "get prefs failed") {
		t.Errorf("renderPrefsPanel() = %q, want the error message", got)
	}
}

func TestRenderPrefsPanelShowsErrorAlongsideLoadedPrefs(t *testing.T) {
	prefs := &ipn.Prefs{RunSSH: true}
	got := renderPrefsPanel(prefs, 0, false, errors.New("edit failed"), 60)
	if !strings.Contains(got, "SSH server") || !strings.Contains(got, "edit failed") {
		t.Errorf("renderPrefsPanel() = %q, want both the row list and the error", got)
	}
}
```

- [ ] **Step 2: Run the test, verify it fails**

Run: `go test ./internal/ui/... -run TestRenderPrefsPanel -v`
Expected: FAIL to compile — `renderPrefsPanel` doesn't exist yet.

- [ ] **Step 3: Implement rendering**

Create `internal/ui/prefs.go`:

```go
package ui

import (
	"strings"

	"tailscale.com/ipn"
)

// numPrefRows is the fixed number of rows the preferences panel shows.
const numPrefRows = 5

type prefRow struct {
	label string
	on    bool
}

// prefRows defines the fixed order of the preferences panel's rows. Both
// renderPrefsPanel and Model.updatePrefsView (which maps prefsCursor to
// the Fetcher setter to call) rely on this exact order: SSH server,
// Shields up, Accept routes, Accept DNS, Advertise as exit node.
func prefRows(prefs *ipn.Prefs) []prefRow {
	return []prefRow{
		{"SSH server", prefs.RunSSH},
		{"Shields up", prefs.ShieldsUp},
		{"Accept routes", prefs.RouteAll},
		{"Accept DNS", prefs.CorpDNS},
		{"Advertise as exit node", prefs.AdvertisesExitNode()},
	}
}

func toggleState(on bool) string {
	if on {
		return onlineStyle.Render("on")
	}
	return offlineStyle.Render("off")
}

// renderPrefsPanel shows the preferences panel opened via 'p' on the peer
// table: a cursor-navigable list of toggles, styled like the exit-node
// picker. loading unconditionally blanks to a loading message (matching
// the DERP/peer-detail views' precedent) even if prefs from a previous
// fetch are available — a fresh EditPrefs/GetPrefs call is always in
// flight while loading is true, and Model never sets it without also
// firing that call.
func renderPrefsPanel(prefs *ipn.Prefs, cursor int, loading bool, prefsErr error, width int) string {
	var b strings.Builder

	if loading {
		b.WriteString(helpStyle.Render("  loading preferences…"))
		return b.String()
	}
	if prefs == nil {
		msg := "no preferences loaded"
		if prefsErr != nil {
			msg = prefsErr.Error()
		}
		b.WriteString(errorStyle.Render("  " + msg))
		return b.String()
	}

	b.WriteString(headerStyle.Render("Preferences"))
	b.WriteString("\n")

	for i, row := range prefRows(prefs) {
		text := "  " + row.label + "  " + toggleState(row.on)
		style := rowStyle
		if i == cursor {
			text = fitWidth(text, width)
			style = selectedRowStyle
		}
		b.WriteString(style.Render(text))
		b.WriteString("\n")
	}

	if prefsErr != nil {
		b.WriteString("\n")
		b.WriteString(errorStyle.Render("  " + prefsErr.Error()))
	}

	return strings.TrimRight(b.String(), "\n")
}
```

- [ ] **Step 4: Run the test, verify it passes**

Run: `go test ./internal/ui/... -run TestRenderPrefsPanel -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/ui/prefs.go internal/ui/prefs_test.go
git commit -m "feat: render the preferences panel"
```

- [ ] **Step 6: Write the failing test for Model wiring**

In `internal/ui/model_test.go`, add `"tailscale.com/ipn"` to the imports
(`"errors"` is already imported, used by other tests in this file).

Add to `internal/ui/model_test.go`:

```go
func TestPrefsPanelOpensAndFetches(t *testing.T) {
	m := newTestModel()

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("p")})
	m = updated.(Model)

	if !m.viewingPrefs {
		t.Fatal("viewingPrefs = false after 'p', want true")
	}
	if !m.prefsLoading {
		t.Fatal("prefsLoading = false immediately after 'p', want true")
	}
	if cmd == nil {
		t.Fatal("Update('p') returned nil cmd, want a prefs-fetch command")
	}
}

func TestPrefsPanelEscCloses(t *testing.T) {
	m := newTestModel()
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("p")})
	m = updated.(Model)

	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = updated.(Model)

	if m.viewingPrefs {
		t.Fatal("viewingPrefs = true after esc, want false")
	}
}

func TestPrefsCursorMovesAndClamps(t *testing.T) {
	m := newTestModel()
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("p")})
	m = updated.(Model)

	for i := 0; i < 10; i++ {
		updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
		m = updated.(Model)
	}
	if m.prefsCursor != numPrefRows-1 {
		t.Errorf("prefsCursor = %d after 10x 'j', want clamped to %d", m.prefsCursor, numPrefRows-1)
	}

	for i := 0; i < 10; i++ {
		updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("k")})
		m = updated.(Model)
	}
	if m.prefsCursor != 0 {
		t.Errorf("prefsCursor = %d after 10x 'k', want clamped to 0", m.prefsCursor)
	}
}

func TestPrefsMsgPopulatesPrefsAndClearsLoading(t *testing.T) {
	m := newTestModel()
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("p")})
	m = updated.(Model)

	updated, _ = m.Update(prefsMsg{prefs: &ipn.Prefs{RunSSH: true}})
	m = updated.(Model)

	if m.prefsLoading {
		t.Fatal("prefsLoading = true after prefsMsg, want false")
	}
	view := m.View()
	if !contains(view, "SSH server") {
		t.Errorf("View() missing preferences panel\n---\n%s", view)
	}
}

func TestPrefsEnterWithNilPrefsIsNoop(t *testing.T) {
	m := newTestModel()
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("p")})
	m = updated.(Model)

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(Model)

	if cmd != nil {
		t.Error("Update(enter) with prefs still nil returned a non-nil cmd, want nil")
	}
	if !m.prefsLoading {
		t.Error("prefsLoading = false after a no-op enter, want it to remain true (fetch still in flight)")
	}
}

func TestPrefsEnterTogglesAndReturnsCmd(t *testing.T) {
	m := newTestModel()
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("p")})
	m = updated.(Model)
	updated, _ = m.Update(prefsMsg{prefs: &ipn.Prefs{RunSSH: false}})
	m = updated.(Model)

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(Model)

	if !m.prefsLoading {
		t.Fatal("prefsLoading = false after enter on a loaded panel, want true")
	}
	if cmd == nil {
		t.Fatal("Update(enter) in prefs panel returned nil cmd, want a toggle command")
	}
}

func TestPrefsMsgErrorKeepsExistingPrefs(t *testing.T) {
	m := newTestModel()
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("p")})
	m = updated.(Model)
	updated, _ = m.Update(prefsMsg{prefs: &ipn.Prefs{RunSSH: true}})
	m = updated.(Model)

	updated, _ = m.Update(prefsMsg{err: errors.New("boom")})
	m = updated.(Model)

	if m.prefs == nil || !m.prefs.RunSSH {
		t.Fatalf("prefs = %+v after a failed toggle, want the last-known-good RunSSH=true prefs preserved", m.prefs)
	}
	if m.prefsErr == nil {
		t.Fatal("prefsErr = nil after prefsMsg with an error, want it set")
	}
}
```

- [ ] **Step 7: Run the test, verify it fails**

Run: `go test ./internal/ui/... -run TestPrefs -v`
Expected: FAIL to compile — `viewingPrefs`, `prefsCursor`, `prefs`, `prefsLoading`, `prefsErr`, `prefsMsg` don't exist on `Model` yet.

- [ ] **Step 8: Wire it into Model**

In `internal/ui/model.go`, add `"tailscale.com/ipn"` to the imports.

Add a new message type after `peerDetailReportMsg`:

```go
type prefsMsg struct {
	prefs *ipn.Prefs
	err   error
}
```

Add five fields to the `Model` struct, after the `viewingPeerDetail` block and before `viewingSSH`:

```go
	viewingPrefs bool
	prefsCursor  int
	prefs        *ipn.Prefs
	prefsLoading bool
	prefsErr     error
```

Add six command functions after `peerDetailCmd`:

```go
func prefsFetchCmd(fetcher *tsnet.Fetcher) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		prefs, err := fetcher.GetPrefs(ctx)
		return prefsMsg{prefs: prefs, err: err}
	}
}

func setRunSSHCmd(fetcher *tsnet.Fetcher, run bool) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		prefs, err := fetcher.SetRunSSH(ctx, run)
		return prefsMsg{prefs: prefs, err: err}
	}
}

func setShieldsUpCmd(fetcher *tsnet.Fetcher, up bool) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		prefs, err := fetcher.SetShieldsUp(ctx, up)
		return prefsMsg{prefs: prefs, err: err}
	}
}

func setAcceptRoutesCmd(fetcher *tsnet.Fetcher, accept bool) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		prefs, err := fetcher.SetAcceptRoutes(ctx, accept)
		return prefsMsg{prefs: prefs, err: err}
	}
}

func setAcceptDNSCmd(fetcher *tsnet.Fetcher, accept bool) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		prefs, err := fetcher.SetAcceptDNS(ctx, accept)
		return prefsMsg{prefs: prefs, err: err}
	}
}

func setAdvertiseExitNodeCmd(fetcher *tsnet.Fetcher, advertise bool) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		prefs, err := fetcher.SetAdvertiseExitNode(ctx, advertise)
		return prefsMsg{prefs: prefs, err: err}
	}
}
```

Add a case to `Update`'s message switch, after `case peerDetailReportMsg:`:

```go
	case prefsMsg:
		m.prefsLoading = false
		if msg.err != nil {
			m.prefsErr = msg.err
			return m, nil
		}
		m.prefs = msg.prefs
		m.prefsErr = nil
		return m, nil
```

Add a case to the `tea.KeyMsg` dispatch switch inside `Update`, alongside `case m.viewingPeerDetail:`:

```go
		case m.viewingPrefs:
			return m.updatePrefsView(msg)
```

In `updateNormal`, add a case after `case "i":`'s block, before the closing of the switch:

```go
	case "p":
		m.viewingPrefs = true
		m.prefsCursor = 0
		m.prefsLoading = true
		m.prefsErr = nil
		return m, prefsFetchCmd(m.fetcher)
```

Add a new method after `updatePeerDetailView`:

```go
func (m Model) updatePrefsView(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "p":
		m.viewingPrefs = false
		return m, nil
	case "j", "down":
		m.prefsCursor++
		m.clampPrefsCursor()
	case "k", "up":
		m.prefsCursor--
		m.clampPrefsCursor()
	case "enter", " ":
		if m.prefs == nil {
			return m, nil
		}
		next := !prefRows(m.prefs)[m.prefsCursor].on
		m.prefsLoading = true
		switch m.prefsCursor {
		case 0:
			return m, setRunSSHCmd(m.fetcher, next)
		case 1:
			return m, setShieldsUpCmd(m.fetcher, next)
		case 2:
			return m, setAcceptRoutesCmd(m.fetcher, next)
		case 3:
			return m, setAcceptDNSCmd(m.fetcher, next)
		case 4:
			return m, setAdvertiseExitNodeCmd(m.fetcher, next)
		}
	}
	return m, nil
}

func (m *Model) clampPrefsCursor() {
	if m.prefsCursor < 0 {
		m.prefsCursor = 0
	}
	if m.prefsCursor > numPrefRows-1 {
		m.prefsCursor = numPrefRows - 1
	}
}
```

In `View()`, add a case alongside `case m.viewingPeerDetail:`:

```go
	case m.viewingPrefs:
		body = renderPrefsPanel(m.prefs, m.prefsCursor, m.prefsLoading, m.prefsErr, width)
		footer = helpStyle.Render("j/k move  enter toggle  esc/p back")
```

Update the default footer's help text to mention `p`:

```go
			footer = helpStyle.Render("j/k move  g/G top/bottom  / search  enter ssh  x exit-node  d derp  i info  p prefs  r refresh  q quit")
```

- [ ] **Step 9: Run the test, verify it passes**

Run: `go test ./internal/ui/... -v`
Expected: PASS, including all pre-existing `internal/ui` tests.

Then run the full suite:

Run: `go build ./... && go vet ./... && gofmt -l . && go test ./...`
Expected: builds cleanly, `gofmt -l .` prints nothing, all tests PASS.

- [ ] **Step 10: Commit**

```bash
git add internal/ui/model.go internal/ui/model_test.go
git commit -m "feat: wire the preferences panel into Model"
```

- [ ] **Step 11: Update docs**

In `README.md`, add a row to the main keybinding table (after the `i` row):

```markdown
| `p` | Open the preferences panel (SSH, shields up, accept routes/DNS, advertise exit node) |
```

Add a new subsection after "### Peer detail" and before "## Flags":

```markdown
### Preferences panel

Press `p` to view and flip the 5 preferences ts-hud exposes: SSH server,
shields up, accept routes, accept DNS, and advertising this device as an
exit node. Toggling a row applies immediately — there's no separate save
step, matching `tailscale set`.

| Key | Action |
|---|---|
| `j` / `k` | Move down / up |
| `enter` | Toggle the highlighted preference |
| `esc` / `p` | Back to the peer table |
```

Commit:

```bash
git add README.md
git commit -m "docs: document the preferences panel"
```

- [ ] **Step 12: Manual verification**

This exercises the real `GetPrefs`/`EditPrefs` LocalAPI calls — not mockable.

```bash
go build -o /tmp/ts-hud-verify .
/tmp/ts-hud-verify
```

Press `p`. Expect: all 5 rows with their real current state. Move the
cursor with `j`/`k` and press `enter` on a low-risk row (Accept DNS or
Accept routes are safest — avoid toggling Shields up or SSH if you're
relying on either right now). Expect the row to flip and stay flipped.
Press `enter` again to flip it back to its original state before
finishing. Press `esc` to return to the peer table.

- [ ] **Step 13: Report back**

Summarize which preference you toggled, its state before and after, and
confirm it was flipped back to its original value.
