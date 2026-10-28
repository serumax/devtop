// Package ui is the Bubble Tea front-end: an inventory table with live status.
package ui

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"devtop/internal/procs"
	"devtop/internal/run"
	"devtop/internal/scan"
)

type viewMode int

const (
	viewTable viewMode = iota
	viewLogs
)

var filterNames = []string{"all", "runnable", "running"}

var (
	headerStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("15")).Background(lipgloss.Color("24"))
	selStyle    = lipgloss.NewStyle().Bold(true).Background(lipgloss.Color("238"))
	onStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("42"))
	dimStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("243"))
	warnStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("214"))
	helpStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
)

type (
	scanDone struct {
		projects []scan.Project
		err      error
	}
	snapDone   map[string]procs.Info
	tickMsg    time.Time
	actionDone string
)

type pending struct {
	prompt  string
	project scan.Project
	restart bool
}

type Model struct {
	root     string
	projects []scan.Project
	info     map[string]procs.Info
	mon      *procs.Monitor

	cursor, offset int
	filter         string
	filtering      bool
	mode           int
	showCompose    bool
	width, height  int
	view           viewMode
	logLines       []string
	confirm        *pending
	msg            string
	scanning       bool
	snapBusy       bool
}

func New(root string) Model {
	return Model{root: root, mon: procs.New(), info: map[string]procs.Info{}, scanning: true}
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(m.scanCmd(), tick())
}

func tick() tea.Cmd {
	return tea.Tick(2*time.Second, func(t time.Time) tea.Msg { return tickMsg(t) })
}

func (m Model) scanCmd() tea.Cmd {
	root := m.root
	return func() tea.Msg {
		ps, err := scan.Scan(root)
		return scanDone{ps, err}
	}
}

func (m *Model) snapCmd() tea.Cmd {
	if m.snapBusy || len(m.projects) == 0 {
		return nil
	}
	m.snapBusy = true
	paths := make([]string, len(m.projects))
	for i, p := range m.projects {
		paths[i] = p.Path
	}
	mon := m.mon
	return func() tea.Msg { return snapDone(mon.Snapshot(paths)) }
}

func (m Model) visible() []scan.Project {
	var out []scan.Project
	needle := strings.ToLower(m.filter)
	for _, p := range m.projects {
		if p.Kind == "compose" && !m.showCompose {
			continue
		}
		running := m.info[p.Path].Running()
		switch m.mode {
		case 1:
			if !p.Runnable() {
				continue
			}
		case 2:
			if !running {
				continue
			}
		}
		if needle != "" {
			hay := strings.ToLower(p.Rel + " " + p.Framework + " " + p.Branch + " " + p.Kind)
			if !strings.Contains(hay, needle) {
				continue
			}
		}
		out = append(out, p)
	}
	return out
}

func (m Model) selected() (scan.Project, bool) {
	v := m.visible()
	if m.cursor < 0 || m.cursor >= len(v) {
		return scan.Project{}, false
	}
	return v[m.cursor], true
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
	case scanDone:
		m.scanning = false
		m.projects = msg.projects
		if msg.err != nil {
			m.msg = "scan: " + msg.err.Error()
		}
		return m, m.snapCmd()
	case snapDone:
		m.snapBusy = false
		m.info = map[string]procs.Info(msg)
	case tickMsg:
		if m.view == viewLogs {
			if p, ok := m.selected(); ok {
				m.logLines = run.Tail(p, 200)
			}
		}
		return m, tea.Batch(tick(), m.snapCmd())
	case actionDone:
		m.msg = string(msg)
		return m, m.snapCmd()
	case tea.KeyMsg:
		return m.key(msg)
	}
	return m, nil
}

