package ui

import (
	"strings"

	"tailscale.com/ipn"
)

// profileName returns a profile's display label: its login name, falling
// back to the tailnet's display name when Name is empty (matching how
// `tailscale switch` lists profiles without a resolved login name yet).
func profileName(p ipn.LoginProfile) string {
	if p.Name != "" {
		return p.Name
	}
	return p.NetworkProfile.DisplayNameOrDefault()
}

// renderAccountsPanel shows the account-switch overlay opened via 'a': a
// cursor-navigable list of already-authenticated profiles, styled like the
// exit-node picker and preferences panel. Mirrors renderPrefsPanel's
// structure: an empty/no-baseline state (no profiles loaded yet, or the
// initial fetch failed) replaces the list entirely; once a list is loaded,
// a later error (e.g. a failed switch) is appended below it instead of
// blanking what's already on screen.
func renderAccountsPanel(current ipn.LoginProfile, all []ipn.LoginProfile, cursor int, loading bool, accountsErr error, width int) string {
	var b strings.Builder

	if loading {
		b.WriteString(helpStyle.Render("  loading accounts…"))
		return b.String()
	}
	if len(all) == 0 {
		msg := "no accounts — run tailscale login"
		style := helpStyle
		if accountsErr != nil {
			msg = accountsErr.Error()
			style = errorStyle
		}
		b.WriteString(style.Render("  " + msg))
		return b.String()
	}

	b.WriteString(headerStyle.Render("Switch account"))
	b.WriteString("\n")

	for i, p := range all {
		label := profileName(p)
		if p.ID == current.ID {
			label += "  (current)"
		}
		row := "  " + label
		style := rowStyle
		if i == cursor {
			row = fitWidth(row, width)
			style = selectedRowStyle
		}
		b.WriteString(style.Render(row))
		b.WriteString("\n")
	}

	if accountsErr != nil {
		b.WriteString("\n")
		b.WriteString(errorStyle.Render("  " + accountsErr.Error()))
	}

	return strings.TrimRight(b.String(), "\n")
}
