# Connection & Account Lifecycle Design

**Status:** Approved
**Context:** Third of three sub-projects closing the gap between ts-hud and
the raw `tailscale` CLI. Build order (user-confirmed): Peer diagnostics
(shipped, PR #13) → Preferences panel (shipped, PR #14) →
**Connection & account lifecycle (this doc)**.

## Goal

Let a user bring the Tailscale connection up/down and switch between
already-authenticated accounts without leaving ts-hud.

## Scope

`tailscale` exposes five lifecycle actions: `up`, `down`, `login`,
`logout`, `switch`. They split into two very different risk profiles:

- **up / down / switch**: reversible, fully in-TUI, no external steps.
  `down` is an `EditPrefs{WantRunning: false}` call; `up` is the same with
  `true`; `switch` moves between accounts that are already authenticated
  on this device — no new auth flow.
- **login / logout**: `login` requires surfacing an `AuthURL` for the user
  to open in an external browser (no way to complete it purely inside a
  terminal keystroke); `logout` invalidates the current node key,
  forcing re-authentication — a destructive, non-undoable action.

User-confirmed scope for this sub-project is **up / down / switch only**.
`login`/`logout` are explicitly deferred to a possible future sub-project.

## Keybindings

- `c` (Connection) — toggle `WantRunning`.
- `a` (Accounts) — open the account-switch overlay.

Both unused today — the existing key set is `q`/`ctrl+c`, `j`/`k`/`down`/
`up`, `g`/`G`, `/`, `enter`, `r`, `x`, `d`, `i`, `p`.

## Data Layer

New file `internal/tsnet/connection.go`:

```go
package tsnet

import (
	"context"

	"tailscale.com/ipn"
)

func (f *Fetcher) SetWantRunning(ctx context.Context, running bool) (*ipn.Prefs, error) {
	return f.lc.EditPrefs(ctx, &ipn.MaskedPrefs{
		Prefs:          ipn.Prefs{WantRunning: running},
		WantRunningSet: true,
	})
}

func (f *Fetcher) ListProfiles(ctx context.Context) (current ipn.LoginProfile, all []ipn.LoginProfile, err error) {
	return f.lc.ProfileStatus(ctx)
}

func (f *Fetcher) SwitchProfile(ctx context.Context, id ipn.ProfileID) error {
	return f.lc.SwitchProfile(ctx, id)
}
```

`internal/tsnet/client.go`'s `localClient` interface gains two methods:

```go
ProfileStatus(ctx context.Context) (current ipn.LoginProfile, all []ipn.LoginProfile, err error)
SwitchProfile(ctx context.Context, profile ipn.ProfileID) error
```

(`*local.Client` already implements both; no new go.mod dependency.)

`Fetcher.Fetch` grows a return value to surface the backend's connection
state, which it currently discards from `ipnstate.Status`:

```go
func (f *Fetcher) Fetch(ctx context.Context) (peers []Peer, self *Peer, backendState string, err error)
```

`backendState` is `status.BackendState` verbatim (one of `"NoState"`,
`"NeedsLogin"`, `"NeedsMachineAuth"`, `"Stopped"`, `"Starting"`,
`"Running"`). `Fetch` has exactly one call site today
(`internal/ui/model.go`'s `fetchCmd`) and no direct unit test of its own
(only `peersFromStatus` is tested separately), so this signature change is
mechanical, not a compatibility concern. The CLI-fallback path
(`statusFromCLI`) already populates `ipnstate.Status.BackendState` via the
same JSON unmarshal, so no extra work is needed there.

## UI Layer

### Header

`internal/ui/model.go`'s `renderHeader` appends the backend state, styled
with `errorStyle`, whenever it isn't the common case:

```
ts-hud  self: my-laptop (100.64.0.1)  peers: 12  [STOPPED]
```

Nothing is appended when `backendState == "Running"` — today's header is
unchanged in the common case. `"Starting"` gets the same treatment as
`"Stopped"` (transitional, not yet serving traffic). This makes it obvious
the peer list below may be stale when the connection isn't up.

### Connection toggle (`c`)

No new full-screen overlay — this is a header-state-plus-footer-prompt
interaction, since there's nothing to list or navigate.

`Model` gains `confirmingDown bool`. Pressing `c`:

- `backendState == "Running"`: sets `confirmingDown = true`. Footer becomes
  `"Bring Tailscale down? y confirm  n/esc cancel"`. Only `y` proceeds
  (calls `SetWantRunning(false)`, clears `confirmingDown`, sets a loading
  flag); `n` or `esc` clears `confirmingDown` with no other effect.
- `backendState == "Stopped"` (or `"Starting"`): calls `SetWantRunning(true)`
  immediately, no confirmation — matches the preferences panel's
  immediate-apply philosophy for the safe direction.
- `backendState` is `"NeedsLogin"`, `"NeedsMachineAuth"`, or `"NoState"`:
  no-op other than setting an inline error, e.g. `"not logged in — run
  tailscale login"` (rendered with `errorStyle` in the footer area, cleared
  on the next keypress). No `EditPrefs` call is attempted, since it can't
  succeed without an authenticated node key.

### Account switch overlay (`a`)

New file `internal/ui/accounts.go`:

```go
func renderAccountsPanel(current ipn.LoginProfile, all []ipn.LoginProfile, cursor int, loading bool, err error, width int) string
```

Same skeleton as the exit-node picker (`internal/ui/table.go`'s
`renderExitNodePicker`): header `"Switch account"` (`headerStyle`), one row
per profile using `profile.Name` (falling back to
`profile.NetworkProfile.DisplayNameOrDefault()` if `Name` is empty), the
current profile's row suffixed with `(current)`, cursor row highlighted via
the existing `rowStyle`/`selectedRowStyle` + `fitWidth` convention. Empty
`all` shows `"no accounts — run tailscale login"` instead of a blank list.
Loading blanks to `"loading accounts…"`. A fetch error replaces the list
entirely (no baseline to show, matching the prefs panel's initial-fetch
error case).

`internal/ui/model.go` wiring, mirroring the prefs-panel wiring:

- `viewingAccounts bool`, `accountsCursor int`, `accountsCurrent
  ipn.LoginProfile`, `accountsAll []ipn.LoginProfile`, `accountsLoading
  bool`, `accountsErr error`
- `accountsMsg{ current ipn.LoginProfile; all []ipn.LoginProfile; err error
  }` for the fetch; `switchProfileMsg{ err error }` for the switch result
  (switching doesn't return a new list — the panel re-fetches via the same
  `accountsFetchCmd` after a successful switch, so the `(current)` marker
  updates)
- `accountsFetchCmd(fetcher *tsnet.Fetcher) tea.Cmd` — calls `ListProfiles`,
  5s timeout matching every other fetch cmd
- `switchProfileCmd(fetcher *tsnet.Fetcher, id ipn.ProfileID) tea.Cmd` — calls
  `SwitchProfile`, 5s timeout
- `updateNormal`'s new `"a"` case: opens the view, `accountsLoading = true`,
  `accountsCursor = 0`, returns `accountsFetchCmd`
- `updateAccountsView(msg tea.KeyMsg)`: `esc`/`a` closes; `j`/`k` move+clamp
  `accountsCursor` across `len(accountsAll)`; `enter` on a row calls
  `switchProfileCmd` with that row's `ID` immediately (no confirmation —
  reversible, consistent with the exit-node picker)
- `case accountsMsg:` and `case switchProfileMsg:` in `Update`'s message
  switch, following the same "error keeps last-known-good state" pattern as
  `prefsMsg`; a successful switch triggers a follow-up `accountsFetchCmd` to
  refresh the `(current)` marker
- `View()`: new case rendering `renderAccountsPanel(...)`, footer `"j/k
  move  enter switch  esc/a back"`
- Default footer help text gains `"c connection  a accounts"`

## Error Handling

- `SetWantRunning` failure: keeps last-known `backendState` (from the next
  periodic refresh), shows an inline error, clears `confirmingDown` if it
  was set.
- `ListProfiles` failure (initial fetch): accounts panel shows the error
  instead of the row list.
- `SwitchProfile` failure: accounts panel keeps showing the last-loaded
  list plus an inline error; user can retry `enter` on the same row.
- No bespoke IPN-bus watch loop after a toggle or switch — state updates
  arrive on the next periodic 5s refresh, consistent with how every other
  overlay in this app already works (the real CLI's `up`/`switch` block
  until fully connected; ts-hud accepts eventual consistency instead).

## Testing

- `internal/tsnet/connection_test.go`: `SetWantRunning` true/false against
  a fake `localClient`, asserting `WantRunningSet`/`WantRunning` reach
  `EditPrefs`; `ListProfiles`/`SwitchProfile` thin-wrapper pass-through
  tests against fake return values.
- `internal/ui/accounts_test.go`: `renderAccountsPanel` cases for empty,
  single profile, multiple profiles with current-marking, loading, and
  error states.
- `internal/ui/model_test.go`: `c` while `backendState == "Running"` sets
  `confirmingDown` and returns no cmd yet; `y` while confirming calls
  `SetWantRunning(false)` and returns a non-nil cmd; `n`/`esc` while
  confirming clears the flag and returns no cmd; `c` while `"Stopped"`
  returns a non-nil cmd immediately with no confirm step; `c` while
  `"NeedsLogin"` sets an inline error and returns no cmd; `a` opens the
  accounts overlay and returns a fetch cmd; `enter` on a profile row
  returns a switch cmd; `renderHeader` includes the `[STATE]` suffix
  exactly when `backendState != "Running"` and omits it otherwise.

## Out of Scope

- `login`/`logout` — deferred; different risk profile (destructive
  node-key invalidation, out-of-band browser URL requirement). Possible
  future sub-project.
- `switch remove` (deleting a stored profile) and adding brand-new
  accounts (`tailscale login` onto an unauthenticated device).
- Any bespoke state-watch loop beyond the existing periodic refresh.
- The other two sub-projects (Peer diagnostics, Preferences panel) —
  already shipped, separate specs.
