# Preferences Panel Design

**Status:** Approved
**Context:** Second of three sub-projects closing the gap between ts-hud and
the raw `tailscale` CLI. Build order (user-confirmed): Peer diagnostics
(shipped, PR #13) → **Preferences panel (this doc)** → Connection/account
lifecycle.

## Goal

Let a user view and flip the handful of `tailscale set` preferences they
actually touch day-to-day, without leaving ts-hud.

## Scope

`tailscale set` exposes ~20 flags. Most are subnet-router/admin edge cases
(relay server ports and endpoints, netfilter mode, operator user, app
connector advertisement, posture reporting, auto-update, nickname) — not
things a daily user flips from a dashboard. User-confirmed scope is the 5
that are:

1. **SSH server** (`--ssh` / `Prefs.RunSSH`)
2. **Shields up** (`--shields-up` / `Prefs.ShieldsUp`)
3. **Accept routes** (`--accept-routes` / `Prefs.RouteAll`)
4. **Accept DNS** (`--accept-dns` / `Prefs.CorpDNS`)
5. **Advertise as exit node** (`--advertise-exit-node` — not its own pref
   field; see Data Layer)

## Keybinding

`p` (Preferences) on the peer table. Unused today — the existing key set is
`q`/`ctrl+c`, `j`/`k`/`down`/`up`, `g`/`G`, `/`, `enter`, `r`, `x`, `d`, `i`.

## Apply Behavior

Each toggle applies immediately on `enter`/`space` — no staged "save" step.
This matches the exit-node picker's existing behavior (`enter` calls
`EditPrefs` right away) and matches `tailscale set` itself, which has no
staging concept.

## Data Layer

New file `internal/tsnet/prefs.go`. All five setters return whatever
`EditPrefs` hands back — no wrapped/summarized result type, so the UI
always renders the daemon's actual new state rather than a locally-guessed
one.

```go
package tsnet

import (
	"context"

	"tailscale.com/ipn"
	"tailscale.com/net/tsaddr"
	"tailscale.com/types/views"
)

func (f *Fetcher) GetPrefs(ctx context.Context) (*ipn.Prefs, error) {
	return f.lc.GetPrefs(ctx)
}

func (f *Fetcher) SetRunSSH(ctx context.Context, run bool) (*ipn.Prefs, error) {
	return f.lc.EditPrefs(ctx, &ipn.MaskedPrefs{
		Prefs:     ipn.Prefs{RunSSH: run},
		RunSSHSet: true,
	})
}

func (f *Fetcher) SetShieldsUp(ctx context.Context, up bool) (*ipn.Prefs, error) {
	return f.lc.EditPrefs(ctx, &ipn.MaskedPrefs{
		Prefs:        ipn.Prefs{ShieldsUp: up},
		ShieldsUpSet: true,
	})
}

func (f *Fetcher) SetAcceptRoutes(ctx context.Context, accept bool) (*ipn.Prefs, error) {
	return f.lc.EditPrefs(ctx, &ipn.MaskedPrefs{
		Prefs:       ipn.Prefs{RouteAll: accept},
		RouteAllSet: true,
	})
}

func (f *Fetcher) SetAcceptDNS(ctx context.Context, accept bool) (*ipn.Prefs, error) {
	return f.lc.EditPrefs(ctx, &ipn.MaskedPrefs{
		Prefs:       ipn.Prefs{CorpDNS: accept},
		CorpDNSSet:  true,
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

(`netip` import needed alongside the above for `netip.Prefix` in the filter
closure.)

`internal/tsnet/client.go`'s `localClient` interface gains one method:

```go
GetPrefs(ctx context.Context) (*ipn.Prefs, error)
```

(`EditPrefs` is already present; `*local.Client` already implements
`GetPrefs`.) No new go.mod dependency — `tsaddr` and `views` are
subpackages of the already-vendored `tailscale.com` module, same as
`ipnstate`/`tailcfg`/`apitype` before them.

## UI Layer

New file `internal/ui/prefs.go`, rendering a fixed 5-row list in this
order: SSH server, Shields up, Accept routes, Accept DNS, Advertise as exit
node. Each row shows the toggle name and its current state styled with the
existing `onlineStyle` (on/green) or `offlineStyle` (off/gray) — no new
styles, matching every prior view in this codebase.

`internal/ui/model.go` wiring, mirroring the DERP/peer-detail views:

- `viewingPrefs bool`
- `prefsCursor int` — 0-4, clamped the same way `exitNodeCursor` is
- `prefs *ipn.Prefs` — nil until the first fetch resolves
- `prefsLoading bool`
- `prefsErr error`
- `prefsMsg{ prefs *ipn.Prefs; err error }` — one message type reused for
  both the initial `GetPrefs` fetch and every toggle's `EditPrefs` result,
  since both return the same shape
- `prefsFetchCmd(fetcher *tsnet.Fetcher) tea.Cmd` — calls `GetPrefs`, 5s
  timeout matching `fetchCmd`
- Five toggle command functions (`setRunSSHCmd`, `setShieldsUpCmd`,
  `setAcceptRoutesCmd`, `setAcceptDNSCmd`, `setAdvertiseExitNodeCmd`), each
  a thin wrapper calling the matching `Fetcher` setter, 5s timeout, all
  producing `prefsMsg`
- `updateNormal`'s new `"p"` case: opens the view, sets `prefsLoading =
  true`, `prefsCursor = 0`, returns `prefsFetchCmd`
- `updatePrefsView(msg tea.KeyMsg)`: `esc`/`p` closes; `j`/`down` and
  `k`/`up` move `prefsCursor` (clamped 0-4); `enter`/`space` reads
  `m.prefs`'s current value for the row at `m.prefsCursor`, flips it, sets
  `prefsLoading = true`, and returns the matching toggle command (a
  `switch m.prefsCursor { case 0: ...; case 4: ... }`, same concrete style
  as the rest of `model.go` — no function-table abstraction for 5 fixed
  rows); a `nil` `m.prefs` (fetch still in flight or failed) makes
  `enter`/`space` a no-op
- `case prefsMsg:` in `Update`'s message switch: `prefsLoading = false`;
  on success, `prefs = msg.prefs` and `prefsErr = nil`; on error,
  `prefsErr = msg.err` and `prefs` is left as whatever it was (so a failed
  toggle doesn't blank out an already-loaded panel)
- `View()`: new case rendering `renderPrefsPanel(m.prefs, m.prefsCursor,
  m.prefsLoading, m.prefsErr)`, footer `"j/k move  enter toggle  esc/p
  back"`
