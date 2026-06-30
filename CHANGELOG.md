# Changelog

All notable changes to agentop are documented here. The format follows [Keep a Changelog](https://keepachangelog.com/), and the project aims for [Semantic Versioning](https://semver.org/).

## [0.1.4] - 2026-06-30

### Fixed

- Killing a session now refreshes the process list automatically. The previous code sent SIGTERM and reloaded immediately, before the asynchronous signal had landed, so the dying session reappeared and you had to press `r` to see it go.
- A single `x` (or reap) now reliably reaps a session. The kill waits for the kernel to confirm the process is gone — sending SIGTERM, giving it a grace period to flush, then escalating a non-responsive survivor to SIGKILL — instead of returning while the process was merely mid-shutdown and looking un-killed.

## [0.1.3] - 2026-06-21

First public release.

### Added

- Master-detail TUI: a sidebar that filters by two axes (source: emdash / cmux / direct, and agent: claude / codex) each with live counts and memory bars; a sortable process table (PID / AGENT / MEM / SRC / IDLE / TASK / CWD, color-coded per agent and source); and a detail pane that follows the cursor.
- Agent detection for Claude Code and OpenAI Codex, via a pluggable detector registry (one detector per agent).
- Source classification of cmux, emdash, and direct terminal sessions, detected from the process tree.
- emdash enrichment: task name and status read from emdash's local SQLite (read-only).
- Real idle measurement from the controlling terminal's I/O time (`max(atime, mtime)`), the `w(1)` method. Framework-agnostic across cmux, emdash, and direct.
- `reap` command: kill sessions idle past a threshold, all sources by default, with a `--source` filter and a confirmation gate.
- `list` (with `--json`) and `version` commands.
- TUI sort picker, reap-by-idle modal, kill confirmation, help overlay, vim keys, Tokyo-Night theme.
- Fuzzy quick-filter (`/`): live subsequence match across the agent, source, task, and cwd columns, combined with the sidebar filter. The table title shows the active query and match count.
- Single static binary (pure-Go SQLite, `CGO_ENABLED=0`) for macOS and Linux, distributed via Homebrew tap, install script, and `go install`.

[0.1.4]: https://github.com/thesatellite-ai/agentop/releases/tag/v0.1.4
[0.1.3]: https://github.com/thesatellite-ai/agentop/releases/tag/v0.1.3
