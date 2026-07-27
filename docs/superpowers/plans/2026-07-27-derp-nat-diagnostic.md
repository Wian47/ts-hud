# DERP UDP/NAT Diagnostic Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Surface a one-line UDP/NAT connectivity verdict in the existing DERP latency view, using data the underlying `netcheck` call already computes.

**Architecture:** `tsnet.Fetcher.NetCheck` changes its return type from `([]DERPRegion, error)` to `(NetCheckResult, error)`, bundling the region list with `UDP`/`HardNAT`/`NATKnown` fields read off the same `netcheck.Report`. The UI's `renderDERPTable` renders one extra summary line derived from those fields, styled with the existing color palette.

**Tech Stack:** Go, Bubble Tea/lipgloss (existing), `tailscale.com/net/netcheck`, `tailscale.com/types/opt`.

## Global Constraints

- No new dependencies, no new keybindings, no new background probing — this is additive to the existing `d`-triggered DERP view only.
- Reuse existing style constants (`onlineStyle`, `offlineStyle`, `errorStyle`, `helpStyle`, `headerStyle` in `internal/ui/styles.go`) — do not add new styles.
- Keep `regionsFromReport` unchanged; wrap it, don't rewrite it.

---

### Task 1: NetCheckResult data layer + DERP view diagnostic summary

**Files:**
- Modify: `internal/tsnet/derp.go`
- Modify: `internal/tsnet/derp_test.go`
- Modify: `internal/ui/model.go`
- Modify: `internal/ui/model_test.go`
- Create: `internal/ui/derp_test.go`
- Modify: `internal/ui/derp.go`
- Modify: `README.md`
- Modify: `ROADMAP.md`

