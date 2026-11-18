// Command dkrv is a terminal UI for Docker that groups containers by compose
// project.
package main

import (
	"flag"
	"fmt"
	"os"
	"runtime/debug"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/dom-schulz/docker-view/internal/compose"
	"github.com/dom-schulz/docker-view/internal/config"
	"github.com/dom-schulz/docker-view/internal/docker"
	"github.com/dom-schulz/docker-view/internal/ui"
)

// version is set at build time with -ldflags "-X main.version=...".
var version = ""

func main() {
	showVersion := flag.Bool("version", false, "print the version and exit")
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "dkrv %s: Docker compose projects in a terminal UI\n\nUsage: dkrv [--version]\n\nConfig: %s\n", getVersion(), config.Path())
	}
	flag.Parse()
	if *showVersion {
		fmt.Println("dkrv", getVersion())
		return
	}

	cli, err := docker.New()
	if err != nil {
		fmt.Fprintln(os.Stderr, "dkrv:", err)
		os.Exit(1)
	}
	defer cli.Close()

	path := config.Path()
	cfg, cfgErr := config.Load(path)

	m := ui.New(ui.Options{
		Client:     cli,
		Config:     cfg,
		ConfigPath: path,
		ConfigErr:  cfgErr,
		Compose:    compose.Detect(),
		Version:    getVersion(),
	})
	// Terminals such as Ghostty turn the mouse wheel into arrow keys in
	// full-screen apps (alternate scroll mode, 1007), which would move the
	// selection. Turn that off while dkrv runs; text selection still works.
	fmt.Print(ui.WheelOff)
	_, err = tea.NewProgram(m, tea.WithAltScreen()).Run()
	fmt.Print(ui.WheelOn)
	if err != nil {
		fmt.Fprintln(os.Stderr, "dkrv:", err)
		os.Exit(1)
	}
}

func getVersion() string {
	if version != "" {
		return version
	}
	if bi, ok := debug.ReadBuildInfo(); ok && bi.Main.Version != "" && bi.Main.Version != "(devel)" {
		return bi.Main.Version
	}
	return "dev"
}
