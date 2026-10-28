// Package scan builds a recursive inventory of the projects under a root.
package scan

import (
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Project is one directory recognised as a project by its marker files.
type Project struct {
	Path      string
	Rel       string
	Kind      string // node, compose, go, php, git
	Framework string
	Manager   string // npm, pnpm, yarn, bun (node only)
	Script    string // script launched to start it; empty when not runnable
	Branch    string
	Worktree  bool
}

// Runnable reports whether devtop knows how to start the project.
func (p Project) Runnable() bool { return p.Kind == "node" && p.Script != "" }

// Command is the argv used to start the project.
func (p Project) Command() []string { return []string{p.Manager, "run", p.Script} }

var skipDirs = map[string]bool{
	"node_modules": true, "vendor": true, "wp-content": true,
	"wp-admin": true, "wp-includes": true, "dist": true, "build": true,
	"__pycache__": true,
}

// frameworks are checked in order; the first dependency found wins.
var frameworks = []struct{ dep, name string }{
	{"astro", "astro"}, {"nuxt", "nuxt"}, {"next", "next"},
	{"@sveltejs/kit", "sveltekit"}, {"@remix-run/dev", "remix"},
	{"@angular/core", "angular"}, {"vite", "vite"}, {"react-scripts", "cra"},
	{"hono", "hono"}, {"fastify", "fastify"}, {"express", "express"},
}

var lockfiles = []struct{ file, manager string }{
	{"bun.lock", "bun"}, {"bun.lockb", "bun"},
	{"pnpm-lock.yaml", "pnpm"}, {"yarn.lock", "yarn"},
}

// Scan walks root recursively and returns every project found, sorted by path.
func Scan(root string) ([]Project, error) {
	root, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	var out []Project
	err = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			if d != nil && d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		if !d.IsDir() {
			return nil
		}
		name := d.Name()
		if path != root && (skipDirs[name] || strings.HasPrefix(name, ".")) {
			return fs.SkipDir
		}
		if p, ok := inspect(root, path); ok {
			out = append(out, p)
		}
		return nil
	})
	sort.Slice(out, func(i, j int) bool { return out[i].Rel < out[j].Rel })
	return out, err
}

func exists(parts ...string) bool {
	_, err := os.Stat(filepath.Join(parts...))
	return err == nil
}

func inspect(root, dir string) (Project, bool) {
	p := Project{Path: dir}
	if dir == root {
		p.Rel = filepath.Base(dir)
	} else {
		p.Rel, _ = filepath.Rel(root, dir)
	}
	branch, wt, hasGit := gitInfo(dir)
	p.Branch, p.Worktree = branch, wt

	switch {
	case exists(dir, "package.json"):
		p.Kind = "node"
		fillNode(&p)
	case exists(dir, "docker-compose.yml"), exists(dir, "docker-compose.yaml"),
		exists(dir, "compose.yml"), exists(dir, "compose.yaml"):
		p.Kind = "compose"
	case exists(dir, "go.mod"):
		p.Kind = "go"
	case exists(dir, "composer.json"):
		p.Kind = "php"
	case hasGit:
		p.Kind = "git"
	default:
		return p, false
	}
	return p, true
}

func fillNode(p *Project) {
	p.Manager = "npm"
	for _, l := range lockfiles {
		if exists(p.Path, l.file) {
			p.Manager = l.manager
			break
		}
	}
	raw, err := os.ReadFile(filepath.Join(p.Path, "package.json"))
	if err != nil {
		return
	}
	var pkg struct {
		Scripts         map[string]string `json:"scripts"`
		Dependencies    map[string]string `json:"dependencies"`
		DevDependencies map[string]string `json:"devDependencies"`
	}
	if json.Unmarshal(raw, &pkg) != nil {
		return
	}
	for _, f := range frameworks {
		if _, ok := pkg.Dependencies[f.dep]; ok {
			p.Framework = f.name
			break
		}
		if _, ok := pkg.DevDependencies[f.dep]; ok {
			p.Framework = f.name
			break
		}
	}
	switch {
	case pkg.Scripts["dev"] != "":
		p.Script = "dev"
	case pkg.Scripts["start"] != "":
		p.Script = "start"
	}
}

// gitInfo reads the checked-out branch without spawning git.
func gitInfo(dir string) (branch string, worktree, hasGit bool) {
	g := filepath.Join(dir, ".git")
	fi, err := os.Stat(g)
	if err != nil {
		return "", false, false
	}
	head := filepath.Join(g, "HEAD")
	if !fi.IsDir() {
		b, err := os.ReadFile(g)
		if err != nil {
			return "", false, true
		}
		gitdir, ok := strings.CutPrefix(strings.TrimSpace(string(b)), "gitdir: ")
		if !ok {
			return "", false, true
		}
		if !filepath.IsAbs(gitdir) {
			gitdir = filepath.Join(dir, gitdir)
		}
		head, worktree = filepath.Join(gitdir, "HEAD"), true
	}
	b, err := os.ReadFile(head)
	if err != nil {
		return "", worktree, true
	}
	s := strings.TrimSpace(string(b))
	if ref, ok := strings.CutPrefix(s, "ref: refs/heads/"); ok {
		return ref, worktree, true
	}
	if len(s) > 7 {
		s = s[:7]
	}
	return s, worktree, true
}
