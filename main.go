package main

import (
	"flag"
	"fmt"
	"os"
	"runtime/debug"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/Wian47/ts-hud/internal/tsnet"
	"github.com/Wian47/ts-hud/internal/ui"
)

// version is overridden at build time via -ldflags -X main.version=... by
// goreleaser. For `go install .../ts-hud@version` builds, which don't set
// ldflags, it falls back to the module version Go embeds in the binary.
var version = "dev"

func resolveVersion() string {
	if version != "dev" {
		return version
	}
	if info, ok := debug.ReadBuildInfo(); ok && info.Main.Version != "" && info.Main.Version != "(devel)" {
		return info.Main.Version
	}
	return version
}

func main() {
	refreshRate := flag.Duration("refresh-rate", 5*time.Second, "background auto-refresh interval")
	showVersion := flag.Bool("version", false, "print version and exit")
	flag.Parse()

	if *showVersion {
		fmt.Println("ts-hud " + resolveVersion())
		return
	}

	if *refreshRate <= 0 {
		fmt.Fprintln(os.Stderr, "ts-hud: --refresh-rate must be positive")
		os.Exit(1)
	}

	m := ui.NewModel(tsnet.NewFetcher(), *refreshRate)
	p := tea.NewProgram(m, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "ts-hud:", err)
		os.Exit(1)
	}
}
