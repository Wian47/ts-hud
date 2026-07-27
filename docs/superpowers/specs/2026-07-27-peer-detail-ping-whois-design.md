# Peer Detail (Ping + WhoIs) Design

**Status:** Approved
**Context:** First of three sub-projects scoped out to close the gap between
ts-hud and the raw `tailscale` CLI, so a daily user never has to drop back
to the terminal. Build order (user-confirmed): **Peer diagnostics (this
doc)** → Preferences panel → Connection/account lifecycle.

## Goal

Let a user diagnose a specific peer — reachability/path and identity/
ownership — without leaving ts-hud, covering what `tailscale ping` and
`tailscale whois` provide today.

## Why Combined, Not Two Views

The peer table already shows hostname, OS, status, and connection type for
every peer. A bare "whois" screen would mostly repeat that. What `whois`
actually *adds* is ownership (which account the device belongs to) and ACL
tags — genuinely new information, best shown alongside a live ping rather
than as its own near-empty screen. One overlay, opened per-peer, covers
both `ping` and `whois` with a single keystroke and a single mental model
(matches the DERP view's open → probe → refresh pattern already
established in this codebase).

## Keybinding

`i` (info) on the peer table, for the currently selected peer — regardless
of online/offline status. Unlike `enter` (ssh, which requires `Online`),
diagnosing why a peer *looks* offline is exactly what this view is for.

No new global state conflicts: `i` is unused today.

## Architecture

`tsnet.Fetcher` gains one new method, `PeerDetail`, which runs a `Ping`
and a `WhoIs` call concurrently against the real LocalAPI client — both
already exist as methods on `*local.Client`
(`tailscale.com/client/local`), so no new dependency. The two halves
degrade independently: if one fails, the view still renders whatever
succeeded, with an inline error only for the half that didn't. There is no
overall hard failure for the view — `PeerDetail` always returns a value,
never an error.

`internal/ui` gains a new overlay (`viewingPeerDetail`), rendered the same
way the DERP view is: loading state while the probe runs, then content,
`r` to re-run, `esc`/`i` to leave.

## Data Layer

New file `internal/tsnet/peerdetail.go`:

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

`internal/tsnet/client.go`'s `localClient` interface gains the two methods
`PeerDetail` depends on:

```go
Ping(ctx context.Context, ip netip.Addr, pingtype tailcfg.PingType) (*ipnstate.PingResult, error)
WhoIs(ctx context.Context, remoteAddr string) (*apitype.WhoIsResponse, error)
```

(`*local.Client` already implements both — this is a pure interface-surface
addition, no behavior change to `NewFetcher`.)

## UI Layer

New file `internal/ui/peerdetail.go`, rendering:

```
<display name>  <OS>  <online/offline>

Owner: <owner or "unknown">
Tags:  <comma-joined tags, or "none">

Ping:  <one of:>
       23.4ms via 100.x.x.x:41641 (direct)
       23.4ms via DERP jnb
       <PingResult.Err> (if the probe ran but couldn't reach the peer)
       <PingErr.Error()> (if the RPC itself failed)
```

Styling reuses the existing palette (`onlineStyle` for a successful direct
ping, `offlineStyle` for a DERP-relayed one, `errorStyle` for any failure)
— no new styles, matching the DERP view precedent.

`internal/ui/model.go` wiring, mirroring the DERP view's fields exactly:

- `viewingPeerDetail bool`
- `peerDetailTarget tsnet.Peer` — captured at open time (`i` press), so the
  overlay keeps showing the peer it was opened for even if the background
  peer-list auto-refresh reorders or updates the underlying table while
  it's open
- `peerDetailResult tsnet.PeerDetail`
- `peerDetailLoading bool`
- `peerDetailReportMsg{ result tsnet.PeerDetail }` — no `err` field, since
  `PeerDetail` never returns one
- `peerDetailCmd(fetcher *tsnet.Fetcher, ip netip.Addr) tea.Cmd` — same
  shape as `derpCheckCmd`, context timeout ~10s
- `updateNormal`'s `"i"` case: requires a selected peer with at least one
  IP (`m.selectedPeer()`); silently no-ops otherwise, matching how `enter`
  already silently no-ops on an offline-peer press
- `updatePeerDetailView(msg tea.KeyMsg)`: `esc`/`i` closes, `r` re-runs

## Testing

- `internal/tsnet/peerdetail_test.go`: table tests over a fake `localClient`
  covering Ping-succeeds/WhoIs-succeeds, Ping-fails/WhoIs-succeeds,
  Ping-succeeds/WhoIs-fails, both-fail, and the `UserProfile.DisplayName`
  vs `LoginName` fallback.
- `internal/ui/peerdetail_test.go`: `renderPeerDetail` cases for direct
  ping, DERP-relayed ping, ping error, whois error, tags present/absent.
- `internal/ui/model_test.go`: `i` opens the view for the selected peer
  (including an offline one), `esc`/`i` closes it, `r` re-triggers the
  probe, and the captured `peerDetailTarget` survives a background peer
  refresh while the view is open.

## Out of Scope

- Whois/ping against an arbitrary IP not already in the peer list (the CLI
  supports this; ts-hud's peer table is a closed set, so this isn't a gap
  for a "leave the CLI behind" use case).
- The other two sub-projects (Preferences panel; Connection/account
  lifecycle) — separate specs, per the agreed build order.
