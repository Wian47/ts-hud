package ui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

const (
	defaultTermWidth  = 80
	defaultTermHeight = 24

	// frameGutter is the blank column kept between the border and content
	// on each side.
	frameGutter = 1
)

var frameBorder = lipgloss.RoundedBorder()

// contentWidth returns how many terminal cells are available for content
// (header/table/footer text) inside a frame of the given terminal width,
// after accounting for the border and its gutter. Callers use this to size
// content so it exactly fills the frame, rather than leaving the frame to
// pad it with unstyled whitespace.
func contentWidth(termWidth int) int {
	if termWidth <= 0 {
		termWidth = defaultTermWidth
	}
	w := termWidth - 2 - 2*frameGutter
	if w < 1 {
		w = 1
	}
	return w
}

// renderFrame wraps header/body/footer in a rounded border sized to fill
// the terminal. body is padded with blank lines so footer stays pinned to
// the bottom of the frame regardless of how many rows body actually has.
func renderFrame(width, height int, header, body, footer string) string {
	if width <= 0 {
		width = defaultTermWidth
	}
	if height <= 0 {
		height = defaultTermHeight
	}

	inner := contentWidth(width)
	if width-2 < 1 {
		// Terminal too narrow to frame meaningfully; fall back to raw content.
		return strings.Join([]string{header, body, footer}, "\n")
	}

	headerLines := strings.Split(header, "\n")
	bodyLines := strings.Split(body, "\n")
	footerLines := strings.Split(footer, "\n")

	// 2 border rows + 2 divider rows, plus header/footer content rows, are
	// fixed; body grows to consume whatever's left so footer stays pinned
	// to the bottom of the frame.
	fixed := 4 + len(headerLines) + len(footerLines)
	if pad := height - fixed - len(bodyLines); pad > 0 {
		bodyLines = append(bodyLines, make([]string, pad)...)
	}

	var b strings.Builder
	writeEdge := func(left, fill, right string) {
		b.WriteString(left + strings.Repeat(fill, width-2) + right)
		b.WriteString("\n")
	}
	writeSection := func(lines []string) {
		for _, line := range lines {
			b.WriteString(frameBorder.Left)
			b.WriteString(strings.Repeat(" ", frameGutter))
			b.WriteString(fitWidth(line, inner))
			b.WriteString(strings.Repeat(" ", frameGutter))
			b.WriteString(frameBorder.Right)
			b.WriteString("\n")
		}
	}

	writeEdge(frameBorder.TopLeft, frameBorder.Top, frameBorder.TopRight)
	writeSection(headerLines)
	writeEdge(frameBorder.MiddleLeft, frameBorder.Top, frameBorder.MiddleRight)
	writeSection(bodyLines)
	writeEdge(frameBorder.MiddleLeft, frameBorder.Top, frameBorder.MiddleRight)
	writeSection(footerLines)
	b.WriteString(frameBorder.BottomLeft + strings.Repeat(frameBorder.Bottom, width-2) + frameBorder.BottomRight)

	return b.String()
}

// fitWidth pads or truncates s (which may contain ANSI styling) to exactly
// width terminal cells. Truncation is a hard single-line cut (via
// ansi.Truncate) rather than lipgloss's MaxWidth, which word-wraps
// overflowing text onto extra lines and would corrupt the frame.
func fitWidth(s string, width int) string {
	w := lipgloss.Width(s)
	switch {
	case w < width:
		return s + strings.Repeat(" ", width-w)
	case w > width:
		return ansi.Truncate(s, width, "…")
	default:
		return s
	}
}
