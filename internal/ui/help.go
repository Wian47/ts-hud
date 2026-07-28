package ui

import "strings"

// helpSection is one grouped block in the ? help overlay, mirroring one
// of the app's existing panels.
type helpSection struct {
	title string
	rows  []string
}

// helpSections is the single source of truth for the ? overlay's content.
// It intentionally duplicates the per-panel footer hints already rendered
// in Model.View — this is the one place the whole keymap is visible at
// once, so keys are spelled out here even though they also appear inline.
func helpSections() []helpSection {
	return []helpSection{
		{"Peer table", []string{
			"j/k        move down/up",
			"g/G        jump to top/bottom",
			"/          search",
			"enter      ssh into selected peer",
			"x          exit node picker",
			"d          DERP latency matrix",
			"i          peer detail",
			"p          preferences panel",
			"c          connection up/down",
			"a          account switch",
			"r          refresh",
			"q          quit",
		}},
		{"Search", []string{
			"esc        clear and exit search",
		}},
		{"Ssh session", []string{
			"ctrl+q     detach",
		}},
		{"Exit node picker", []string{
			"j/k        move down/up",
			"enter      select highlighted peer",
			"l          toggle allow LAN access",
			"esc/x      cancel",
		}},
		{"DERP latency matrix", []string{
			"r          re-run the check",
			"esc/d      back",
		}},
		{"Peer detail", []string{
			"r          re-run the probe",
			"esc/i      back",
		}},
		{"Preferences panel", []string{
			"j/k        move down/up",
			"enter      toggle highlighted preference",
			"esc/p      back",
		}},
		{"Connection-down confirm", []string{
			"y          confirm bringing the connection down",
			"n/esc      cancel",
		}},
		{"Account switch", []string{
			"j/k        move down/up",
			"enter      switch to highlighted account",
			"esc/a      back",
		}},
	}
}

// renderHelpPanel renders the ? overlay: a static, grouped list of every
// keybinding in the app. Unlike the other panels it never loads or
// errors, so there is no loading/error branch here.
func renderHelpPanel(width int) string {
	var b strings.Builder

	b.WriteString(headerStyle.Render("Help"))
	b.WriteString("\n")

	for i, section := range helpSections() {
		if i > 0 {
			b.WriteString("\n")
		}
		b.WriteString(rowStyle.Render("  " + section.title))
		b.WriteString("\n")
		for _, row := range section.rows {
			b.WriteString(helpStyle.Render("    " + row))
			b.WriteString("\n")
		}
	}

	b.WriteString("\n")
	b.WriteString(helpStyle.Render("  esc/? close"))

	return strings.TrimRight(b.String(), "\n")
}
