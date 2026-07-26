# Embedded SSH Pane — Design

## Problem

`ts-hud` currently ssh's into a peer by suspending Bubble Tea and handing the
whole terminal to a real `ssh` subprocess via `tea.ExecProcess` — a clean
full-screen handoff, but it leaves the app's own chrome (bordered frame,
header, footer) entirely. The ask is for the ssh session to visibly run
*inside* the ts-hud TUI: same bordered frame, header/footer chrome staying
in place, ssh output rendered live in the body.

This is a UX upgrade to the "Seamless SSH Integration" capability already
shipped in Phase 1 (`ROADMAP.md`), not a new roadmap line item. Target:
v0.5.0 (MINOR — new capability, no breaking change).

## Scope decisions (from brainstorming)

- **Full pane, chrome stays**: ssh fills the whole body area; peer list is
  hidden while connected. No split/tmux-style side-by-side layout.
- **Kill on detach**: one session at a time, no background/reattach session
  management. Detaching ends the ssh connection, same as exiting the shell.
- **Detach key is `ctrl+q`**, not `esc` — `esc` must reach the remote session
  untouched (vim, less, fzf, shell vi-mode all rely on it).
- Key forwarding is **best-effort**, not byte-perfect: printable runes,
  enter/tab/backspace/esc, arrows, ctrl+letter, home/end/pgup/pgdn. Mouse
  events and bracketed paste are out of scope for v1.
- Resize forwarding (SIGWINCH-equivalent via `pty.Setsize` + emulator
  `Resize`) is in scope — remote full-screen apps (vim, htop, less) must
  redraw correctly when the terminal window resizes.

## Architecture

New dependencies:
- `github.com/creack/pty` — spawns `ssh` attached to a real OS pty. Standard,
  widely used (docker CLI, gh CLI), low risk.
- `github.com/charmbracelet/x/vt` (`vt.SafeEmulator`) — Charm's own
  concurrency-safe VT100 emulator. `Write([]byte)` feeds it raw pty output;
  `Render() string` returns an ANSI-styled snapshot sized to exact
  rows×cols, ready to embed as the frame body via the existing `fitWidth`
  path in `frame.go`. Caveat: pre-v1, pseudo-versioned (no tagged releases)
  — pinned via `go.sum` as usual, but a future `go get -u` could pull a
  breaking API change from upstream.

New file `internal/ui/sshpane.go`:
- A narrow injectable interface for spawning the pty (mirrors the
  `localClient` DI pattern already used in `internal/tsnet/client.go` for
  testability):
  ```go
  type ptySpawner interface {
      Start(cmd *exec.Cmd) (ptySession, error)
  }
  type ptySession interface {
      io.ReadWriteCloser
      Setsize(rows, cols int) error
  }
  ```
- `sshPane` struct: holds the spawned session, the `*vt.SafeEmulator`, the
  output channel, and the last error (if any).
- `Model` gains `viewingSSH bool` and `sshPane *sshPane`, following the same
  shape as `viewingDERP`/`pickingExitNode`.

`ssh.go` (the old `tea.ExecProcess` full-takeover path — `buildSSHCommand`,
`sshCmd`, `sshFinishedMsg`) is deleted entirely; this feature replaces it,
not sits alongside it. `Peer.SSHTarget()` is reused unchanged for building
the `ssh` command's argument.

## Data flow

**Output (remote → screen):**
A background goroutine loops `session.Read(buf)`, pushing each chunk onto a
channel. A `tea.Cmd` (`waitForPtyOutput`, the standard Bubble Tea streaming
pattern — block on a channel, return a message, get re-issued) returns one
`sshOutputMsg{data}` per chunk. `Update` writes the chunk into the
`SafeEmulator` and re-issues the wait-cmd. `View()` renders the body via
`pane.term.Render()` on every frame — no extra throttling/ticker; chunking
is already bounded by the OS read size.

**Input (keystrokes → remote):**
While `viewingSSH`, `tea.KeyMsg` bypasses the normal `updateNormal`
dispatch entirely. A hand-rolled `keyMsgToBytes(msg tea.KeyMsg) []byte`
encodes the supported key set back into raw bytes written straight to the
pty session. `ctrl+q` is intercepted *before* this encoding (never
forwarded) and triggers detach.

**Detach (`ctrl+q`):** kill the ssh process, close the pty session, return
to the peer table.

**Remote exit:** goroutine hits `io.EOF` on the pty read → sends
`sshClosedMsg{err}` → same return-to-table path (auto-detach). Non-nil
`err` (e.g. connection refused before a pty ever came up) surfaces via the
existing `errorStyle` footer treatment.

**Resize:** `tea.WindowSizeMsg` while `viewingSSH` calls `session.Setsize`
and `term.Resize(cols, rows)`. Requires a new `contentHeight()` helper in
`frame.go` mirroring the existing `contentWidth()` — today only width is
threaded down to body renderers; the pane needs an exact row count too so
it fills the frame precisely.

## Error handling

- Spawn failure (`pty.Start` fails — e.g. `ssh` not on `$PATH`): surfaces
  immediately as a footer error via `errMsg`-style handling; no pane opens,
  stays on the peer table.
- Mid-session errors (connection drop, non-zero exit): `sshClosedMsg{err}`
  → footer error → peer table, same shape as spawn failure.

## Testing

- `keyMsgToBytes` and the new `contentHeight` sizing math are pure
  functions — fully TDD-able, table-driven tests.
- The pane's `Update` state machine (start / output / detach / remote-exit
  transitions) is tested via the injected `ptySpawner`/`ptySession`
  interfaces with a fake, same DI pattern as `fakeLocalClient` in
  `internal/tsnet/client_test.go`.
- The actual live ssh session (real remote shell, real colors, resize
  behavior against vim/htop/less) is verified manually over tmux against
  the real tailnet — consistent with how the DERP latency matrix and
  exit-node picker were verified in this project.

## Roadmap

`ROADMAP.md`'s Phase 1 "Seamless SSH Integration" bullet gets a note that
it now runs embedded in the TUI rather than as a full-terminal handoff, no
new top-level roadmap entry.
