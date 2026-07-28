# Help Overlay Design

## Problem

ts-hud has grown to 9 keys across the peer table and its sub-panels (search,
ssh session, exit node picker, DERP matrix, peer detail, prefs panel,
connection confirm, account switch). The main footer lists the top-level
keys, but sub-panel-specific keys (e.g. `l` toggle LAN access in the exit
node picker, `y`/`n` in the connection-down confirm) are only visible once
you're already inside that panel. There's no single place to see everything
at once.

## Solution

A `?` key opens a static help overlay listing every keybinding in the app,
grouped by section, mirroring the README's existing per-section tables.
`esc` or `?` closes it and returns to exactly whatever screen was open
underneath — opening help doesn't touch the underlying screen's state, it
only layers on top.

### Where it works

`?` is live everywhere except:
- **Search input** (`m.searching`) — every keystroke is search text.
- **Embedded ssh session** (`m.viewingSSH`) — every keystroke is forwarded
  to the remote shell. (This is why ssh already uses `ctrl+q` instead of
  `esc` to detach — same reasoning applies to `?`.)

Everywhere else — the peer table itself, and any of the DERP/peer-detail/
prefs/connection-confirm/accounts/exit-node sub-panels — `?` opens the
overlay on top of that panel.

### Content

Static text, grouped by section, one section per existing panel:

- Peer table (`j/k`, `g/G`, `/`, `enter`, `x`, `d`, `i`, `p`, `c`, `a`, `r`,
  `q`)
- Search (`esc` clear and exit)
- Ssh session (`ctrl+q` detach)
- Exit node picker (`j/k`, `enter`, `l`, `esc/x`)
- DERP latency matrix (`r`, `esc/d`)
- Peer detail (`r`, `esc/i`)
- Preferences panel (`j/k`, `enter`, `esc/p`)
- Connection-down confirm (`y`, `n/esc`)
- Account switch (`j/k`, `enter`, `esc/a`)

This list is the source of truth for both the overlay and the README's
keybinding tables — if a key's behavior changes, both need updating (no
new coupling here, they're already kept in sync by hand).

## Implementation

**State:** one new `Model` field, `viewingHelp bool`.

**Routing:** a single interception point at the top of `Update()`'s
`tea.KeyMsg` case, before the existing view-dispatch switch:

```go
case tea.KeyMsg:
    if msg.String() == "?" && !m.searching && !m.viewingSSH {
        m.viewingHelp = !m.viewingHelp
        return m, nil
    }
    switch {
    case m.viewingHelp:
        return m.updateHelpView(msg)
    case m.searching:
        ...
```

`updateHelpView` handles only `esc`/`?` to close (`m.viewingHelp = false`).
Because opening help doesn't clear any other `viewing*`/`picking*`/
`confirming*` flag, closing it resumes exactly where the user was — no
extra state to save/restore.

`m.viewingHelp` is also checked first in `View()`'s render switch, so the
overlay draws on top of whatever the underlying panel would have rendered.

**Rendering:** `renderHelpPanel(width int) string` in a new
`internal/ui/help.go`, styled like `renderAccountsPanel`/`renderPrefsPanel`
(bordered panel, `headerStyle` section headers, `rowStyle` rows) but
entirely static — no loading state, no error state, no cursor, since the
content never changes at runtime.

**Footer:** add `? help` to the main peer-table footer hint
(`model.go:781`) so the feature is discoverable.

**Docs:** add a "### Help overlay" section to README.md following the
existing per-panel subsection pattern, plus a `?` row in the main
keybinding table.

## Testing

- `updateHelpView`: `?`/`esc` close it; opening from the peer table,
  and from at least one sub-panel (e.g. DERP view), preserves that
  sub-panel's own state after closing.
- `?` is a no-op (does not toggle) while `m.searching` or `m.viewingSSH`.
- `renderHelpPanel` produces non-empty output containing each section
  header once.

## Out of scope

- Context-sensitive help (only the current panel's keys) — rejected in
  favor of a single global cheat-sheet (see prior discussion).
- Scrolling/pagination — the full list fits on one screen at typical
  terminal sizes; revisit if the keyspace grows enough to not fit.
