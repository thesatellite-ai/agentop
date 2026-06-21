# Contributing to agentop

Thanks for your interest. `agentop` is open source under the [Apache License 2.0](LICENSE). Contributions are welcome.

## Before you start

- Open an issue describing the bug or feature before large changes, so we can agree on the approach.
- Small fixes (docs, typos, obvious bugs) can go straight to a PR.

## Development

`agentop` is a single Go module — no workspace, no codegen. Requires Go 1.22+ on macOS or Linux.

```sh
task build      # build ./bin/agentop
task run        # build + run the TUI
task list       # build + run one-shot list mode
task test       # go test ./...
task fmt        # gofmt -w .
task vet        # go vet ./...
task snapshot   # local goreleaser snapshot (no publish)
```

(No `task`? Use the underlying commands — `go build -o bin/agentop .`, `go test ./...`, etc. See `Taskfile.yml`.)

### Layout

```
main.go                  entry point (cobra)
internal/
  agent/                 pluggable agent-detector registry (Claude today)
  discover/              process listing (ps), cwd (lsof), tty activity, sysmem
  source/                cmux / emdash / direct classification
  emdash/                read-only emdash DB reader (task names)
  model/                 Session type + idle computation
  collect/               pipeline orchestration + kill
  cli/                   cobra commands: list, reap, version
  tui/                   tview master-detail dashboard
```

A platform note: process/terminal introspection is Unix-only. cwd resolution lives in `discover/cwd_unix.go` (`//go:build darwin || linux`); terminal-time stat is split across `activity_darwin.go` / `activity_linux.go` for the OS-specific `Stat_t` fields. Keep both in sync when changing the activity logic.

## Pull requests

- Branch from `main`; keep PRs focused.
- `gofmt` clean; `go vet ./...` clean; `go test ./...` passes.
- Cross-compile must stay green: `goreleaser build --snapshot --clean` (darwin + linux, arm64 + amd64).
- Use [Conventional Commits](https://www.conventionalcommits.org/) (`feat:`, `fix:`, `refactor:`, `docs:`, `chore:`, `test:`). **No AI-attribution / co-author trailers** in commit messages.
- A push to `main` auto-publishes a patch release. Put `[skip release]` in the commit **subject** for docs-only or workflow-only changes.

## Reporting security issues

Do **not** open a public issue for security problems. Contact the maintainer privately via the repo owner's profile.
