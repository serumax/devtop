package run

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"devtop/internal/scan"
)

// Uses a throwaway project whose dev script spawns a child process, to prove
// Start detaches it, Owned tracks it, and Stop kills the whole process group.
func TestStartStopKillsWholeGroup(t *testing.T) {
	if _, err := exec.LookPath("npm"); err != nil {
		t.Skip("npm not installed")
	}
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	dir := t.TempDir()
	marker := "sleep 4242"
	pkg := `{"scripts":{"dev":"` + marker + `"}}`
	if err := os.WriteFile(filepath.Join(dir, "package.json"), []byte(pkg), 0o644); err != nil {
		t.Fatal(err)
	}
	ps, err := scan.Scan(dir)
	if err != nil || len(ps) != 1 {
		t.Fatalf("scan: %v %v", ps, err)
	}
	p := ps[0]

	if err := Start(p); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = exec.Command("pkill", "-f", marker).Run() })
	time.Sleep(1500 * time.Millisecond)

	if !Owned(p) {
		t.Fatal("project should be owned after Start")
	}
	if err := exec.Command("pgrep", "-f", marker).Run(); err != nil {
		t.Fatal("child process not running after Start")
	}

	b, _ := os.ReadFile(pidPath(p))
	var leader int32
	for _, c := range string(b) {
		leader = leader*10 + int32(c-'0')
	}
	if err := Stop([]int32{leader}, 5*time.Second); err != nil {
		t.Fatal(err)
	}
	time.Sleep(300 * time.Millisecond)
	if Owned(p) {
		t.Error("leader still alive after Stop")
	}
	if err := exec.Command("pgrep", "-f", marker).Run(); err == nil {
		t.Error("child survived Stop: process group was not killed")
	}
}

func TestStartRejectsNonRunnable(t *testing.T) {
	if err := Start(scan.Project{Kind: "go"}); err == nil {
		t.Error("expected error for non-runnable project")
	}
}
