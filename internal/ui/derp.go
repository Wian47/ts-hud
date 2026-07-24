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
func renderDERPTable(regions []tsnet.DERPRegion, loading bool, checkErr error, width int) string {
	var b strings.Builder

	b.WriteString(headerStyle.Render(formatDERPRow("CODE", "REGION", "LATENCY")))
	b.WriteString("\n")

	switch {
	case loading:
		b.WriteString(helpStyle.Render("  checking DERP latency…"))
		return b.String()
	case checkErr != nil:
		b.WriteString(errorStyle.Render("  " + checkErr.Error()))
		return b.String()
	case len(regions) == 0:
		b.WriteString(helpStyle.Render("  no DERP regions available"))
		return b.String()
	}

	for _, r := range regions {
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
			// Same trick as the peer table's selected row: fit to an exact
			// width *before* styling, since lipgloss's Style.Width() would
			// word-wrap rather than pad a row that's shorter than width.
			row = fitWidth(row, width)
			style = selectedRowStyle
		}
		b.WriteString(style.Render(row))
		b.WriteString("\n")
	}

	return strings.TrimRight(b.String(), "\n")
}

func formatDERPRow(code, region, latency string) string {
	return fmt.Sprintf(
		"%s %s %s",
		padRight(code, colDERPCodeWidth),
		padRight(region, colDERPNameWidth),
		padRight(latency, colDERPLatencyWidth),
	)
}
