package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"tailscale.com/ipn/ipnstate"

	"github.com/Wian47/ts-hud/internal/tsnet"
)

// renderPeerDetail shows a live ping + whois probe result for one peer,
// opened via 'i' on the peer table.
func renderPeerDetail(target tsnet.Peer, detail tsnet.PeerDetail, loading bool) string {
	var b strings.Builder

	status := "offline"
	statusStyle := offlineStyle
	if target.Online {
		status = "online"
		statusStyle = onlineStyle
	}
	b.WriteString(headerStyle.Render(target.DisplayName()))
	b.WriteString("  " + target.OS + "  ")
	b.WriteString(statusStyle.Render(status))
	b.WriteString("\n\n")

	if loading {
		b.WriteString(helpStyle.Render("  probing…"))
		return b.String()
	}

	if detail.WhoIsErr != nil {
		b.WriteString("Owner: " + errorStyle.Render(detail.WhoIsErr.Error()))
	} else {
		owner := detail.Owner
		if owner == "" {
			owner = "unknown"
		}
		tags := "none"
		if len(detail.Tags) > 0 {
			tags = strings.Join(detail.Tags, ", ")
		}
		b.WriteString("Owner: " + owner + "\n")
		b.WriteString("Tags:  " + tags)
	}
	b.WriteString("\n\n")

	switch {
	case detail.PingErr != nil:
		b.WriteString("Ping:  " + errorStyle.Render(detail.PingErr.Error()))
	case detail.Ping != nil:
		text, style := formatPingResult(detail.Ping)
		b.WriteString("Ping:  " + style.Render(text))
	default:
		b.WriteString("Ping:  " + helpStyle.Render("no result"))
	}

	return strings.TrimRight(b.String(), "\n")
}

// formatPingResult renders one PingResult as a one-line summary and the
// style to render it in: onlineStyle for a direct path, offlineStyle for
// a relayed one, errorStyle if the probe ran but couldn't reach the peer.
func formatPingResult(pr *ipnstate.PingResult) (string, lipgloss.Style) {
	if pr.Err != "" {
		return pr.Err, errorStyle
	}
	latency := fmt.Sprintf("%.1fms", pr.LatencySeconds*1000)
	switch {
	case pr.Endpoint != "":
		return fmt.Sprintf("%s via %s (direct)", latency, pr.Endpoint), onlineStyle
	case pr.DERPRegionCode != "":
		return fmt.Sprintf("%s via DERP %s", latency, pr.DERPRegionCode), offlineStyle
	case pr.PeerRelay != "":
		return fmt.Sprintf("%s via peer relay %s", latency, pr.PeerRelay), offlineStyle
	default:
		return latency, onlineStyle
	}
}
