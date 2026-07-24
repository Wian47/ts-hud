package main

import (
	"fmt"
	"os"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/Wian47/ts-hud/internal/tsnet"
	"github.com/Wian47/ts-hud/internal/ui"
)

func main() {
	m := ui.NewModel(tsnet.NewFetcher(), 5*time.Second)
	p := tea.NewProgram(m)
	if _, err := p.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "ts-hud:", err)
		os.Exit(1)
	}
}