func (m Model) key(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	s := k.String()
	if m.confirm != nil {
		c := m.confirm
		m.confirm = nil
		if s == "y" {
			return m, m.stopCmd(c.project, c.restart)
		}
		m.msg = "cancelled"
		return m, nil
	}
	if m.filtering {
		switch s {
		case "enter", "esc":
			m.filtering = false
		case "backspace":
			if r := []rune(m.filter); len(r) > 0 {
				m.filter = string(r[:len(r)-1])
			}
		default:
			if len(k.Runes) > 0 {
				m.filter += string(k.Runes)
			}
		}
		m.cursor, m.offset = 0, 0
		return m, nil
	}
	if m.view == viewLogs {
		if s == "esc" || s == "q" || s == "l" {
			m.view = viewTable
		}
		return m, nil
	}

	n := len(m.visible())
	switch s {
	case "q", "ctrl+c":
		return m, tea.Quit
	case "j", "down":
		m.cursor = min(m.cursor+1, max(n-1, 0))
	case "k", "up":
		m.cursor = max(m.cursor-1, 0)
	case "g", "home":
		m.cursor = 0
	case "G", "end":
		m.cursor = max(n-1, 0)
	case "/":
		m.filtering = true
	case "esc":
		m.filter = ""
	case "f":
		m.mode = (m.mode + 1) % len(filterNames)
		m.cursor, m.offset = 0, 0
	case "d":
		m.showCompose = !m.showCompose
		m.cursor, m.offset = 0, 0
	case "R":
		m.scanning = true
		return m, m.scanCmd()
	case "l":
		if p, ok := m.selected(); ok {
			m.logLines = run.Tail(p, 200)
			m.view = viewLogs
		}
	case "o":
		if p, ok := m.selected(); ok {
			if ports := m.info[p.Path].Ports; len(ports) > 0 {
				_ = exec.Command("xdg-open", fmt.Sprintf("http://localhost:%d", ports[0])).Start()
				m.msg = fmt.Sprintf("opening localhost:%d", ports[0])
			} else {
				m.msg = "no listening port"
			}
		}
	case "enter", " ":
		return m.toggle(false)
	case "r":
		return m.toggle(true)
	}
	return m, nil
}

func (m Model) toggle(restart bool) (tea.Model, tea.Cmd) {
	p, ok := m.selected()
	if !ok {
		return m, nil
	}
	if !p.Runnable() {
		m.msg = p.Rel + ": no dev/start script, cannot start"
		return m, nil
	}
	if !m.info[p.Path].Running() {
		return m, startCmd(p)
	}
	if !run.Owned(p) {
		m.confirm = &pending{
			prompt:  fmt.Sprintf("%s was not started by devtop. Stop it anyway? [y/N]", p.Rel),
			project: p, restart: restart,
		}
		return m, nil
	}
	return m, m.stopCmd(p, restart)
}

func startCmd(p scan.Project) tea.Cmd {
	return func() tea.Msg {
		if err := run.Start(p); err != nil {
			return actionDone("start " + p.Rel + ": " + err.Error())
		}
		return actionDone("started " + p.Rel)
	}
}

func (m Model) stopCmd(p scan.Project, restart bool) tea.Cmd {
	pids := m.info[p.Path].PIDs
	return func() tea.Msg {
		if err := run.Stop(pids, 5*time.Second); err != nil {
			return actionDone("stop " + p.Rel + ": " + err.Error())
		}
		if restart {
			if err := run.Start(p); err != nil {
				return actionDone("restart " + p.Rel + ": " + err.Error())
			}
			return actionDone("restarted " + p.Rel)
		}
		return actionDone("stopped " + p.Rel)
	}
}

func trunc(s string, w int) string {
	r := []rune(s)
	if w <= 0 {
		return ""
	}
	if len(r) <= w {
		return s + strings.Repeat(" ", w-len(r))
	}
	return string(r[:w-1]) + "…"
}

// truncLeft keeps the tail of s: for paths the last segments identify the project.
func truncLeft(s string, w int) string {
	r := []rune(s)
	if w <= 0 {
		return ""
	}
	if len(r) <= w {
		return s + strings.Repeat(" ", w-len(r))
	}
	return "…" + string(r[len(r)-w+1:])
}

func fmtMem(b uint64) string {
	if b == 0 {
		return "-"
	}
	return fmt.Sprintf("%dM", b/1024/1024)
}

