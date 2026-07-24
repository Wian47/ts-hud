package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/Wian47/ts-hud/internal/tsnet"
)

const (
	colHostWidth   = 22
	colIPWidth     = 16
	colOSWidth     = 10
	colStatusWidth = 8
	colConnWidth   = 11
)

func renderTable(peers []tsnet.Peer, cursor, width int) string {
	var b strings.Builder

	b.WriteString(headerStyle.Render(formatRow("HOSTNAME", "IP", "OS", "STATUS", "CONN")))
	b.WriteString("\n")

	if len(peers) == 0 {
		b.WriteString(helpStyle.Render("  no peers match"))
		return b.String()
	}

	for i, p := range peers {
		ip := ""
		if len(p.IPs) > 0 {
			ip = p.IPs[0].String()
		}

		status := offlineStyle.Render(padRight("offline", colStatusWidth))
		if p.Online {
			status = onlineStyle.Render(padRight("online", colStatusWidth))
		}

		row := fmt.Sprintf(
			"%s %s %s %s %s",
			padRight(p.DisplayName(), colHostWidth),
			padRight(ip, colIPWidth),
			padRight(p.OS, colOSWidth),
			status,
			connCell(p),
		)

		style := rowStyle
		if i == cursor {
			// Fit the row to an exact width *before* styling it: lipgloss's
			// Style.Width() pads short content but word-wraps content that's
			// already too wide, which would break a single table row across
			// multiple lines. fitWidth does a hard single-line truncate
			// instead, so the highlight background still fills the row
			// (via the padding case) without ever wrapping it.
			row = fitWidth(row, width)
			style = selectedRowStyle
		}
		b.WriteString(style.Render(row))
		b.WriteString("\n")
	}

	return strings.TrimRight(b.String(), "\n")
}

// renderExitNodePicker lists exit-node-eligible peers plus a leading
// "none" entry that clears the exit node selection. cursor 0 addresses
// the "none" entry; cursor i (i>0) addresses candidates[i-1].
func renderExitNodePicker(candidates []tsnet.Peer, cursor int, allowLAN bool, width int) string {
	var b strings.Builder

	lanState := "off"
	if allowLAN {
		lanState = "on"
	}
	b.WriteString(headerStyle.Render(fmt.Sprintf("Select exit node  (allow LAN access: %s)", lanState)))
	b.WriteString("\n")

	rows := make([]string, 0, len(candidates)+1)
	rows = append(rows, "(none — disable exit node)")
	for _, p := range candidates {
		label := p.DisplayName()
		if p.IsExitNode {
			label += "  [active]"
		}
		rows = append(rows, label)
	}

	for i, row := range rows {
		row = "  " + row
		style := rowStyle
		if i == cursor {
			row = fitWidth(row, width)
			style = selectedRowStyle
		}
		b.WriteString(style.Render(row))
		b.WriteString("\n")
	}

	if len(candidates) == 0 {
		b.WriteString(helpStyle.Render("  no peers are advertising as exit nodes"))
	}

	return strings.TrimRight(b.String(), "\n")
}

func formatRow(hostname, ip, os, status, conn string) string {
	return fmt.Sprintf(
		"%s %s %s %s %s",
		padRight(hostname, colHostWidth),
		padRight(ip, colIPWidth),
		padRight(os, colOSWidth),
		padRight(status, colStatusWidth),
		padRight(conn, colConnWidth),
	)
}

func connCell(p tsnet.Peer) string {
	label := p.ConnType.String()
	if p.ConnType == tsnet.ConnDERP && p.DERPRegion != "" {
		label = fmt.Sprintf("DERP(%s)", p.DERPRegion)
	}
	label = padRight(label, colConnWidth)

	switch p.ConnType {
	case tsnet.ConnDirect:
		return connDirectStyle.Render(label)
	case tsnet.ConnDERP:
		return connDERPStyle.Render(label)
	case tsnet.ConnPeerRelay:
		return connPeerRelayStyle.Render(label)
	default:
		return connUnknownStyle.Render(label)
	}
}

func padRight(s string, width int) string {
	if lipgloss.Width(s) >= width {
		return s
	}
	return s + strings.Repeat(" ", width-lipgloss.Width(s))
}
