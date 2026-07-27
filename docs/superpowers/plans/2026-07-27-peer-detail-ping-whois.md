# Peer Detail (Ping + WhoIs) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Let a user ping and whois the selected peer without leaving ts-hud, via a single `i` (info) overlay combining both.

**Architecture:** `tsnet.Fetcher` gains `PeerDetail(ctx, ip) PeerDetail`, running `Ping` and `WhoIs` concurrently against the real LocalAPI client (both already exist as methods on `*local.Client` — pure additive interface surface, no existing signature changes). `internal/ui` gains a new overlay, `viewingPeerDetail`, following the exact open→probe→refresh→close pattern the DERP view already established.

**Tech Stack:** Go, Bubble Tea/lipgloss (existing), `tailscale.com/client/local`, `tailscale.com/client/tailscale/apitype`, `tailscale.com/ipn/ipnstate`, `tailscale.com/tailcfg`.

## Global Constraints

- No new dependencies — `Ping`/`WhoIs` are already-vendored methods on `*local.Client`.
- No new styles — reuse `onlineStyle`, `offlineStyle`, `errorStyle`, `helpStyle`, `headerStyle` from `internal/ui/styles.go`.
- `PeerDetail` never returns a top-level error: `Ping` and `WhoIs` degrade independently, each carrying its own error field. The UI always renders whatever came back.
- `i` works for any selected peer, online or offline (unlike `enter`/ssh, which requires `Online`).
- The overlay's target peer is captured at open time and must not change if the background peer-list auto-refresh (every `--refresh-rate`, default 5s) reorders or updates the underlying table while the view is open.

---

### Task 1: `tsnet.Fetcher.PeerDetail` data layer

**Files:**
- Create: `internal/tsnet/peerdetail.go`
- Modify: `internal/tsnet/client.go`
- Modify: `internal/tsnet/client_test.go`
- Create: `internal/tsnet/peerdetail_test.go`

**Interfaces:**
- Produces: `tsnet.PeerDetail{ Ping *ipnstate.PingResult; PingErr error; Owner string; Tags []string; WhoIsErr error }` and `func (f *Fetcher) PeerDetail(ctx context.Context, ip netip.Addr) PeerDetail` (no error return — see Global Constraints).

- [ ] **Step 1: Write the failing test**

Create `internal/tsnet/peerdetail_test.go`:

