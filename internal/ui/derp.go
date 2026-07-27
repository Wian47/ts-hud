package ui

import (
	"fmt"
	"strings"
	"time"

	"github.com/Wian47/ts-hud/internal/tsnet"
)

const (
	colDERPCodeWidth    = 8
	colDERPNameWidth    = 24
	colDERPLatencyWidth = 12
)

// renderDERPTable shows live per-region DERP latency. Unlike renderTable,
// it's read-only: nothing here is selectable, so there's no cursor.
func renderDERPTable(result tsnet.NetCheckResult, loading bool, checkErr error, width int) string {
	var b strings.Builder

	switch {
	case loading:
		b.WriteString(helpStyle.Render("  checking DERP latency…"))
		return b.String()
	case checkErr != nil:
		b.WriteString(errorStyle.Render("  " + checkErr.Error()))
		return b.String()
	case len(result.Regions) == 0:
		b.WriteString(helpStyle.Render("  no DERP regions available"))
		return b.String()
	}

	b.WriteString(connectivitySummary(result))
	b.WriteString("\n\n")

	b.WriteString(headerStyle.Render(formatDERPRow("CODE", "REGION", "LATENCY")))
	b.WriteString("\n")

	for _, r := range result.Regions {
		latency := "—"
		latencyStyle := offlineStyle
		if r.Available {
			latency = r.Latency.Round(time.Millisecond / 10).String()
			latencyStyle = onlineStyle
		}

		name := r.Name
		if r.Preferred {
			name += "  [preferred]"
		}

		row := fmt.Sprintf(
			"%s %s %s",
			padRight(r.Code, colDERPCodeWidth),
			padRight(name, colDERPNameWidth),
			latencyStyle.Render(padRight(latency, colDERPLatencyWidth)),
		)

		style := rowStyle
		if r.Preferred {
			row = fitWidth(row, width)
			style = selectedRowStyle
		}
		b.WriteString(style.Render(row))
		b.WriteString("\n")
	}

	return strings.TrimRight(b.String(), "\n")
}

// connectivitySummary renders a one-line UDP/NAT verdict: whether direct
// UDP peer connections are likely to work, or whether hole-punching is
// unreliable and connections will fall back to relayed DERP (TCP).
func connectivitySummary(result tsnet.NetCheckResult) string {
	switch {
	case !result.UDP:
		return errorStyle.Render("Connectivity: UDP unavailable — DERP (TCP) relay only")
	case result.NATKnown && result.HardNAT:
		return offlineStyle.Render("Connectivity: UDP ok · NAT hard — expect relayed (DERP/TCP) connections")
	case result.NATKnown && !result.HardNAT:
		return onlineStyle.Render("Connectivity: UDP ok · NAT easy — direct connections likely")
	default:
		return onlineStyle.Render("Connectivity: UDP ok · NAT unknown")
	}
}

func formatDERPRow(code, region, latency string) string {
	return fmt.Sprintf(
		"%s %s %s",
		padRight(code, colDERPCodeWidth),
		padRight(region, colDERPNameWidth),
		padRight(latency, colDERPLatencyWidth),
	)
}
