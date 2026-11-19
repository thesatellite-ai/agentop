# Changelog

All notable changes to agentop are documented here. The format follows [Keep a Changelog](https://keepachangelog.com/), and the project aims for [Semantic Versioning](https://semver.org/).

## [Unreleased]

### Fixed

- Kill safety: right before SIGTERM, and again before any SIGKILL, agentop re-reads each process and signals it only when the pid still names the same agent session it listed (same start time). A pid that ended or was reused since the list is skipped and reported as "process changed or ended".
- `agentop reap` now uses the same kill path as the TUI: SIGTERM, a 4 second grace period, an identity re-check, then SIGKILL for survivors. It used to send SIGTERM only and return without waiting. It prints any session that was not cleanly stopped and exits non-zero if one could not be stopped.
- `claude mcp serve`, `codex app-server` / `mcp-server` and the npm Codex launcher are no longer listed as sessions, so they can no longer be reaped.

### Changed

- Process listing, agent matching, process-to-conversation joins and stop planning now come from [agentio](https://github.com/ubgo/agentio). agentop's own `ps` parser, `lsof` pool and agent detectors are gone; it keeps the TUI, the CLI, launcher labels and the signal loop. On Linux processes are read from `/proc`, so `ps` and `lsof` are no longer needed there.
- Idle takes the newer of terminal I/O and the agent's own activity time (Claude's session file). A session the agent reports busy shows `busy` and is never picked by a threshold reap.
- Kills follow the agent's stop plan (targets, graceful signal, grace, whether forcing is safe). A Claude background session is refused with the `claude stop <id>` command that ends it, since its supervisor respawns a killed worker. `agentop reap` prints the resume command of each stopped session when the agent knows the conversation.

### Added

- Gemini CLI, pi and opencode sessions are listed and reapable (Gemini's relaunch parent is stopped with its child).
- The detail pane and `list --json` show the conversation id and how it was tied to the process (`exact`, `argv`), the agent status, version and home.
- runbay source: sessions started by runbay are labelled `runbay` instead of `direct` (detected from runbay's pty daemon in the parent chain). `reap --source runbay` selects them.

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
