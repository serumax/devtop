// devtop is a k9s/htop-style TUI: a recursive inventory of the projects under
// a root directory, with live process status and start/stop control.
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	tea "github.com/charmbracelet/bubbletea"

	"devtop/internal/procs"
	"devtop/internal/scan"
	"devtop/internal/ui"
)

func main() {
	list := flag.Bool("list", false, "print the inventory once and exit (no TUI)")
	flag.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: devtop [-list] [root]\n\nroot defaults to the current directory.")
	}
	flag.Parse()

	root := "."
	if flag.NArg() > 0 {
		root = flag.Arg(0)
	}
	root, err := filepath.Abs(root)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	if *list {
		printList(root)
		return
	}
	if _, err := tea.NewProgram(ui.New(root), tea.WithAltScreen()).Run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func printList(root string) {
	ps, err := scan.Scan(root)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
	}
	paths := make([]string, len(ps))
	for i, p := range ps {
		paths[i] = p.Path
	}
	info := procs.New().Snapshot(paths)
	for _, p := range ps {
		state, port := "off", "-"
		in := info[p.Path]
		if in.Running() {
			state = "on"
		}
		if len(in.Ports) > 0 {
			port = fmt.Sprint(in.Ports[0])
		}
		typ := p.Kind
		if p.Framework != "" {
			typ = p.Framework
		}
		fmt.Printf("%-3s %-45s %-10s %-16s %s\n", state, p.Rel, typ, p.Branch, port)
	}
}
