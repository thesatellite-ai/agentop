<p align="center">
  <img src="assets/logo.svg" alt="agentop" width="440">
</p>

# agentop

**htop for AI coding agents.** It finds every running Claude Code, OpenAI Codex, Gemini CLI, pi and opencode session across cmux, emdash, runbay, and plain terminals. For each one you see how much memory it holds, how long it has been idle, and which task it belongs to. Then you kill the idle ones and get your RAM back. The agent's conversation is preserved; only the process dies.

## Why I built this

Run a few AI coding agents in parallel and you learn a quiet truth: they never leave. Every emdash worktree and every cmux pane spawns a Claude Code process holding 100 MB to over 1 GB of memory, and when you walk away it just stays. Idle. For days.

Stack up a busy week and one afternoon you look up to find twenty-plus forgotten sessions quietly eating 5 to 10 GB, the fans spinning, a 64 GB machine crawling, and no obvious culprit. Activity Monitor is no help: every session hides behind the same opaque version string (`2.1.183`), so you can't tell which is which, how stale it is, or which is safe to kill.

Here's the kicker: they're all safe to kill. A Claude session keeps its history on disk, so you can end the process, take the memory back, and pick up exactly where you left off. The part you care about was never in RAM.

The tools just won't let you do the simple thing:

- **emdash has no "end process" button.** The only way to free a session is to delete the entire task, worktree and all. I didn't want to throw away the work. I wanted the process gone. And emdash restores the session by id the moment you reopen the task, so ending it is exactly right: free it now, get it back later.
- **cmux** stacks up the same way, with no quick read on which panes are actually idle.

So I built `agentop`: one fast view of every agent across every tool, and the one move I actually wanted. Kill the idle, keep the work.

## What it does

- **Discovers** every running agent session across cmux, emdash, runbay, and plain terminals by inspecting the process tree. No integration, no setup.
- **Shows** memory, real idle time, agent, source, the owning task, and what the agent itself recorded (conversation id, busy or idle, version, home) for each one, in a live `htop`-style dashboard.
- **Filters** by source or agent in the sidebar (with per-group memory bars), and by a fuzzy search across the text columns.
- **Reaps** the idle ones, either select-and-kill in the TUI or `agentop reap --idle 2` from the shell. It follows the agent's own stop plan: `SIGTERM` (graceful), then `SIGKILL` for a session still running after the agent's grace period (4 seconds for Claude and Codex), and a confirmation that shows exactly what dies and how much you reclaim. After a stop it prints the line that resumes the conversation when the agent knows it (with `CLAUDE_CONFIG_DIR` for a non-default Claude home, never a token).

Killing frees only the process. emdash restores the conversation when you reopen the task, and cmux and Claude resume from disk. You lose memory pressure, not work.

## Screenshots

<p align="center">
  <img src="assets/screen-sessions.png" alt="agentop — every agent session, sorted by idle" width="820">
</p>

| | |
|:--:|:--:|
| <img src="assets/screen-group.png" width="400" alt="filter by agent"><br>**Filter by agent or source** | <img src="assets/screen-sort.png" width="400" alt="sort picker"><br>**Sort by any column** |
| <img src="assets/screen-reap.png" width="400" alt="reap by idle"><br>**Reap idle sessions in one pass** | <img src="assets/screen-help.png" width="400" alt="keymap"><br>**Full keymap (`?`)** |

## Supported agents and sources