```go
package tsnet

import (
	"context"
	"errors"
	"net/netip"
	"testing"

	"tailscale.com/client/tailscale/apitype"
	"tailscale.com/ipn/ipnstate"
	"tailscale.com/tailcfg"
)

func TestPeerDetailCombinesPingAndWhoIs(t *testing.T) {
	fake := &fakeLocalClient{
		pingResult: &ipnstate.PingResult{LatencySeconds: 0.0234, Endpoint: "100.64.0.5:41641"},
		whoIsResult: &apitype.WhoIsResponse{
			UserProfile: &tailcfg.UserProfile{DisplayName: "Alice Smith", LoginName: "alice@example.com"},
			Node:        &tailcfg.Node{Tags: []string{"tag:server"}},
		},
	}
	f := &Fetcher{lc: fake}

	got := f.PeerDetail(context.Background(), netip.MustParseAddr("100.64.0.5"))

	if got.PingErr != nil {
		t.Errorf("PingErr = %v, want nil", got.PingErr)
	}
	if got.Ping == nil || got.Ping.Endpoint != "100.64.0.5:41641" {
		t.Errorf("Ping = %+v, want Endpoint 100.64.0.5:41641", got.Ping)
	}
	if got.Owner != "Alice Smith" {
		t.Errorf("Owner = %q, want %q", got.Owner, "Alice Smith")
	}
	if len(got.Tags) != 1 || got.Tags[0] != "tag:server" {
		t.Errorf("Tags = %v, want [tag:server]", got.Tags)
	}
}

func TestPeerDetailFallsBackToLoginNameWhenDisplayNameEmpty(t *testing.T) {
	fake := &fakeLocalClient{
		whoIsResult: &apitype.WhoIsResponse{
			UserProfile: &tailcfg.UserProfile{LoginName: "alice@example.com"},
		},
	}
	f := &Fetcher{lc: fake}

	got := f.PeerDetail(context.Background(), netip.MustParseAddr("100.64.0.5"))

	if got.Owner != "alice@example.com" {
		t.Errorf("Owner = %q, want %q", got.Owner, "alice@example.com")
	}
}

func TestPeerDetailPingFailureDoesNotBlockWhoIs(t *testing.T) {
	fake := &fakeLocalClient{
		pingErr: errors.New("ping timeout"),
		whoIsResult: &apitype.WhoIsResponse{
			UserProfile: &tailcfg.UserProfile{DisplayName: "Alice Smith"},
		},
	}
	f := &Fetcher{lc: fake}

	got := f.PeerDetail(context.Background(), netip.MustParseAddr("100.64.0.5"))

	if got.PingErr == nil {
		t.Fatal("PingErr = nil, want an error")
	}
	if got.Ping != nil {
		t.Errorf("Ping = %+v, want nil", got.Ping)
	}
	if got.Owner != "Alice Smith" {
		t.Errorf("Owner = %q, want %q (WhoIs should still succeed)", got.Owner, "Alice Smith")
	}
}

func TestPeerDetailWhoIsFailureDoesNotBlockPing(t *testing.T) {
	fake := &fakeLocalClient{
		pingResult: &ipnstate.PingResult{LatencySeconds: 0.01, DERPRegionCode: "jnb"},
		whoIsErr:   errors.New("whois: not found"),
	}
	f := &Fetcher{lc: fake}

	got := f.PeerDetail(context.Background(), netip.MustParseAddr("100.64.0.5"))

	if got.WhoIsErr == nil {
		t.Fatal("WhoIsErr = nil, want an error")
	}
	if got.Owner != "" {
		t.Errorf("Owner = %q, want empty", got.Owner)
	}
	if got.Ping == nil || got.Ping.DERPRegionCode != "jnb" {
		t.Errorf("Ping = %+v, want DERPRegionCode jnb (ping should still succeed)", got.Ping)
	}
}
```

- [ ] **Step 2: Run the test, verify it fails**

Run: `go test ./internal/tsnet/... -run TestPeerDetail -v`
Expected: FAIL to compile — `fakeLocalClient` has no `pingResult`/`pingErr`/`whoIsResult`/`whoIsErr` fields yet, and `Fetcher.PeerDetail` doesn't exist.

- [ ] **Step 3: Implement**

Create `internal/tsnet/peerdetail.go`:

```go
package tsnet

import (
	"context"
	"net/netip"
	"sync"

	"tailscale.com/ipn/ipnstate"
	"tailscale.com/tailcfg"
)

// PeerDetail is the result of a live ping + whois probe against one peer.
// Ping and WhoIs results are independent: either can fail without the
// other, and there is no overall error — the UI renders whatever came
// back and shows an inline error for whichever half failed.
type PeerDetail struct {
	Ping    *ipnstate.PingResult // nil if the ping RPC itself errored (see PingErr)
	PingErr error

	Owner    string // UserProfile.DisplayName, falling back to LoginName; empty if unknown
	Tags     []string
	WhoIsErr error
}

// PeerDetail runs a live ping (the same "disco" ping tailscale ping uses
// by default) and a whois lookup against ip, concurrently.
func (f *Fetcher) PeerDetail(ctx context.Context, ip netip.Addr) PeerDetail {
	var d PeerDetail
	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		d.Ping, d.PingErr = f.lc.Ping(ctx, ip, tailcfg.PingDisco)
	}()

	go func() {
		defer wg.Done()
		who, err := f.lc.WhoIs(ctx, ip.String())
		if err != nil {
			d.WhoIsErr = err
			return
		}
		if who.UserProfile != nil {
			d.Owner = who.UserProfile.DisplayName
			if d.Owner == "" {
				d.Owner = who.UserProfile.LoginName
			}
		}
		if who.Node != nil {
			d.Tags = who.Node.Tags
		}
	}()

	wg.Wait()
	return d
}
```

In `internal/tsnet/client.go`, add `"net/netip"` and `"tailscale.com/client/tailscale/apitype"` to the imports, and widen the `localClient` interface:

