# ts-hud

A blazing-fast, terminal-native dashboard for your Tailscale mesh network.
Replaces `tailscale status`, adds instant fuzzy search, and lets you SSH
into any peer with one keystroke.

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
| `enter` | SSH into the selected online peer |
| `r` | Manual refresh |
| `q` / `ctrl+c` | Quit |

Peers auto-refresh every 5 seconds by default.

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
