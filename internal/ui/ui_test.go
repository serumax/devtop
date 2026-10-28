package ui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"devtop/internal/procs"
	"devtop/internal/scan"
)

func loaded() Model {
	var m tea.Model = New("/w")
	m, _ = m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	m, _ = m.Update(scanDone{projects: []scan.Project{
		{Path: "/w/a", Rel: "a", Kind: "node", Framework: "astro", Manager: "npm", Script: "dev", Branch: "main"},
		{Path: "/w/b", Rel: "b", Kind: "go"},
		{Path: "/w/d", Rel: "d", Kind: "compose"},
		{Path: "/w/c", Rel: "c", Kind: "node", Framework: "nuxt", Manager: "npm", Script: "dev"},
	}})
	m, _ = m.Update(snapDone{"/w/c": {PIDs: []int32{1}, Ports: []uint32{3000}}})
	return m.(Model)
}

func TestViewShowsInventoryAndStatus(t *testing.T) {
	out := loaded().View()
	for _, want := range []string{"3 projects, 1 running", "docker: 1 hidden", "astro", "nuxt", "3000", "main"} {
		if !strings.Contains(out, want) {
			t.Errorf("view missing %q:\n%s", want, out)
		}
	}
}

func TestFilterAndViewModes(t *testing.T) {
	m := loaded()
	if n := len(m.visible()); n != 3 {
		t.Errorf("compose projects must be hidden by default: want 3, got %d", n)
	}
	m.mode = 1 // runnable
	if n := len(m.visible()); n != 2 {
		t.Errorf("runnable view: want 2, got %d", n)
	}
	m.mode = 2 // running
	if v := m.visible(); len(v) != 1 || v[0].Rel != "c" {
		t.Errorf("running view: %+v", v)
	}
	m.mode, m.filter = 0, "ASTRO"
	if v := m.visible(); len(v) != 1 || v[0].Rel != "a" {
		t.Errorf("filter is case-insensitive over framework: %+v", v)
	}
}

func TestToggleGuards(t *testing.T) {
	m := loaded()
	m.cursor = 1 // "b": go project, not runnable
	next, cmd := m.toggle(false)
	if cmd != nil || !strings.Contains(next.(Model).msg, "cannot start") {
		t.Errorf("non-runnable project must not start: %+v", next.(Model).msg)
	}
	m.cursor = 2 // "c": running but not started by devtop -> needs confirmation
	next, cmd = m.toggle(false)
	if cmd != nil || next.(Model).confirm == nil {
		t.Error("stopping a foreign process must ask for confirmation")
	}
}

func TestDockerToggle(t *testing.T) {
	var m tea.Model = loaded()
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	mm := m.(Model)
	if n := len(mm.visible()); n != 4 {
		t.Errorf("after d: want 4 visible, got %d", n)
	}
	if !strings.Contains(mm.View(), "docker: shown") {
		t.Error("header should say docker: shown")
	}
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	if n := len(m.(Model).visible()); n != 3 {
		t.Errorf("second d hides again: want 3, got %d", n)
	}
}

var _ = procs.Info{}
