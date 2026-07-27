# ts-hud

[![Release](https://img.shields.io/github/v/release/Wian47/ts-hud)](https://github.com/Wian47/ts-hud/releases)
[![Go Version](https://img.shields.io/github/go-mod/go-version/Wian47/ts-hud)](go.mod)
[![License: MIT](https://img.shields.io/github/license/Wian47/ts-hud)](LICENSE)

A blazing-fast, terminal-native dashboard for your Tailscale mesh network.
Replaces `tailscale status`, adds instant fuzzy search, and lets you SSH
into any peer with one keystroke.

<!-- TODO: demo GIF/asciinema cast here -->

## Why not just `tailscale status`?

- **Live, searchable, keyboard-driven** — fuzzy-filter peers by hostname, OS, or IP as you type, instead of scrolling a flat text dump.
- **SSH in one keystroke** — `enter` on a peer drops you into an embedded ssh session, live inside the same frame, not a separate terminal handoff.
- **Built-in diagnostics** — a DERP latency matrix with a UDP/NAT connectivity verdict (`d`), and an exit-node switcher (`x`), no `tailscale` subcommands to memorize.
- **Auto-refreshing** — the peer table updates itself every 5 seconds; no re-running a command to see what changed.

## Install

```bash
go install github.com/Wian47/ts-hud@latest
```

Or download a prebuilt binary from the [Releases](https://github.com/Wian47/ts-hud/releases) page (Linux, macOS, FreeBSD; x86_64/arm64).

## Usage

```bash
ts-hud
```

| Key | Action |
|---|---|
| `j` / `k` | Move down / up |
| `g` / `G` | Jump to top / bottom |
| `/` | Fuzzy search by hostname, OS, or IP |
| `esc` | Clear search and exit search mode |
| `enter` | SSH into the selected online peer, embedded in the TUI |
| `x` | Open the exit node picker |
| `d` | Open the DERP latency matrix |
| `i` | Open peer detail (live ping + owner/tags) for the selected peer |
| `p` | Open the preferences panel (SSH, shields up, accept routes/DNS, advertise exit node) |
| `c` | Bring the Tailscale connection up, or down (with confirmation) |
| `a` | Open the account-switch overlay |
| `r` | Manual refresh |
| `q` / `ctrl+c` | Quit |

Peers auto-refresh every 5 seconds by default.

### Exit node picker

Press `x` to open a list of peers advertising as exit nodes.

| Key | Action |
|---|---|
| `j` / `k` | Move down / up |
| `enter` | Select the highlighted peer as your exit node, or clear it via the `(none)` entry |
| `l` | Toggle "allow LAN access" (applied when you select a node) |
| `esc` / `x` | Cancel without changing anything |

### Embedded ssh session

Press `enter` on an online peer to ssh into it — the session runs live
inside the ts-hud frame (header/footer chrome stays put), not as a
separate full-terminal handoff.

| Key | Action |
|---|---|
| `ctrl+q` | Detach — ends the ssh session and returns to the peer table |

Detaching (or the remote session ending on its own) always terminates that
ssh connection; there's no background/reattach. Resizing your terminal
while connected resizes the remote session too.

### DERP latency matrix

Press `d` to run a live network check (real STUN probes, same as `tailscale
netcheck`) and see latency to every DERP relay region, fastest first, with
your current home region marked `[preferred]`. This takes a few seconds,
unlike the rest of the UI.

Above the region list, a connectivity line reports whether UDP is reachable
and whether your NAT looks "easy" or "hard" — hard NAT means UDP
hole-punching is unreliable, so expect relayed (DERP/TCP) connections
instead of direct ones. The line may also read "UDP unavailable" (DERP/TCP
relay only) or "NAT unknown" (when netcheck couldn't determine NAT hardness,
typical on IPv6-only paths).

| Key | Action |
|---|---|
| `r` | Re-run the check |
| `esc` / `d` | Back to the peer table |

### Peer detail

Press `i` on any peer (online or offline) to see a live ping and whois
lookup: latency and path (direct, DERP-relayed, or unreachable), plus
which account owns the device and its ACL tags.

| Key | Action |
|---|---|
| `r` | Re-run the probe |
| `esc` / `i` | Back to the peer table |

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

## Flags

| Flag | Default | Description |
|---|---|---|
| `--refresh-rate` | `5s` | Background auto-refresh interval |
| `--version` | | Print version and exit |

## Connection types

- **Direct** — a direct peer-to-peer WireGuard path.
- **DERP(region)** — relayed through a Tailscale DERP server in the given region.
- **Peer-Relay** — relayed through another peer.

## Building from source

```bash
git clone https://github.com/Wian47/ts-hud
cd ts-hud
make build
```

Requires Go 1.26.4+ (see `go.mod` — `tailscale.com` itself requires it).

## Roadmap

See [ROADMAP.md](ROADMAP.md) for what's planned next (Taildrop TUI, Serve/Funnel
manager, key expiry warnings, and more).

## Contributing

Issues and PRs welcome — this is a young project and feedback on what's
actually useful day-to-day is worth more than feature requests in the
abstract.

## License

[MIT](LICENSE)