```go
type localClient interface {
	Status(ctx context.Context) (*ipnstate.Status, error)
	EditPrefs(ctx context.Context, mp *ipn.MaskedPrefs) (*ipn.Prefs, error)
	CurrentDERPMap(ctx context.Context) (*tailcfg.DERPMap, error)
	Ping(ctx context.Context, ip netip.Addr, pingtype tailcfg.PingType) (*ipnstate.PingResult, error)
	WhoIs(ctx context.Context, remoteAddr string) (*apitype.WhoIsResponse, error)
}
```

In `internal/tsnet/client_test.go`, add `"tailscale.com/client/tailscale/apitype"` to the imports (`"net/netip"` is already imported), add four fields to the `fakeLocalClient` struct, and add the two new methods:

```go
type fakeLocalClient struct {
	editErr    error
	gotMasked  *ipn.MaskedPrefs
	derpMap    *tailcfg.DERPMap
	derpMapErr error

	pingResult  *ipnstate.PingResult
	pingErr     error
	whoIsResult *apitype.WhoIsResponse
	whoIsErr    error
}
```

```go
func (f *fakeLocalClient) Ping(ctx context.Context, ip netip.Addr, pingtype tailcfg.PingType) (*ipnstate.PingResult, error) {
	return f.pingResult, f.pingErr
}

func (f *fakeLocalClient) WhoIs(ctx context.Context, remoteAddr string) (*apitype.WhoIsResponse, error) {
	return f.whoIsResult, f.whoIsErr
}
```

- [ ] **Step 4: Run the test, verify it passes**

Run: `go test ./internal/tsnet/... -v`
Expected: PASS, including all pre-existing `internal/tsnet` tests.

- [ ] **Step 5: Commit**

```bash
git add internal/tsnet/peerdetail.go internal/tsnet/peerdetail_test.go internal/tsnet/client.go internal/tsnet/client_test.go
git commit -m "feat: add Fetcher.PeerDetail (live ping + whois)"
```

---

### Task 2: Peer detail overlay UI

**Files:**
- Create: `internal/ui/peerdetail.go`
- Modify: `internal/ui/model.go`
- Modify: `internal/ui/model_test.go`
- Create: `internal/ui/peerdetail_test.go`
- Modify: `README.md`

**Interfaces:**
- Consumes: `tsnet.PeerDetail` and `func (f *Fetcher) PeerDetail(ctx context.Context, ip netip.Addr) tsnet.PeerDetail` from Task 1.
- Produces: `func renderPeerDetail(target tsnet.Peer, detail tsnet.PeerDetail, loading bool) string`.

- [ ] **Step 1: Write the failing test for rendering**

Create `internal/ui/peerdetail_test.go`:

```go
package ui

import (
	"errors"
	"strings"
	"testing"

	"github.com/Wian47/ts-hud/internal/tsnet"
	"tailscale.com/ipn/ipnstate"
)

var peerDetailTestTarget = tsnet.Peer{HostName: "bravo", OS: "linux", Online: true}

func TestRenderPeerDetailShowsLoadingState(t *testing.T) {
	got := renderPeerDetail(peerDetailTestTarget, tsnet.PeerDetail{}, true)
	if !strings.Contains(got, "probing") {
		t.Errorf("renderPeerDetail() = %q, want loading message", got)
	}
}

func TestRenderPeerDetailDirectPing(t *testing.T) {
	detail := tsnet.PeerDetail{
		Owner: "Alice Smith",
		Tags:  []string{"tag:server"},
		Ping:  &ipnstate.PingResult{LatencySeconds: 0.0234, Endpoint: "100.64.0.5:41641"},
	}
	got := renderPeerDetail(peerDetailTestTarget, detail, false)
	for _, want := range []string{"Alice Smith", "tag:server", "23.4ms", "100.64.0.5:41641", "direct"} {
		if !strings.Contains(got, want) {
			t.Errorf("renderPeerDetail() = %q, want to contain %q", got, want)
		}
	}
}

func TestRenderPeerDetailDERPPing(t *testing.T) {
	detail := tsnet.PeerDetail{
		Ping: &ipnstate.PingResult{LatencySeconds: 0.05, DERPRegionCode: "jnb"},
	}
	got := renderPeerDetail(peerDetailTestTarget, detail, false)
	for _, want := range []string{"50.0ms", "DERP jnb"} {
		if !strings.Contains(got, want) {
			t.Errorf("renderPeerDetail() = %q, want to contain %q", got, want)
		}
	}
}

func TestRenderPeerDetailPingError(t *testing.T) {
	detail := tsnet.PeerDetail{PingErr: errors.New("ping rpc failed")}
	got := renderPeerDetail(peerDetailTestTarget, detail, false)
	if !strings.Contains(got, "ping rpc failed") {
		t.Errorf("renderPeerDetail() = %q, want the ping error", got)
	}
}

func TestRenderPeerDetailWhoIsError(t *testing.T) {
	detail := tsnet.PeerDetail{WhoIsErr: errors.New("whois: not found")}
	got := renderPeerDetail(peerDetailTestTarget, detail, false)
	if !strings.Contains(got, "whois: not found") {
		t.Errorf("renderPeerDetail() = %q, want the whois error", got)
	}
}

func TestRenderPeerDetailNoTagsShowsNone(t *testing.T) {
	detail := tsnet.PeerDetail{Owner: "Alice"}
	got := renderPeerDetail(peerDetailTestTarget, detail, false)
	if !strings.Contains(got, "none") {
		t.Errorf("renderPeerDetail() = %q, want \"none\" for empty tags", got)
	}
}
```

