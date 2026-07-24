# Product Roadmap: `ts-hud`

> **Vision:** To create the undisputed best terminal utility for Tailscale—combining real-time network topology, edge service inspection, and zero-trust access management into a keyboard-driven interface.

---

## 🗺️ Strategic Phase Overview

[ Phase 1: Core HUD ] ──► [ Phase 2: Operations ] ──► [ Phase 3: Security ] ──► [ Phase 4: Platform Ecosystem ]
(v0.1 MVP - Released)     (Exit Nodes, DERP, Serve)    (Keys, ACLs, Locks)       (Tmux, Daemon, Headless)

---

## Phase 1: Core HUD & Node Management (v0.1.0 – v0.3.0)
*Focus: Read-only network visibility and instant client actions.*

* **Status Dashboard & Search:** Filter peers by tag, OS, online state, or IP subnet.
* **Seamless SSH Integration:** Execute SSH sessions directly into peers with automatic port/user detection.
* **Self-Node Telemetry:** Inspect local machine's public IP mapping, Hairpinning support, and PortMapping types (UPnP, NAT-PMP, PCP).
* **Lightweight Binary Distribution:** Native support for Linux (x86/ARM), macOS, and FreeBSD.

---

## Phase 2: Mesh Operations & Connectivity (v0.4.0 – v0.6.0)
*Focus: Deep network operations, traffic routing, and built-in service sharing.*

### 1. Dynamic Exit Node Switcher ✅
* Inspect all advertised exit nodes across the tailnet.
* One-key toggle to route internet traffic through a selected exit node with **Allow LAN Access** checkbox.

### 2. DERP Latency Matrix & Ping Diagnostics
* Live latency graph across all Tailscale DERP relay locations (e.g., `nyc`, `fra`, `tok`, `syd`).
* One-click UDP vs. TCP connection diagnostic test to troubleshoot hard NAT issues.

### 3. Taildrop TUI File Manager
* Terminal file picker integration to send files or directories over Taildrop.
* Inbox receiver view showing incoming files with quick actions (`Accept`, `Overwrite`, `Rename`).

### 4. Tailscale Serve & Funnel Manager
* Interactive inspector for active local HTTP proxies, file servers, and static directories exposed via `tailscale serve` / `tailscale funnel`.
* Create or tear down Funnels and Serve instances directly inside the UI.

---

## Phase 3: Security, Governance & Audit (v0.7.0 – v0.9.0)
*Focus: Access control visibility, key management, and security posture.*

* **Key Expiry Radar:** Banner warnings for peer device keys nearing expiration.
* **Tailnet Lock Inspector:** View signing key trusted state and verify locked tailnet signatures.
* **Application Capability Inspector:** View app capability grants (e.g., `tailscale.com/cap/drive` or custom caps) attached to active connections.
* **Admin Console Direct Jumps:** Quick hotkeys (`Shift+A`) to open selected device pages in the web Admin console.

---

## Phase 4: Power Integrations & Ecosystem (v1.0.0+)
*Focus: Workflow integration and developer experience.*

* **Tmux Floating Popup Mode:** Native helper script (`tmux-ts-hud`) optimized for tmux floating popups (`tmux display-popup`).
* **Headless Background Daemon:** Background system worker that fires native desktop notifications when:
  * A new device joins the tailnet.
  * An exit node becomes unreachable.
  * A device key expires in less than 24 hours.
* **Theme Engine:** Built-in support for popular terminal palettes (Catppuccin, Tokyo Night, Nord, Gruvbox, Dracula).
* **LocalSocket Realtime Streaming:** Move from periodic polling to streaming state updates via the LocalClient API for zero-latency UI updates.

---

## 📊 Long-Term Feature Matrix

| Feature Module | Level | Core Tech / Protocol | Target Release |
|---|---|---|---|
| **Node Status & Search** | Essential | LocalClient Go API | v0.1.0 |
| **SSH Suspension** | Essential | `tea.ExecProcess` + OS pty | v0.1.0 |
| **Exit Node Switcher** | Operator | `tailscale up --exit-node` | v0.4.0 |
| **DERP Latency Graph** | Diagnostic | Netcheck / UDP probes | v0.5.0 |
| **Taildrop File Manager** | Utility | Taildrop Local API | v0.5.0 |
| **Serve / Funnel Manager** | Developer | `tailscale.com/client/local` | v0.6.0 |
| **Key Expiry & Lock HUD** | Security | Admin/Local Key API | v0.7.0 |
| **Tmux & Background Daemon** | Workflow | Unix Sockets + Desktop Notify | v1.0.0 |
