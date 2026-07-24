package main

import (
	"flag"
	"fmt"
	"os"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/Wian47/ts-hud/internal/tsnet"
	"github.com/Wian47/ts-hud/internal/ui"
)

var version = "dev"

func main() {
	refreshRate := flag.Duration("refresh-rate", 5*time.Second, "background auto-refresh interval")
	showVersion := flag.Bool("version", false, "print version and exit")
	flag.Parse()

	if *showVersion {
		fmt.Println("ts-hud " + version)
		return
	}

	if *refreshRate <= 0 {
		fmt.Fprintln(os.Stderr, "ts-hud: --refresh-rate must be positive")
		os.Exit(1)
	}

	m := ui.NewModel(tsnet.NewFetcher(), *refreshRate)
	p := tea.NewProgram(m)
	if _, err := p.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "ts-hud:", err)
		os.Exit(1)
	}
}