- [ ] **Step 2: Run the test, verify it fails**

Run: `go test ./internal/ui/... -run TestRenderPeerDetail -v`
Expected: FAIL to compile — `renderPeerDetail` doesn't exist yet.

- [ ] **Step 3: Implement rendering**

Create `internal/ui/peerdetail.go`:

```go
package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"tailscale.com/ipn/ipnstate"

	"github.com/Wian47/ts-hud/internal/tsnet"
)

// renderPeerDetail shows a live ping + whois probe result for one peer,
// opened via 'i' on the peer table.
func renderPeerDetail(target tsnet.Peer, detail tsnet.PeerDetail, loading bool) string {
	var b strings.Builder

	status := "offline"
	statusStyle := offlineStyle
	if target.Online {
		status = "online"
		statusStyle = onlineStyle
	}
	b.WriteString(headerStyle.Render(target.DisplayName()))
	b.WriteString("  " + target.OS + "  ")
	b.WriteString(statusStyle.Render(status))
	b.WriteString("\n\n")

	if loading {
		b.WriteString(helpStyle.Render("  probing…"))
		return b.String()
	}

	if detail.WhoIsErr != nil {
		b.WriteString("Owner: " + errorStyle.Render(detail.WhoIsErr.Error()))
	} else {
		owner := detail.Owner
		if owner == "" {
			owner = "unknown"
		}
		tags := "none"
		if len(detail.Tags) > 0 {
			tags = strings.Join(detail.Tags, ", ")
		}
		b.WriteString("Owner: " + owner + "\n")
		b.WriteString("Tags:  " + tags)
	}
	b.WriteString("\n\n")

	switch {
	case detail.PingErr != nil:
		b.WriteString("Ping:  " + errorStyle.Render(detail.PingErr.Error()))
	case detail.Ping != nil:
		text, style := formatPingResult(detail.Ping)
		b.WriteString("Ping:  " + style.Render(text))
	default:
		b.WriteString("Ping:  " + helpStyle.Render("no result"))
	}

	return strings.TrimRight(b.String(), "\n")
}

// formatPingResult renders one PingResult as a one-line summary and the
// style to render it in: onlineStyle for a direct path, offlineStyle for
// a relayed one, errorStyle if the probe ran but couldn't reach the peer.
func formatPingResult(pr *ipnstate.PingResult) (string, lipgloss.Style) {
	if pr.Err != "" {
		return pr.Err, errorStyle
	}
	latency := fmt.Sprintf("%.1fms", pr.LatencySeconds*1000)
	switch {
	case pr.Endpoint != "":
		return fmt.Sprintf("%s via %s (direct)", latency, pr.Endpoint), onlineStyle
	case pr.DERPRegionCode != "":
		return fmt.Sprintf("%s via DERP %s", latency, pr.DERPRegionCode), offlineStyle
	case pr.PeerRelay != "":
		return fmt.Sprintf("%s via peer relay %s", latency, pr.PeerRelay), offlineStyle
	default:
		return latency, onlineStyle
	}
}
```

