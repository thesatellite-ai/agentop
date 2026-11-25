# Contributing to agentop

Thanks for your interest. `agentop` is open source under the [Apache License 2.0](LICENSE). Contributions are welcome.

## Before you start

- Open an issue describing the bug or feature before large changes, so we can agree on the approach.
- Small fixes (docs, typos, obvious bugs) can go straight to a PR.

## Development

`agentop` is a single Go module — no workspace, no codegen. Requires Go 1.26.4+ on macOS or Linux.

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
  collect/               pipeline on agentio (process table, agent classification,
                         process-to-session join) + the kill path (signals from
                         agentio's stop plan, pid-reuse guard)
  discover/              tty activity and physical memory
  source/                cmux / emdash / runbay / direct launcher classification
  emdash/                read-only emdash DB reader (task names)
  model/                 Session type + idle computation
  cli/                   cobra commands: list, reap, version
  tui/                   tview master-detail dashboard
```

Agent knowledge (which process is which agent, its kind, the conversation it serves, how to stop it) lives in [agentio](https://github.com/ubgo/agentio), not here: add or fix an agent there. agentop keeps the TUI and CLI, launcher labels, and the signal loop. A platform note: terminal-time stat is split across `activity_darwin.go` / `activity_linux.go` for the OS-specific `Stat_t` fields. Keep both in sync when changing the activity logic.

## Pull requests

- Branch from `main`; keep PRs focused.
- `gofmt` clean; `go vet ./...` clean; `go test -race ./...` passes; `golangci-lint run ./...` reports 0 issues.
- Cross-compile must stay green: `goreleaser build --snapshot --clean` (darwin + linux, arm64 + amd64).
- Use [Conventional Commits](https://www.conventionalcommits.org/) (`feat:`, `fix:`, `refactor:`, `docs:`, `chore:`, `test:`). **No AI-attribution / co-author trailers** in commit messages.
- A push to `main` auto-publishes a patch release. Put `[skip release]` in the commit **subject** for docs-only or workflow-only changes.

## Reporting security issues

Do **not** open a public issue for security problems. Contact the maintainer privately via the repo owner's profile.

## Names guard

Before contributing, enable the repo's git hooks once per clone: `git config core.hooksPath .githooks`. They run `scripts/check-names.sh`, which blocks private names (clients, people, internal hosts) from commits, commit messages and pushes. It reads a word list kept outside the repo (`$NAMES_GUARD_FILE` or `~/.config/names-guard/terms.txt`) and skips with a warning when there is none. Run it over the whole tree with `scripts/check-names.sh --all`. Words that are legitimate in this repo go in `.github/names-allow.txt`.
