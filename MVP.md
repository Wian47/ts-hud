# MVP Development Plan: `ts-hud` (v0.1.0)

## 🎯 Goal
Deliver a blazing-fast, terminal-native dashboard that gives instantaneous visibility over your Tailscale mesh network, replaces basic `tailscale status` calls, and lets you SSH into any node with a single keystroke.

---

## 🏗️ Core Stack & Architecture
* **Language:** Go 1.22+
* **TUI Framework:** `github.com/charmbracelet/bubbletea` (Elm Architecture)
* **Styling Engine:** `github.com/charmbracelet/lipgloss`
* **Tailscale Interface:** Tailscale Go SDK (`tailscale.com/client/local`) with CLI JSON fallback (`tailscale status --json`)

---

## 🚀 Scope: Included vs. Excluded in MVP

### In Scope (v0.1)
* [x] **Peer Network Table:** Hostname, Tailscale IPv4/IPv6, OS icon/label, and Online/Offline status badge.
* [x] **Connection Type Indicator:** Identify if a node is connected **Direct**, relayed via **DERP**, or **Peer-Relayed**.
* [x] **Fuzzy Filter / Search:** Press `/` to search peers by name, OS, or IP address instantly.
* [x] **Interactive SSH (`tea.ExecProcess`):** Select any online node and press `Enter` to suspend the TUI and spawn an active SSH session to that device.
* [x] **Manual & Auto-Refresh:** Press `r` to refresh on demand; auto-poll background status every 5 seconds.
* [x] **Vim Keybindings:** Full navigation with `j`/`k`, `g`/`G`, and `/`.

### Out of Scope (Saved for Phase 2+)
* ❌ Modifying network state (switching exit nodes, toggling Tailscale up/down).
* ❌ Real-time packet ping history charts.
* ❌ Taildrop UI file manager.

---

## 📅 5-Day Implementation Timeline

| Day | Focus Area | Key Deliverables |
|---|---|---|
| **Day 1** | **Data Layer & Client SDK** | Connect to `LocalClient` socket, parse `Status` struct, and build background ticker command. |
| **Day 2** | **Core UI & Styling** | Lay out table view using `Lip Gloss`, add color badges for Direct vs. DERP vs. Offline nodes. |
| **Day 3** | **Filtering & Navigation** | Add text input component for fuzzy searching and handle list cursor state bounds. |
| **Day 4** | **Subprocess Execution** | Implement `tea.ExecProcess` to handle clean SSH context switching and terminal restoration. |
| **Day 5** | **Polish & Packaging** | Add standard CLI flags (`--refresh-rate`, `--version`), double-check edge cases, write `README.md`. |

---

## 🛠️ Definition of Done (MVP Release Checklist)
- [ ] Binary compiles cleanly with zero CGO dependencies (`CGO_ENABLED=0 go build`).
- [ ] Handles offline or unauthenticated Tailscale state gracefully without crashing.
- [ ] Launches in under 15ms.
- [ ] Includes `goreleaser` setup for automated GitHub binary releases (Linux/macOS/BSD).
