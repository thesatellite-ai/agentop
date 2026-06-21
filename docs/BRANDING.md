# Branding

The visual identity for agentop: assets, the full color palette, and how the logo is built. The palette is shared between the brand assets and the TUI theme (`internal/tui/theme.go`), so the product and its marks read as one thing. The base palette is [Tokyo Night](https://github.com/enkia/tokyo-night-vscode-theme).

## Assets

| File | What it is |
|---|---|
| [`assets/icon.svg`](../assets/icon.svg) | **App icon.** A terminal window (traffic-light dots) with process-meter bars in the brand palette (blue, teal, violet) and an amber cursor block as the "reap target." Scales clean from favicon to 1024px. |
| [`assets/logo.svg`](../assets/logo.svg) | **Horizontal lockup.** Icon + monospace `agentop` wordmark (`agent` in light, `op` in blue) + the tagline. Monospace because it's a CLI tool. |

The icon is the product in miniature: a process monitor. The three dots read "terminal window", the three meter bars read "memory and activity, of decreasing length", and the amber block at the end of the shortest bar is the idle session you are about to reap. Every shape maps to something agentop does.

## Color palette

One palette, used by both the SVG assets and the TUI. The hex values are defined once in `internal/tui/theme.go`.

### Surface and structure

| Token | Hex | Role |
|---|---|---|
| Base | `#1A1B26` | app background (TUI panes, icon fill base) |
| Icon gradient | `#24283B` to `#16161E` | icon background gradient (top-left to bottom-right) |
| HeaderBG | `#1F2335` | header, footer, and modal background |
| BarTrack | `#292E42` | unfilled meter track |
| SelectedBG | `#283457` | selected sidebar row |
| CursorBG | `#2E3C64` | table cursor row |
| Border | `#3B4261` | pane borders (idle) |
| BorderActive | `#7AA2F7` | focused pane border |

### Text and accent

| Token | Hex | Role |
|---|---|---|
| Accent | `#7AA2F7` | brand blue: `op` in the wordmark, active focus, primary bar |
| Text / Header | `#C0CAF5` | primary text, `agent` in the wordmark |
| Footer / Muted | `#565F89` | secondary text, tagline, section headers |

### Semantic (status)

| Token | Hex | Role |
|---|---|---|
| Good | `#9ECE6A` | green: healthy, selected check, idle under 1 day |
| Warn | `#E0AF68` | amber: idle 1 day or more, the cursor / reap block |
| Danger | `#F7768E` | red: idle 3 days or more, the first terminal dot |

### Source colors (sidebar and SRC column)

| Source | Hex | Note |
|---|---|---|
| emdash | `#73DACA` | teal |
| cmux | `#BB9AF7` | violet |
| direct | `#E0AF68` | amber |

### Agent colors (AGENT column)

| Agent | Hex | Note |
|---|---|---|
| claude | `#FF9E64` | orange |
| codex | `#7DCFFF` | cyan |

The agent colors are chosen to stay clear of the source colors so the AGENT and SRC columns never blur together. `direct` (amber) and `claude` (orange) are deliberately different oranges. If you add an agent, give it a hue not already used by a source.

## Logo construction

- **Wordmark:** monospace (`SF Mono`, `JetBrains Mono`, `Fira Code` fallback stack), weight 700, letter-spacing `-2`. `agent` in Text `#C0CAF5`, `op` in Accent `#7AA2F7`. The split puts the brand color on the suffix that makes it a tool name (`-op`, as in htop, atop, btop).
- **Tagline:** "htop for AI coding agents", same monospace, Muted `#565F89`.
- **Icon to wordmark:** the icon is the full mark scaled to `0.72` inside the lockup; both share the `bg` gradient definition.

## Usage

- **Favicon and small sizes:** use `icon.svg`. The wordmark is unreadable below about 64px wide.
- **README and docs header:** `logo.svg` centered.
- **Backgrounds:** the icon sits on its own dark surface, so it works on any background. On a light page, the dark squircle provides its own contrast.
- **Avoid:** recoloring the wordmark halves, swapping the monospace for a proportional face, or placing the icon bars on a non-dark fill (the `#292E42` tracks vanish on light).

## Regenerate the PNGs

```bash
rsvg-convert -w 256  assets/icon.svg -o icon-256.png
rsvg-convert -w 1024 assets/icon.svg -o icon-1024.png
rsvg-convert -w 760  assets/logo.svg -o logo.png
```