func (m Model) View() string {
	if m.width == 0 {
		return "loading…"
	}
	if m.view == viewLogs {
		return m.logView()
	}
	v := m.visible()
	running, hidden := 0, 0
	for _, p := range m.projects {
		if m.info[p.Path].Running() {
			running++
		}
		if p.Kind == "compose" && !m.showCompose {
			hidden++
		}
	}
	docker := "docker: shown"
	if !m.showCompose {
		docker = fmt.Sprintf("docker: %d hidden", hidden)
	}
	var b strings.Builder
	status := ""
	if m.scanning {
		status = " (scanning…)"
	}
	b.WriteString(trunc(fmt.Sprintf("devtop %s  %d projects, %d running, view: %s, %s%s",
		filepath.Base(m.root), len(m.projects)-hidden, running, filterNames[m.mode], docker, status), m.width) + "\n")

	const wState, wFw, wBranch, wPort, wCPU, wMem = 2, 10, 18, 6, 5, 5
	showBranch := m.width >= 100
	fixed := wState + wFw + wPort + wCPU + wMem + 4
	if showBranch {
		fixed += wBranch + 1
	}
	wName := max(m.width-fixed, 12)
	head := trunc("", wState) + trunc("PROJECT", wName+1) + trunc("TYPE", wFw+1)
	if showBranch {
		head += trunc("BRANCH", wBranch+1)
	}
	head += trunc("PORT", wPort+1) + trunc("CPU", wCPU+1) + trunc("MEM", wMem)
	b.WriteString(headerStyle.Render(head))
	b.WriteString("\n")

	rows := max(m.height-5, 3)
	if m.cursor < m.offset {
		m.offset = m.cursor
	}
	if m.cursor >= m.offset+rows {
		m.offset = m.cursor - rows + 1
	}
	for i := m.offset; i < min(len(v), m.offset+rows); i++ {
		p := v[i]
		in := m.info[p.Path]
		dot, port, cpu := dimStyle.Render("○ "), "-", "-"
		if in.Running() {
			dot = onStyle.Render("● ")
			cpu = fmt.Sprintf("%.0f%%", in.CPU)
		}
		if len(in.Ports) > 0 {
			port = fmt.Sprint(in.Ports[0])
		}
		typ := p.Kind
		if p.Framework != "" {
			typ = p.Framework
		}
		branch := p.Branch
		if p.Worktree {
			branch += " ⎇"
		}
		line := truncLeft(p.Rel, wName) + " " + trunc(typ, wFw) + " "
		if showBranch {
			line += trunc(branch, wBranch) + " "
		}
		line += trunc(port, wPort) + " " + trunc(cpu, wCPU) + " " + trunc(fmtMem(in.RSS), wMem)
		if i == m.cursor {
			b.WriteString(dot + selStyle.Render(line))
		} else if !p.Runnable() {
			b.WriteString(dot + dimStyle.Render(line))
		} else {
			b.WriteString(dot + line)
		}
		b.WriteString("\n")
	}
	for i := len(v) - m.offset; i < rows; i++ {
		b.WriteString("\n")
	}
	switch {
	case m.confirm != nil:
		b.WriteString(warnStyle.Render(m.confirm.prompt))
	case m.filtering:
		b.WriteString("/" + m.filter + "█")
	default:
		line := "enter start/stop  r restart  l logs  o open  / filter  f view  d docker  R rescan  q quit"
		if m.width < 85 {
			line = "⏎ on/off  r restart  l logs  o open  / filter  f view  d docker  q quit"
		}
		if m.filter != "" {
			line = "filter: " + m.filter + " (esc)  " + line
		}
		b.WriteString(helpStyle.Render(trunc(line, m.width)))
	}
	if m.msg != "" && m.confirm == nil && !m.filtering {
		b.WriteString("\n" + warnStyle.Render(trunc(m.msg, m.width)))
	}
	return b.String()
}

func (m Model) logView() string {
	p, _ := m.selected()
	rows := max(m.height-3, 3)
	lines := m.logLines
	if len(lines) > rows {
		lines = lines[len(lines)-rows:]
	}
	var b strings.Builder
	b.WriteString(headerStyle.Render(trunc("logs: "+p.Rel+"  ("+run.LogPath(p)+")", m.width)) + "\n")
	for _, l := range lines {
		b.WriteString(trunc(l, m.width) + "\n")
	}
	b.WriteString(helpStyle.Render("esc/q/l back"))
	return b.String()
}
