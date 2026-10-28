// Package run starts and stops projects and manages their logs.
package run

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"devtop/internal/scan"
)

func stateDir() string {
	base := os.Getenv("XDG_STATE_HOME")
	if base == "" {
		home, _ := os.UserHomeDir()
		base = filepath.Join(home, ".local", "state")
	}
	return filepath.Join(base, "devtop")
}

func slug(p scan.Project) string {
	return strings.ReplaceAll(strings.Trim(p.Path, "/"), "/", "_")
}

func LogPath(p scan.Project) string { return filepath.Join(stateDir(), slug(p)+".log") }
func pidPath(p scan.Project) string { return filepath.Join(stateDir(), slug(p)+".pid") }

func alive(pid int) bool {
	return pid > 0 && syscall.Kill(pid, 0) == nil
}

// Owned reports whether devtop started the project and its leader still runs.
func Owned(p scan.Project) bool {
	b, err := os.ReadFile(pidPath(p))
	if err != nil {
		return false
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(b)))
	return err == nil && alive(pid)
}

// Start launches the project in its own session so it outlives the TUI.
func Start(p scan.Project) error {
	if !p.Runnable() {
		return errors.New("no dev/start script")
	}
	if err := os.MkdirAll(stateDir(), 0o755); err != nil {
		return err
	}
	log, err := os.OpenFile(LogPath(p), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	defer log.Close()
	argv := p.Command()
	fmt.Fprintf(log, "\n--- %s: %s ---\n", time.Now().Format(time.RFC3339), strings.Join(argv, " "))

	cmd := exec.Command(argv[0], argv[1:]...)
	cmd.Dir = p.Path
	cmd.Stdout, cmd.Stderr = log, log
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		return err
	}
	_ = os.WriteFile(pidPath(p), []byte(strconv.Itoa(cmd.Process.Pid)), 0o644)
	go cmd.Wait() // reap the child while devtop lives
	return nil
}

// Stop sends SIGTERM to each process (its whole group when it leads one other
// than ours), waits up to grace, then escalates to SIGKILL.
func Stop(pids []int32, grace time.Duration) error {
	own := syscall.Getpgrp()
	signal := func(sig syscall.Signal) {
		for _, pid := range pids {
			if pgid, err := syscall.Getpgid(int(pid)); err == nil && pgid > 1 && pgid != own {
				_ = syscall.Kill(-pgid, sig)
			} else {
				_ = syscall.Kill(int(pid), sig)
			}
		}
	}
	anyAlive := func() bool {
		for _, pid := range pids {
			if alive(int(pid)) {
				return true
			}
		}
		return false
	}
	signal(syscall.SIGTERM)
	deadline := time.Now().Add(grace)
	for time.Now().Before(deadline) {
		if !anyAlive() {
			return nil
		}
		time.Sleep(150 * time.Millisecond)
	}
	signal(syscall.SIGKILL)
	time.Sleep(200 * time.Millisecond)
	if anyAlive() {
		return errors.New("process survived SIGKILL")
	}
	return nil
}

// Tail returns the last n lines of the project's log.
func Tail(p scan.Project, n int) []string {
	f, err := os.Open(LogPath(p))
	if err != nil {
		return []string{"(no log yet)"}
	}
	defer f.Close()
	var ring []string
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 64*1024), 1024*1024)
	for sc.Scan() {
		ring = append(ring, sc.Text())
		if len(ring) > n {
			ring = ring[1:]
		}
	}
	return ring
}