- [ ] **Step 4: Run the test, verify it passes**

Run: `go test ./internal/ui/... -run TestRenderPeerDetail -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/ui/peerdetail.go internal/ui/peerdetail_test.go
git commit -m "feat: render the peer detail overlay"
```

- [ ] **Step 6: Write the failing test for Model wiring**

Add to `internal/ui/model_test.go` (`"net/netip"` is already imported, used by `testPeers()`; no import changes needed):

```go
func TestPeerDetailOpensForSelectedPeerIncludingOffline(t *testing.T) {
	m := newTestModel()
	m.cursor = 2 // charlie, offline

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("i")})
	m = updated.(Model)

	if !m.viewingPeerDetail {
		t.Fatal("viewingPeerDetail = false after 'i' on an offline peer, want true")
	}
	if m.peerDetailTarget.HostName != "charlie" {
		t.Errorf("peerDetailTarget.HostName = %q, want %q", m.peerDetailTarget.HostName, "charlie")
	}
	if !m.peerDetailLoading {
		t.Fatal("peerDetailLoading = false immediately after 'i', want true")
	}
	if cmd == nil {
		t.Fatal("Update('i') returned nil cmd, want a peer-detail probe command")
	}
}

func TestPeerDetailEscCloses(t *testing.T) {
	m := newTestModel()
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("i")})
	m = updated.(Model)

	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = updated.(Model)

	if m.viewingPeerDetail {
		t.Fatal("viewingPeerDetail = true after esc, want false")
	}
}

func TestPeerDetailReportMsgClearsLoading(t *testing.T) {
	m := newTestModel()
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("i")})
	m = updated.(Model)

	result := tsnet.PeerDetail{Owner: "Alice Smith", Tags: []string{"tag:server"}}
	updated, _ = m.Update(peerDetailReportMsg{result: result})
	m = updated.(Model)

	if m.peerDetailLoading {
		t.Fatal("peerDetailLoading = true after peerDetailReportMsg, want false")
	}
	view := m.View()
	if !contains(view, "Alice Smith") {
		t.Errorf("View() missing owner\n---\n%s", view)
	}
}

func TestPeerDetailTargetSurvivesBackgroundPeerRefresh(t *testing.T) {
	m := newTestModel()
	m.cursor = 0 // bravo
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("i")})
	m = updated.(Model)

	// Simulate the background auto-refresh reordering the peer list while
	// the detail view is open.
	updated, _ = m.Update(peersMsg{peers: []tsnet.Peer{
		{HostName: "alpha", OS: "linux", Online: true, IPs: []netip.Addr{netip.MustParseAddr("100.64.0.1")}},
		{HostName: "bravo", OS: "linux", Online: false, IPs: []netip.Addr{netip.MustParseAddr("100.64.0.2")}},
	}})
	m = updated.(Model)

	if m.peerDetailTarget.HostName != "bravo" {
		t.Errorf("peerDetailTarget.HostName = %q after background refresh, want %q (should stay pinned to the peer the view was opened for)", m.peerDetailTarget.HostName, "bravo")
	}
}

func TestPeerDetailRefreshRetriggersProbe(t *testing.T) {
	m := newTestModel()
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("i")})
	m = updated.(Model)
	updated, _ = m.Update(peerDetailReportMsg{result: tsnet.PeerDetail{Owner: "Alice"}})
	m = updated.(Model)

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("r")})
	m = updated.(Model)

	if !m.peerDetailLoading {
		t.Fatal("peerDetailLoading = false after 'r' in peer detail view, want true")
	}
	if cmd == nil {
		t.Fatal("Update('r') in peer detail view returned nil cmd, want a probe command")
	}
}
```

- [ ] **Step 7: Run the test, verify it fails**

Run: `go test ./internal/ui/... -run TestPeerDetail -v`
Expected: FAIL to compile — `viewingPeerDetail`, `peerDetailTarget`, `peerDetailLoading`, `peerDetailReportMsg` don't exist on `Model` yet.

