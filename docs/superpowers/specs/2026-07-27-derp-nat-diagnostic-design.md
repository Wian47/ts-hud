# DERP UDP/NAT Diagnostic Design

**Status:** Approved
**Roadmap item:** Phase 2, "DERP Latency Matrix & Ping Diagnostics" — "One-click UDP vs. TCP connection diagnostic test to troubleshoot hard NAT issues."

## Goal

Surface a one-line connectivity verdict (UDP reachability + NAT hardness) in
the existing DERP latency view, so a user pressing `d` can immediately tell
whether they're likely to get direct UDP peer connections or are stuck
falling back to relayed DERP-over-TCP.

## Architecture

The DERP view already runs a full `netcheck` (real STUN probes, same
machinery `tailscale netcheck` uses) via `tsnet.Fetcher.NetCheck` — it just
discards everything from the `netcheck.Report` except per-region latency.
This change extends that same call to also surface two fields the report
already computes, with no additional probing or latency cost:

- `report.UDP` — whether a UDP STUN round-trip succeeded at all.
- `report.MappingVariesByDestIP` (`opt.Bool`) — whether the STUN-observed
  NAT mapping varies by destination. If it varies, UDP hole-punching is
  unreliable (symmetric/"hard" NAT); if unset, we don't have a verdict yet.

No new keybinding, no new dependency, no new background probing — this is a
render-layer addition on top of an existing network call.

## Data Layer

`internal/tsnet/derp.go`: `NetCheck` currently returns `([]DERPRegion,
error)`. It changes to return a single result struct instead:

```go
type NetCheckResult struct {
	Regions  []DERPRegion
	UDP      bool // a UDP STUN round trip completed
	HardNAT  bool // NAT mapping varies by destination (hole-punching unreliable)
	NATKnown bool // whether HardNAT is a real verdict (report.MappingVariesByDestIP was set)
}

func (f *Fetcher) NetCheck(ctx context.Context) (NetCheckResult, error)
```

`HardNAT`/`NATKnown` are read via `report.MappingVariesByDestIP.Get()`
(`opt.Bool.Get() (v bool, ok bool)`).

## UI Layer

`internal/ui/model.go`: `derpReportMsg` carries `NetCheckResult` instead of
`regions []tsnet.DERPRegion`; `m.derpRegions` is replaced by
`m.derpNetCheck tsnet.NetCheckResult` (zero value renders nothing extra,
same as today's `nil` slice).

`internal/ui/derp.go`: `renderDERPTable` gains a summary line rendered above
the existing `CODE REGION LATENCY` header, once a result is available (not
during `loading`, not on `checkErr`):

- UDP ok, NAT known easy: `Connectivity: UDP ok · NAT easy — direct connections likely` (`onlineStyle`)
- UDP ok, NAT known hard: `Connectivity: UDP ok · NAT hard — expect relayed (DERP/TCP) connections` (`offlineStyle` or `errorStyle`)
- UDP ok, NAT unknown: `Connectivity: UDP ok · NAT unknown` (`onlineStyle` for the UDP half only, or plain)
- UDP unavailable: `Connectivity: UDP unavailable — DERP (TCP) relay only` (`errorStyle`)

Exact style constants reused from the existing palette in `internal/ui/styles.go`
(`onlineStyle`, `offlineStyle`, `errorStyle`) — no new styles.

## Testing

- `internal/tsnet/derp_test.go`: extend `regionsFromReport`-equivalent
  coverage (or a new `netCheckResultFromReport` helper) with table cases for
  UDP true/false and `MappingVariesByDestIP` true/false/unset.
- `internal/ui/derp_test.go`: extend `renderDERPTable` tests (or add if none
  exist yet — check current test file) for the four summary-line variants
  above.

## Out of Scope

- Self-Node Telemetry (Hairpinning, UPnP/NAT-PMP/PCP port mapping) — that's
  a separate Phase 1 roadmap bullet, not implemented yet, and not part of
  this change.
- Any new keybinding or separate diagnostic screen — this is additive to
  the existing DERP view only, per the approved design discussion.