- Default footer help text gains `p prefs`

## Error Handling

If the initial `GetPrefs` fetch fails, the panel shows the error (reusing
`errorStyle`) instead of the row list — nothing to toggle without a
baseline. If a toggle's `EditPrefs` call fails, the panel keeps showing the
last-known-good state plus an inline error, exactly like the peer-detail
view's per-field error handling; the user can retry by pressing
`enter`/`space` on the same row again.

## Testing

- `internal/tsnet/prefs_test.go`: table tests over a fake `localClient` for
  all 5 setters — `SetRunSSH`/`SetShieldsUp`/`SetAcceptRoutes`/
  `SetAcceptDNS` each assert the right masked field and value reach
  `EditPrefs`. `SetAdvertiseExitNode` gets its own cases: turning on from
  no advertised routes, turning on while a subnet route (e.g.
  `10.0.0.0/8`) is already advertised (must survive untouched), turning
  off (must remove only the two `/0` routes), and the already-in-desired-
  state case (must not call `EditPrefs` at all — verified via a call
  counter on the fake).
- `internal/ui/prefs_test.go`: `renderPrefsPanel` cases for all-off,
  all-on, mixed, loading, and error states.
- `internal/ui/model_test.go`: `p` opens the panel and returns a fetch
  cmd; `esc`/`p` closes it; `j`/`k` move and clamp `prefsCursor` across
  the 5 rows; `enter` on a row with `prefs` loaded sets `prefsLoading =
  true` and returns a non-nil cmd (checked without executing it, matching
  how `TestExitNodePickerEnterSelectsAndReturnsCmd` avoids touching a real
  socket); `enter` with `prefs == nil` is a no-op; `prefsMsg` with an
  error sets `prefsErr` without clearing an already-loaded `prefs`.

## Out of Scope

- The other ~15 `tailscale set` flags not selected above.
- The other two sub-projects (Peer diagnostics, shipped; Connection/
  account lifecycle) — separate specs, per the agreed build order.
