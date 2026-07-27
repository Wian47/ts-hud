package ui

import (
	"strings"

	"tailscale.com/ipn"
)

// numPrefRows is the fixed number of rows the preferences panel shows.
const numPrefRows = 5

type prefRow struct {
	label string
	on    bool
}

// prefRows defines the fixed order of the preferences panel's rows. Both
// renderPrefsPanel and Model.updatePrefsView (which maps prefsCursor to
// the Fetcher setter to call) rely on this exact order: SSH server,
// Shields up, Accept routes, Accept DNS, Advertise as exit node.
func prefRows(prefs *ipn.Prefs) []prefRow {
	return []prefRow{
		{"SSH server", prefs.RunSSH},
		{"Shields up", prefs.ShieldsUp},
		{"Accept routes", prefs.RouteAll},
		{"Accept DNS", prefs.CorpDNS},
		{"Advertise as exit node", prefs.AdvertisesExitNode()},
	}
}

func toggleState(on bool) string {
	if on {
		return onlineStyle.Render("on")
	}
	return offlineStyle.Render("off")
}

// renderPrefsPanel shows the preferences panel opened via 'p' on the peer
// table: a cursor-navigable list of toggles, styled like the exit-node
// picker. loading unconditionally blanks to a loading message (matching
// the DERP/peer-detail views' precedent) even if prefs from a previous
// fetch are available — a fresh EditPrefs/GetPrefs call is always in
// flight while loading is true, and Model never sets it without also
// firing that call.
func renderPrefsPanel(prefs *ipn.Prefs, cursor int, loading bool, prefsErr error, width int) string {
	var b strings.Builder

	if loading {
		b.WriteString(helpStyle.Render("  loading preferences…"))
		return b.String()
	}
	if prefs == nil {
		msg := "no preferences loaded"
		if prefsErr != nil {
			msg = prefsErr.Error()
		}
		b.WriteString(errorStyle.Render("  " + msg))
		return b.String()
	}

	b.WriteString(headerStyle.Render("Preferences"))
	b.WriteString("\n")

	for i, row := range prefRows(prefs) {
		text := "  " + row.label + "  " + toggleState(row.on)
		style := rowStyle
		if i == cursor {
			text = fitWidth(text, width)
			style = selectedRowStyle
		}
		b.WriteString(style.Render(text))
		b.WriteString("\n")
	}

	if prefsErr != nil {
		b.WriteString("\n")
		b.WriteString(errorStyle.Render("  " + prefsErr.Error()))
	}

	return strings.TrimRight(b.String(), "\n")
}