agentop works with **Claude Code**, **OpenAI Codex**, **Gemini CLI**, **pi** and **opencode**. Agent knowledge (which process is which agent, which conversation it serves, how to stop it without losing it) comes from [agentio](https://github.com/ubgo/agentio), so a new agent there is a new agent here. agentop groups each session by the controller that launched it:

- **emdash**: tasks running in [emdash](https://github.com/generalaction/emdash) git worktrees
- **cmux**: sessions in [cmux](https://github.com/) terminal panes
- **runbay**: sessions started by runbay (their shell runs under runbay's pty daemon)
- **direct**: agents started straight from a normal terminal (iTerm, Ghostty, tmux, VS Code)

Only conversation processes are listed. Agent servers, launchers and one-shot agent commands are not sessions and are never listed or reaped: `claude mcp serve` and other `claude mcp` commands, `codex app-server`, `codex mcp-server` and `codex mcp …`, and the npm Codex launcher (`node …/codex.js`, whose native `codex` child is the session). The Claude desktop app, its bundled CLI and helpers, `claude-in-chrome`, Claude's shell snapshots and `npm install` of an agent are not agents at all.

## Install

### Homebrew

```bash
brew install thesatellite-ai/tap/agentop
```

### Install script (macOS or Linux)

```bash
curl -sL https://raw.githubusercontent.com/thesatellite-ai/agentop/main/install.sh | sh
```

### Go

```bash
go install github.com/thesatellite-ai/agentop@latest
```

### From source

```bash
git clone https://github.com/thesatellite-ai/agentop
cd agentop
task build      # builds ./bin/agentop  (or: go build -o bin/agentop .)
```

Requires macOS or Linux. On macOS it reads processes with the system `ps` and `lsof`; on Linux it reads `/proc` directly, so minimal images without `ps` or `lsof` work. The binary is a single static file (pure-Go SQLite, `CGO_ENABLED=0`) with no runtime dependencies.

## Usage

```bash
agentop                              # interactive TUI (default)
agentop list                         # one-shot table to stdout
agentop list --json                  # machine-readable JSON
agentop reap --idle 2                # kill any session idle >= 2 days (asks first)
agentop reap --idle 2 --source cmux  # restrict to one source (emdash|cmux|runbay|direct)
agentop reap --idle 7 --yes          # skip the confirmation prompt
agentop version                      # print version
```

### The TUI

A master-detail dashboard. The **sidebar** filters by two independent axes, source (emdash, cmux, runbay, direct) and agent (claude, codex, gemini, pi, opencode), each with live counts and memory bars. The **table** shows the selected filter and sorts by any column. The **detail pane** follows the cursor with full process info and what the agent recorded about the session: conversation id (and how it was tied to the process: `exact` from the agent's own pid file, `argv` from a resume id on the command line), status, version and home.

| Key | Action |
|---|---|
| `Tab` | switch focus (sidebar and table) |
| `↑/↓`, `j/k` | move cursor |
| `g` / `G` | jump to top / bottom |
| `space` | select or deselect a row |
| `a` / `c` | select all in view / clear selection |
| `x`, `Del` | kill selected (or the cursor row), with confirmation |
| `i` | reap by idle threshold (see scope below) |
| `/` | fuzzy filter across agent, src, task, cwd (live; `Esc` clears) |
| `s` | sort picker (pick a field; press again to flip direction) |
| `r` | refresh |
| `?` | help overlay |
| `q` | quit |

### What reap acts on (scope)

The three kill paths scope differently. This matters, so it is worth being explicit.

| Action | What it kills |
|---|---|
| **TUI `i`** (reap by idle) | the **current view**: the active sidebar filter plus the active `/` quick-filter, narrowed to sessions idle past the threshold you enter |
| **TUI `x`** (kill) | the rows you **selected** with `space`, or the cursor row if none are selected |
| **CLI `agentop reap --idle N`** | **all sources** by default, since the CLI has no sidebar; narrow it with `--source emdash\|cmux\|runbay\|direct` |

So in the TUI, reap follows what you are looking at. If the **emdash** group is selected, `i` reaps **only emdash** sessions and leaves cmux and direct untouched. Add a `/sync` filter on top and `i` reaps only the emdash sessions whose agent, source, task, or cwd matches "sync". The reap dialog's title shows the live scope (for example `reap idle · scope: emdash · /sync`), and the confirmation always lists the exact count and the RAM to be reclaimed before anything dies.

### Safe by design

- **The agent decides what to signal.** agentio's stop plan names the pids (for Gemini CLI, the conversation process and its relaunch parent), the graceful signal, the grace period and whether forcing is safe. agentop sends the signals itself; agentio never does.
- **SIGTERM first**, so the agent shuts down gracefully and flushes its state. agentop then waits for the plan's grace (4 seconds for Claude and Codex); only a session still running after that is escalated to SIGKILL, so a kill does not silently no-op. The TUI (`x`, `i`) and `agentop reap` use this same path.
- **Never the wrong process.** Right before SIGTERM, and again right before any SIGKILL, agentop re-reads the process table and signals only when the pid still names the same agent session it listed (same start time, still the same agent process). A pid that ended or was reused by another process since the list is skipped and reported as "process changed or ended".
- **Servers are not sessions.** `claude mcp serve`, `codex app-server` and the Codex launcher are never listed, so they cannot be reaped.
- **Busy sessions are never reaped by idle.** A session its agent reports busy (working on a turn) shows `busy` and has no idle time, so `i` and `reap --idle` never pick it.
- **Background sessions use the agent's command.** A Claude background session is respawned by its supervisor when killed, so agentop refuses to signal it and prints the `claude stop <id>` command that ends it.
- **A confirmation gate** on every kill shows the count and the RAM you will reclaim.
- **Read-only until you act.** Discovery never touches anything; the only write is the signal you ask for.

## How idle works

Idle is the time since the session last did anything, from two clocks: the terminal's last I/O (the same signal the classic `w` command uses) and the activity time the agent records itself (Claude's session file). The newer one wins, so a turn that streams with a quiet screen does not look idle, and a session without a terminal still has a real idle time. A session the agent reports busy shows `busy` and is never idle.

When neither clock is known, idle falls back to the emdash task's last interaction, then to the process age, marked with a `*` (for example `5d*`) so you know it is an estimate.

## Platform support

macOS and Linux. agentop reads processes through agentio (`ps` and `lsof` on macOS, `/proc` on Linux), ends them with `SIGTERM` and, after the grace period, `SIGKILL` (Unix-only), and reads terminal device timestamps through OS-specific syscalls. Windows uses a different console model and is out of scope.

## Branding

Logo, icon, and the full color palette: [docs/BRANDING.md](docs/BRANDING.md). Assets live in [`assets/`](assets/).

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md). In short: `gofmt` clean, `go vet ./...` clean, `go test ./...` passes, Conventional Commits, and no AI-attribution trailers.

## License

[Apache License 2.0](LICENSE).