**Interfaces:**
- Produces: `tsnet.NetCheckResult{ Regions []DERPRegion; UDP bool; HardNAT bool; NATKnown bool }` and `func (f *Fetcher) NetCheck(ctx context.Context) (NetCheckResult, error)` — replaces the old `([]DERPRegion, error)` signature. `NATKnown` is `true` only when `netcheck.Report.MappingVariesByDestIP` was actually set (`opt.Bool.Get()`'s second return value); when `false`, `HardNAT` has no meaning.
- Produces: `func renderDERPTable(result tsnet.NetCheckResult, loading bool, checkErr error, width int) string` — replaces the old `(regions []tsnet.DERPRegion, ...)` signature.

#### Step 1: Write the failing test for the data layer

Add to `internal/tsnet/derp_test.go` (add `"tailscale.com/types/opt"` to the import block):

```go
func TestNetCheckResultFromReportSurfacesUDPAndNAT(t *testing.T) {
	dm := &tailcfg.DERPMap{
		Regions: map[int]*tailcfg.DERPRegion{
			1: {RegionID: 1, RegionCode: "nyc", RegionName: "New York City"},
		},
	}

	tests := []struct {
		name         string
		udp          bool
		mapping      opt.Bool
		wantHardNAT  bool
		wantNATKnown bool
	}{
		{
			name:         "udp ok, easy NAT",
			udp:          true,
			mapping:      opt.NewBool(false),
			wantHardNAT:  false,
			wantNATKnown: true,
		},
		{
			name:         "udp ok, hard NAT",
			udp:          true,
			mapping:      opt.NewBool(true),
			wantHardNAT:  true,
			wantNATKnown: true,
		},
		{
			name:         "udp failed, NAT unknown",
			udp:          false,
			mapping:      opt.Bool(""),
			wantHardNAT:  false,
			wantNATKnown: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			report := &netcheck.Report{
				UDP:                   tt.udp,
				MappingVariesByDestIP: tt.mapping,
			}

			got := netCheckResultFromReport(dm, report)

			if got.UDP != tt.udp {
				t.Errorf("UDP = %v, want %v", got.UDP, tt.udp)
			}
			if got.HardNAT != tt.wantHardNAT {
				t.Errorf("HardNAT = %v, want %v", got.HardNAT, tt.wantHardNAT)
			}
			if got.NATKnown != tt.wantNATKnown {
				t.Errorf("NATKnown = %v, want %v", got.NATKnown, tt.wantNATKnown)
			}
			if len(got.Regions) != 1 || got.Regions[0].Code != "nyc" {
				t.Errorf("Regions = %+v, want the nyc region", got.Regions)
			}
		})
	}
}
```

#### Step 2: Run the test, verify it fails

Run: `go test ./internal/tsnet/... -run TestNetCheckResultFromReportSurfacesUDPAndNAT -v`
Expected: FAIL — `netCheckResultFromReport` and `opt` (if unused elsewhere yet) are undefined.

#### Step 3: Implement the data layer

In `internal/tsnet/derp.go`, add `"tailscale.com/types/opt"` to imports, add the new type, change `NetCheck`'s return type, and add the wrapping helper:

```go
// NetCheckResult is the outcome of a live network check: per-DERP-region
// latency plus a UDP/NAT connectivity verdict, both read from the same
// netcheck.Report.
type NetCheckResult struct {
	Regions []DERPRegion

	UDP bool // a UDP STUN round trip completed

	// HardNAT and NATKnown describe report.MappingVariesByDestIP: if the
	// STUN-observed NAT mapping varies by destination, UDP hole-punching is
	// unreliable ("hard"/symmetric NAT). NATKnown is false when netcheck
	// couldn't determine this (the opt.Bool was unset) — HardNAT has no
	// meaning in that case.
	HardNAT  bool
	NATKnown bool
}
```

Change the `NetCheck` signature and every `return nil, ...` / final return inside it:

```go
func (f *Fetcher) NetCheck(ctx context.Context) (NetCheckResult, error) {
	dm, err := f.lc.CurrentDERPMap(ctx)
	if err != nil {
		return NetCheckResult{}, fmt.Errorf("get DERP map: %w", err)
	}
	if dm == nil || len(dm.Regions) == 0 {
		return NetCheckResult{}, fmt.Errorf("no DERP map available from tailscaled")
	}

	bus := eventbus.New()
	defer bus.Close()

	netMon, err := netmon.New(bus, logger.Discard)
	if err != nil {
		return NetCheckResult{}, fmt.Errorf("start network monitor: %w", err)
	}
	defer netMon.Close()

	c := &netcheck.Client{
		NetMon: netMon,
		Logf:   logger.Discard,
	}
	if err := c.Standalone(ctx, ":0"); err != nil {
		return NetCheckResult{}, fmt.Errorf("bind netcheck probe socket: %w", err)
	}

	report, err := c.GetReport(ctx, dm, nil)
	if err != nil {
		return NetCheckResult{}, fmt.Errorf("get netcheck report: %w", err)
	}

	return netCheckResultFromReport(dm, report), nil
}

func netCheckResultFromReport(dm *tailcfg.DERPMap, report *netcheck.Report) NetCheckResult {
	hardNAT, natKnown := report.MappingVariesByDestIP.Get()
	return NetCheckResult{
		Regions:  regionsFromReport(dm, report),
		UDP:      report.UDP,
		HardNAT:  hardNAT,
		NATKnown: natKnown,
	}
}
```

`regionsFromReport` itself is unchanged.

#### Step 4: Run the test, verify it passes

Run: `go test ./internal/tsnet/... -v`
Expected: PASS — including the pre-existing `TestRegionsFromReportSortsAvailableFirstByLatency`, `TestNetCheckPropagatesDERPMapError`, `TestNetCheckRejectsEmptyDERPMap` (these don't inspect the return value beyond its error, so the signature change doesn't require editing them).

#### Step 5: Commit the data layer

```bash
git add internal/tsnet/derp.go internal/tsnet/derp_test.go
git commit -m "feat: surface UDP/NAT verdict from the netcheck report"
```

#### Step 6: Write the failing test for the UI layer

Create `internal/ui/derp_test.go`:

```go
package ui

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/Wian47/ts-hud/internal/tsnet"
)

var derpTestRegions = []tsnet.DERPRegion{
	{Code: "nyc", Name: "New York City", Available: true, Latency: 20 * time.Millisecond},
}

func TestRenderDERPTableShowsLoadingState(t *testing.T) {
	got := renderDERPTable(tsnet.NetCheckResult{}, true, nil, 60)
	if !strings.Contains(got, "checking DERP latency") {
		t.Errorf("renderDERPTable() = %q, want loading message", got)
	}
}

func TestRenderDERPTableShowsError(t *testing.T) {
	got := renderDERPTable(tsnet.NetCheckResult{}, false, errors.New("boom"), 60)
	if !strings.Contains(got, "boom") {
		t.Errorf("renderDERPTable() = %q, want error message", got)
	}
}

func TestRenderDERPTableShowsNoRegions(t *testing.T) {
	got := renderDERPTable(tsnet.NetCheckResult{}, false, nil, 60)
	if !strings.Contains(got, "no DERP regions available") {
		t.Errorf("renderDERPTable() = %q, want no-regions message", got)
	}
}

func TestRenderDERPTableConnectivitySummary(t *testing.T) {
	tests := []struct {
		name   string
		result tsnet.NetCheckResult
		want   string
	}{
		{
			name:   "udp unavailable",
			result: tsnet.NetCheckResult{Regions: derpTestRegions, UDP: false},
			want:   "UDP unavailable",
		},
		{
			name:   "udp ok, hard nat",
			result: tsnet.NetCheckResult{Regions: derpTestRegions, UDP: true, HardNAT: true, NATKnown: true},
			want:   "NAT hard",
		},
		{
			name:   "udp ok, easy nat",
			result: tsnet.NetCheckResult{Regions: derpTestRegions, UDP: true, HardNAT: false, NATKnown: true},
			want:   "NAT easy",
		},
		{
			name:   "udp ok, nat unknown",
			result: tsnet.NetCheckResult{Regions: derpTestRegions, UDP: true, NATKnown: false},
			want:   "NAT unknown",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := renderDERPTable(tt.result, false, nil, 60)
			if !strings.Contains(got, tt.want) {
				t.Errorf("renderDERPTable() = %q, want to contain %q", got, tt.want)
			}
		})
	}
}
```

#### Step 7: Run the test, verify it fails

Run: `go test ./internal/ui/... -run TestRenderDERPTable -v`
Expected: FAIL to compile — `renderDERPTable` still takes `[]tsnet.DERPRegion` as its first argument, not `tsnet.NetCheckResult`.

#### Step 8: Implement the UI layer

In `internal/ui/derp.go`, change `renderDERPTable`'s signature and body to take a `tsnet.NetCheckResult` and render the summary line, and add `connectivitySummary`:

```go
func renderDERPTable(result tsnet.NetCheckResult, loading bool, checkErr error, width int) string {
	var b strings.Builder

	switch {
	case loading:
		b.WriteString(helpStyle.Render("  checking DERP latency…"))
		return b.String()
	case checkErr != nil:
		b.WriteString(errorStyle.Render("  " + checkErr.Error()))
		return b.String()
	case len(result.Regions) == 0:
		b.WriteString(helpStyle.Render("  no DERP regions available"))
		return b.String()
	}

	b.WriteString(connectivitySummary(result))
	b.WriteString("\n\n")

	b.WriteString(headerStyle.Render(formatDERPRow("CODE", "REGION", "LATENCY")))
	b.WriteString("\n")

	for _, r := range result.Regions {
		latency := "—"
		latencyStyle := offlineStyle
		if r.Available {
			latency = r.Latency.Round(time.Millisecond / 10).String()
			latencyStyle = onlineStyle
		}

		name := r.Name
		if r.Preferred {
			name += "  [preferred]"
		}

		row := fmt.Sprintf(
			"%s %s %s",
			padRight(r.Code, colDERPCodeWidth),
			padRight(name, colDERPNameWidth),
			latencyStyle.Render(padRight(latency, colDERPLatencyWidth)),
		)

		style := rowStyle
		if r.Preferred {
			row = fitWidth(row, width)
			style = selectedRowStyle
		}
		b.WriteString(style.Render(row))
		b.WriteString("\n")
	}

	return strings.TrimRight(b.String(), "\n")
}

// connectivitySummary renders a one-line UDP/NAT verdict: whether direct
// UDP peer connections are likely to work, or whether hole-punching is
// unreliable and connections will fall back to relayed DERP (TCP).
func connectivitySummary(result tsnet.NetCheckResult) string {
	switch {
	case !result.UDP:
		return errorStyle.Render("Connectivity: UDP unavailable — DERP (TCP) relay only")
	case result.NATKnown && result.HardNAT:
		return offlineStyle.Render("Connectivity: UDP ok · NAT hard — expect relayed (DERP/TCP) connections")
	case result.NATKnown && !result.HardNAT:
		return onlineStyle.Render("Connectivity: UDP ok · NAT easy — direct connections likely")
	default:
		return onlineStyle.Render("Connectivity: UDP ok · NAT unknown")
	}
}
```

Note the loop body is the pre-existing code, unchanged except reading from `result.Regions` instead of the old `regions` parameter.

Then wire the new type through `internal/ui/model.go`:

- `derpReportMsg` (around line 25): change `regions []tsnet.DERPRegion` to `result tsnet.NetCheckResult`.
- The `Model` struct (around line 49): change `derpRegions []tsnet.DERPRegion` to `derpNetCheck tsnet.NetCheckResult`.
- `derpCheckCmd` (around line 121-128): change `regions, err := fetcher.NetCheck(ctx)` / `return derpReportMsg{regions: regions, err: err}` to `result, err := fetcher.NetCheck(ctx)` / `return derpReportMsg{result: result, err: err}`.
- The `derpReportMsg` case in `Update` (around line 162-166): change `m.derpRegions = msg.regions` to `m.derpNetCheck = msg.result`.
- The `renderDERPTable` call site in `View` (around line 430): change `renderDERPTable(m.derpRegions, m.derpLoading, m.derpErr, width)` to `renderDERPTable(m.derpNetCheck, m.derpLoading, m.derpErr, width)`.

`internal/ui/model_test.go` also constructs `derpReportMsg` literals directly with the old `regions` field (around lines 344 and 374) and will fail to compile otherwise. Update both:

- Line 344: `m.Update(derpReportMsg{regions: regions})` → `m.Update(derpReportMsg{result: tsnet.NetCheckResult{Regions: regions}})`
- Line 374: `m.Update(derpReportMsg{regions: []tsnet.DERPRegion{{Code: "fra"}}})` → `m.Update(derpReportMsg{result: tsnet.NetCheckResult{Regions: []tsnet.DERPRegion{{Code: "fra"}}}})`

#### Step 9: Run the test, verify it passes

Run: `go test ./internal/ui/... -v`
Expected: PASS, including all pre-existing `internal/ui` tests.

Then run the full suite to confirm nothing else references the old signatures:

Run: `go build ./... && go test ./...`
Expected: builds cleanly, all tests PASS.

#### Step 10: Commit the UI layer

```bash
git add internal/ui/derp.go internal/ui/derp_test.go internal/ui/model.go internal/ui/model_test.go
git commit -m "feat: render UDP/NAT connectivity summary in the DERP view"
```

#### Step 11: Update docs

In `README.md`, in the "### DERP latency matrix" section, add a paragraph after the existing "This takes a few seconds, unlike the rest of the UI." line and before the keybinding table:

```markdown
Above the region list, a connectivity line reports whether UDP is reachable
and whether your NAT looks "easy" or "hard" — hard NAT means UDP
hole-punching is unreliable, so expect relayed (DERP/TCP) connections
instead of direct ones.
```

In `ROADMAP.md`, under "### 2. DERP Latency Matrix & Ping Diagnostics", change:

```markdown
- [ ] One-click UDP vs. TCP connection diagnostic test to troubleshoot hard NAT issues.
```

to:

```markdown
- [x] One-click UDP vs. TCP connection diagnostic test to troubleshoot hard NAT issues.
```

Commit:

```bash
git add README.md ROADMAP.md
git commit -m "docs: document the DERP view's UDP/NAT connectivity summary"
```

#### Step 12: Manual verification

This exercises the real `netcheck` probe path — not mockable, matches how the DERP latency matrix itself was verified when first built.

```bash
go build -o /tmp/ts-hud-verify .
/tmp/ts-hud-verify
```

Press `d` to open the DERP view. Expect: after the "checking DERP latency…" moment, a `Connectivity: UDP …` line appears above the `CODE REGION LATENCY` header, colored green (UDP ok, easy/unknown NAT) or red/gray depending on your actual network's NAT behavior, followed by the region table exactly as before. Press `esc` to return to the peer table; press `d` again to re-run and confirm it updates.

#### Step 13: Report back

Summarize what the connectivity line showed on the real tailnet (UDP ok/unavailable, NAT easy/hard/unknown) before considering this task done.
