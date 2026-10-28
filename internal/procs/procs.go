// Package procs maps running processes onto projects by working directory.
package procs

import (
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/shirou/gopsutil/v4/net"
	"github.com/shirou/gopsutil/v4/process"
)

// Info aggregates the processes attributed to one project.
type Info struct {
	PIDs  []int32
	Ports []uint32
	CPU   float64
	RSS   uint64
}

// Running reports whether any process is attributed to the project.
func (i Info) Running() bool { return len(i.PIDs) > 0 }

type entry struct {
	p      *process.Process
	cwd    string
	runner bool // runtime binary whose command line looks like a dev server
}

// Monitor keeps per-process state between snapshots so CPU deltas work.
type Monitor struct {
	mu    sync.Mutex
	cache map[int32]*entry
}

func New() *Monitor { return &Monitor{cache: map[int32]*entry{}} }

var runtimes = map[string]bool{
	"node": true, "bun": true, "deno": true,
	"npm": true, "pnpm": true, "yarn": true, "npx": true,
}

// devTokens must equal an argument (or its base name) exactly; substring
// matching would also catch paths and tools such as "codegraph serve".
var devTokens = map[string]bool{
	"dev": true, "start": true, "preview": true,
	"astro": true, "nuxt": true, "nuxi": true, "vite": true, "next": true, "keystatic": true,
}

func classify(p *process.Process) *entry {
	e := &entry{p: p}
	e.cwd, _ = p.Cwd()
	name, _ := p.Name()
	if !runtimes[name] {
		return e
	}
	args, _ := p.CmdlineSlice()
	for _, a := range args[min(1, len(args)):] {
		if devTokens[a] || devTokens[filepath.Base(a)] {
			e.runner = true
			return e
		}
	}
	return e
}

// Snapshot attributes live processes to the deepest project containing their
// cwd. A process counts when it listens on a port, or is a JS runtime whose
// command line looks like a dev server.
func (m *Monitor) Snapshot(projects []string) map[string]Info {
	m.mu.Lock()
	defer m.mu.Unlock()

	pids, _ := process.Pids()
	live := make(map[int32]bool, len(pids))
	for _, pid := range pids {
		live[pid] = true
		if _, ok := m.cache[pid]; ok {
			continue
		}
		if p, err := process.NewProcess(pid); err == nil {
			m.cache[pid] = classify(p)
		}
	}
	for pid := range m.cache {
		if !live[pid] {
			delete(m.cache, pid)
		}
	}

	listening := map[int32][]uint32{}
	if conns, err := net.Connections("tcp"); err == nil {
		for _, c := range conns {
			if c.Status == "LISTEN" && c.Pid > 0 {
				listening[c.Pid] = append(listening[c.Pid], c.Laddr.Port)
			}
		}
	}

	sorted := append([]string(nil), projects...)
	sort.Slice(sorted, func(i, j int) bool { return len(sorted[i]) > len(sorted[j]) })

	out := map[string]Info{}
	for pid, e := range m.cache {
		if e.cwd == "" {
			continue
		}
		ports := listening[pid]
		if len(ports) == 0 && !e.runner {
			continue
		}
		proj := deepest(sorted, e.cwd)
		if proj == "" {
			continue
		}
		info := out[proj]
		info.PIDs = append(info.PIDs, pid)
		info.Ports = append(info.Ports, ports...)
		if cpu, err := e.p.Percent(0); err == nil {
			info.CPU += cpu
		}
		if mem, err := e.p.MemoryInfo(); err == nil && mem != nil {
			info.RSS += mem.RSS
		}
		out[proj] = info
	}
	for k, info := range out {
		sort.Slice(info.PIDs, func(i, j int) bool { return info.PIDs[i] < info.PIDs[j] })
		sort.Slice(info.Ports, func(i, j int) bool { return info.Ports[i] < info.Ports[j] })
		out[k] = info
	}
	return out
}

// deepest returns the longest project path containing cwd (sorted longest first).
func deepest(sorted []string, cwd string) string {
	for _, p := range sorted {
		if cwd == p || strings.HasPrefix(cwd, p+"/") {
			return p
		}
	}
	return ""
}
