# devtop

A k9s/htop-style TUI: a recursive inventory of the projects under a root
directory, with live process status and start/stop control.

    devtop              # scan the current directory
    devtop ~/workspace  # scan another root
    devtop -list        # print the inventory once, no TUI

The root is whatever you launch it from; the scan goes recursively inward.
Skipped: `node_modules`, `vendor`, `wp-content`, `wp-admin`, `wp-includes`,
`dist`, `build` and every hidden directory (so `.claude/worktrees` is ignored).

## Install

macOS (Apple Silicon or Intel) and Linux, through Homebrew:

    brew tap serumax/tap
    brew install devtop

Homebrew builds it from source, so Go is pulled in as a build dependency.

## Detection

| Marker | Kind |
| --- | --- |
| `package.json` | node (framework from dependencies, manager from lockfile) |
| `docker-compose.yml` / `compose.yaml` | compose |
| `go.mod` | go |
| `composer.json` | php |
| `.git` only | git |

Only node projects with a `dev` (or else `start`) script can be started.
Other kinds are listed but dimmed.

## Status

A process belongs to the deepest project containing its cwd. It counts when it
listens on a TCP port, or is a JS runtime (node, bun, deno, npm, pnpm, yarn)
with an exact `dev`/`start`/`preview`/`astro`/`nuxt`/`vite`/`next` argument.

## Keys

| Key | Action |
| --- | --- |
| `j`/`k`, arrows, `g`/`G` | move |
| `enter` / `space` | start or stop |
| `r` | restart |
| `l` | log of the selected project |
| `o` | open `localhost:<port>` |
| `/` | filter (name, framework, branch, kind); `esc` clears |
| `f` | cycle view: all, runnable, running |
| `d` | show or hide docker compose projects (hidden by default) |
| `R` | rescan |
| `q` | quit |

Starting runs `<manager> run <script>` in its own session, so it survives
closing devtop. Output goes to `~/.local/state/devtop/<path>.log`. Stopping
sends SIGTERM to the process group, then SIGKILL after 5 s. Stopping a process
devtop did not start asks for confirmation first.

## Not implemented

Compose projects are inventoried but cannot be started or stopped yet, and
their running state is not detected.

## Build

    go build -o devtop . && install devtop ~/.local/bin/