- [ ] **Step 8: Wire it into Model**

In `internal/ui/model.go`, add `"net/netip"` to the imports.

Add a new message type after `derpReportMsg` (around line 27):

```go
type peerDetailReportMsg struct {
	result tsnet.PeerDetail
}
```

Add four fields to the `Model` struct, after the `viewingDERP` block and before `viewingSSH` (around line 53):

```go
	viewingPeerDetail bool
	peerDetailTarget  tsnet.Peer
	peerDetailResult  tsnet.PeerDetail
	peerDetailLoading bool
```

Add a new command function after `derpCheckCmd` (around line 128):

```go
// peerDetailCmd runs a live ping + whois probe against one peer's primary
// Tailscale IP.
func peerDetailCmd(fetcher *tsnet.Fetcher, ip netip.Addr) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return peerDetailReportMsg{result: fetcher.PeerDetail(ctx, ip)}
	}
}
```

Add a case to `Update`'s message switch, after `case derpReportMsg:` (around line 166):

```go
	case peerDetailReportMsg:
		m.peerDetailLoading = false
		m.peerDetailResult = msg.result
		return m, nil
```

Add a case to the `tea.KeyMsg` dispatch switch inside `Update`, alongside `case m.viewingDERP:` (around line 221):

```go
		case m.viewingPeerDetail:
			return m.updatePeerDetailView(msg)
```

In `updateNormal`, add a case after `case "d":` (around line 266), before the closing of the switch:

```go
	case "i":
		if peer, ok := m.selectedPeer(); ok && len(peer.IPs) > 0 {
			m.viewingPeerDetail = true
			m.peerDetailTarget = peer
			m.peerDetailLoading = true
			m.peerDetailResult = tsnet.PeerDetail{}
			return m, peerDetailCmd(m.fetcher, peer.IPs[0])
		}
```

Add a new method after `updateDERPView` (around line 283):

```go
func (m Model) updatePeerDetailView(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "i":
		m.viewingPeerDetail = false
		return m, nil
	case "r":
		m.peerDetailLoading = true
		return m, peerDetailCmd(m.fetcher, m.peerDetailTarget.IPs[0])
	}
	return m, nil
}
```

In `View()`, add a case alongside `case m.viewingDERP:` (around line 429):

```go
	case m.viewingPeerDetail:
		body = renderPeerDetail(m.peerDetailTarget, m.peerDetailResult, m.peerDetailLoading)
		footer = helpStyle.Render("r refresh  esc/i back")
```

Update the default footer's help text (around line 440) to mention `i`:

```go
			footer = helpStyle.Render("j/k move  g/G top/bottom  / search  enter ssh  x exit-node  d derp  i info  r refresh  q quit")
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
git commit -m "feat: wire the peer detail overlay into Model"
```

- [ ] **Step 11: Update docs**

In `README.md`, add a row to the main keybinding table (after the `d` row):

```markdown
| `i` | Open peer detail (live ping + owner/tags) for the selected peer |
```

Add a new subsection after "### DERP latency matrix" and before "## Flags":

```markdown
### Peer detail

Press `i` on any peer (online or offline) to see a live ping and whois
lookup: latency and path (direct, DERP-relayed, or unreachable), plus
which account owns the device and its ACL tags.

| Key | Action |
|---|---|
| `r` | Re-run the probe |
| `esc` / `i` | Back to the peer table |
```

Commit:

```bash
git add README.md
git commit -m "docs: document the peer detail overlay"
```

- [ ] **Step 12: Manual verification**

This exercises the real `Ping`/`WhoIs` LocalAPI calls — not mockable.

```bash
go build -o /tmp/ts-hud-verify .
/tmp/ts-hud-verify
```

Press `i` on an online peer. Expect: owner, tags (or "none"), and a ping
result showing latency plus "(direct)" or "via DERP <region>". Press `r`
to re-run. Press `esc` to return. Move to an offline peer and press `i`
again: expect the view to still open (owner/tags still resolve; the ping
line likely shows an unreachable error, which is the expected/correct
behavior for an offline peer, not a bug).

- [ ] **Step 13: Report back**

Summarize what the peer detail view showed for both an online and an
offline peer before considering this task done.
