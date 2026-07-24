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

func renderTable(peers []tsnet.Peer, cursor int) string {
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
			padRight(p.HostName, colHostWidth),
			padRight(ip, colIPWidth),
			padRight(p.OS, colOSWidth),
			status,
			connCell(p),
		)

		style := rowStyle
		if i == cursor {
			style = selectedRowStyle
		}
		b.WriteString(style.Render(row))
		b.WriteString("\n")
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
