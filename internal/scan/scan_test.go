package scan

import (
	"os"
	"path/filepath"
	"testing"
)

func write(t *testing.T, root, rel, content string) {
	t.Helper()
	p := filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func fixture(t *testing.T) map[string]Project {
	root := t.TempDir()
	write(t, root, "a/package.json", `{"scripts":{"dev":"astro dev"},"devDependencies":{"astro":"5"}}`)
	write(t, root, "a/bun.lock", "")
	write(t, root, "a/node_modules/x/package.json", `{}`)
	write(t, root, "b/go.mod", "module b")
	write(t, root, "c/.git/HEAD", "ref: refs/heads/develop\n")
	write(t, root, "c/web/package.json", `{"scripts":{"start":"node ."},"dependencies":{"express":"4"}}`)
	write(t, root, "c/web/pnpm-lock.yaml", "")
	write(t, root, "d/docker-compose.yml", "services: {}")
	write(t, root, "e/package.json", `{"name":"lib"}`)
	write(t, root, "wt/.git", "gitdir: "+filepath.Join(root, "c/.git")+"\n")
	write(t, root, ".hidden/package.json", `{}`)

	ps, err := Scan(root)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]Project{}
	for _, p := range ps {
		got[p.Rel] = p
	}
	return got
}

func TestScanFindsProjectsAndSkipsNoise(t *testing.T) {
	got := fixture(t)
	for _, rel := range []string{"a", "b", "c", "c/web", "d", "e", "wt"} {
		if _, ok := got[rel]; !ok {
			t.Errorf("missing project %q", rel)
		}
	}
	for rel := range got {
		if rel == ".hidden" || rel == "a/node_modules/x" {
			t.Errorf("should have skipped %q", rel)
		}
	}
	if len(got) != 7 {
		t.Errorf("want 7 projects, got %d: %v", len(got), got)
	}
}

func TestNodeDetection(t *testing.T) {
	got := fixture(t)
	a := got["a"]
	if a.Kind != "node" || a.Framework != "astro" || a.Manager != "bun" || a.Script != "dev" || !a.Runnable() {
		t.Errorf("unexpected astro project: %+v", a)
	}
	if cmd := a.Command(); len(cmd) != 3 || cmd[0] != "bun" || cmd[2] != "dev" {
		t.Errorf("unexpected command: %v", cmd)
	}
	w := got["c/web"]
	if w.Manager != "pnpm" || w.Script != "start" || w.Framework != "express" {
		t.Errorf("unexpected nested project: %+v", w)
	}
	if e := got["e"]; e.Runnable() || e.Manager != "npm" {
		t.Errorf("library without scripts must not be runnable: %+v", e)
	}
}

func TestGitBranchAndWorktree(t *testing.T) {
	got := fixture(t)
	if got["c"].Branch != "develop" || got["c"].Worktree {
		t.Errorf("repo: %+v", got["c"])
	}
	if !got["wt"].Worktree {
		t.Errorf("worktree not detected: %+v", got["wt"])
	}
	if got["b"].Kind != "go" || got["d"].Kind != "compose" {
		t.Errorf("kinds: b=%s d=%s", got["b"].Kind, got["d"].Kind)
	}
}
