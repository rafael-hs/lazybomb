# lazybomb

A terminal UI for [hey](https://github.com/rakyll/hey) — the HTTP load testing tool.

`lazybomb` wraps `hey` as a library to give you real-time metrics, interactive configuration, and visual feedback directly in your terminal. Inspired by `lazygit` and `lazydocker`.

![demo placeholder]()

---

## Features

### Core

- **Interactive request configuration** — set URL, HTTP method, headers, body, and auth without memorizing flags
- **Concurrency and request control** — configure number of workers (`-c`) and total requests (`-n`) with interactive inputs
- **Duration-based testing** — run tests for a fixed time window instead of a fixed request count (`-z`)
- **Real-time latency histogram** — ASCII bar chart of latency distribution updating live as the test runs
- **Live metrics dashboard** — requests/sec, p50/p90/p99 latencies, error count, and total completed requests updating in real time

### Differential

- **Saved profiles** — store and reload request configurations for frequently tested endpoints
- **Side-by-side comparison** — run two scenarios simultaneously and compare their results (e.g. before/after a deploy)
- **Ramp-up mode** — gradually increase concurrency and observe system behavior under growing load
- **Latency-over-time graph** — sparkline or ASCII chart showing how latency evolves during the test window
- **Export results** — save test output as JSON or CSV for external analysis

### Nice to Have

- **Status code breakdown** — visual split of 2xx / 4xx / 5xx responses with color coding
- **Pause and resume** — interrupt a running test and continue it later
- **Run history** — browse past test results and re-execute any previous configuration

---

## Installation

```bash
go install github.com/rafael-honorio/lazybomb@latest
```

> Requires Go 1.22+

---

## Usage

```bash
lazybomb
```

Use keyboard shortcuts to navigate:

| Key | Action |
|-----|--------|
| `Tab` | Switch panel |
| `Enter` | Start test |
| `Esc` | Stop test |
| `s` | Save profile |
| `l` | Load profile |
| `q` | Quit |

---

## How it works

`lazybomb` imports `hey` as a Go library rather than shelling out to the binary. This gives direct access to in-progress metrics and allows the TUI to update in real time instead of waiting for the test to finish.

TUI is built with [Bubble Tea](https://github.com/charmbracelet/bubbletea) using the [Elm architecture](https://guide.elm-lang.org/architecture/).

---

## License

MIT
