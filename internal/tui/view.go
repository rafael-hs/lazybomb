package tui

import (
	"fmt"
	"sort"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/rafael-hs/lazybomb/internal/runner"
)

// ── Colour palette ────────────────────────────────────────────────────────────

var (
	colorAccent  = lipgloss.Color("#7C3AED") // purple
	colorSuccess = lipgloss.Color("#10B981") // green
	colorWarn    = lipgloss.Color("#F59E0B") // amber
	colorError   = lipgloss.Color("#EF4444") // red
	colorMuted   = lipgloss.Color("#6B7280") // grey
	colorBg      = lipgloss.Color("#1E1E2E") // dark bg
	colorFg      = lipgloss.Color("#CDD6F4") // light fg
)

// ── Base styles ───────────────────────────────────────────────────────────────

var (
	styleBase = lipgloss.NewStyle().
			Foreground(colorFg).
			Background(colorBg)

	styleBorder = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(colorMuted).
			Padding(0, 1)

	styleBorderActive = styleBorder.
				BorderForeground(colorAccent)

	styleTitle = lipgloss.NewStyle().
			Foreground(colorAccent).
			Bold(true).
			MarginBottom(1)

	styleLabel = lipgloss.NewStyle().
			Foreground(colorMuted).
			Width(14)

	styleValue = lipgloss.NewStyle().
			Foreground(colorFg)

	styleAccent  = lipgloss.NewStyle().Foreground(colorAccent)
	styleSuccess = lipgloss.NewStyle().Foreground(colorSuccess).Bold(true)
	styleWarn    = lipgloss.NewStyle().Foreground(colorWarn).Bold(true)
	styleError   = lipgloss.NewStyle().Foreground(colorError).Bold(true)
	styleMuted   = lipgloss.NewStyle().Foreground(colorMuted)

	styleTab = lipgloss.NewStyle().
			Foreground(colorMuted).
			Padding(0, 2)

	styleTabActive = lipgloss.NewStyle().
			Foreground(colorAccent).
			Bold(true).
			Padding(0, 2).
			Underline(true)

	styleStatusBar = lipgloss.NewStyle().
			Foreground(colorMuted).
			Background(lipgloss.Color("#181825")).
			Width(0). // set dynamically
			Padding(0, 1)
)

// ── View entry point ──────────────────────────────────────────────────────────

func (m Model) View() string {
	if m.width == 0 {
		return "Loading…"
	}

	if m.showHelp {
		return m.helpView()
	}

	tabs := m.tabsView()
	statusBar := m.statusBarView()

	availH := m.height - lipgloss.Height(tabs) - lipgloss.Height(statusBar) - 2

	var body string
	switch m.activePanel {
	case panelConfig:
		body = m.configView(availH)
	case panelMetrics:
		body = m.metricsView(availH)
	case panelProfiles:
		body = m.profilesView(availH)
	}

	return lipgloss.JoinVertical(lipgloss.Left, tabs, body, statusBar)
}

// ── Tabs ──────────────────────────────────────────────────────────────────────

func (m Model) tabsView() string {
	type tabDef struct {
		label string
		p     panel
	}
	defs := []tabDef{
		{"Config", panelConfig},
		{"Metrics", panelMetrics},
		{"Profiles", panelProfiles},
	}

	tabs := make([]string, len(defs))
	for i, d := range defs {
		if d.p == m.activePanel {
			tabs[i] = styleTabActive.Render(d.label)
		} else {
			tabs[i] = styleTab.Render(d.label)
		}
	}

	bar := lipgloss.JoinHorizontal(lipgloss.Left, tabs...)
	sep := styleMuted.Render(strings.Repeat("─", m.width))
	return lipgloss.JoinVertical(lipgloss.Left, bar, sep)
}

// ── Config panel ──────────────────────────────────────────────────────────────

func (m Model) configView(availH int) string {
	rows := make([]string, 0, 24)
	rows = append(rows, styleTitle.Render("Request Configuration"))

	// ── Main fields ───────────────────────────────────────────────────────────
	mainFields := []struct {
		label string
		field configField
	}{
		{"Name", fieldName},
		{"Description", fieldDescription},
		{"URL", fieldURL},
		{"Method", fieldMethod},
		{"Headers", fieldHeaders},
		{"Body", fieldBody},
		{"Requests", fieldRequests},
		{"Concurrency", fieldConcurrency},
		{"Duration", fieldDuration},
		{"Rate limit", fieldRateLimit},
		{"Timeout (s)", fieldTimeout},
	}
	for _, fd := range mainFields {
		label := styleLabel.Render(fd.label)
		input := m.inputs[fd.field].View()
		rows = append(rows, lipgloss.JoinHorizontal(lipgloss.Top, label, input))
	}

	// ── Auth section ──────────────────────────────────────────────────────────
	rows = append(rows, "")
	rows = append(rows, styleMuted.Render("── Auth "+strings.Repeat("─", 50)))

	// Auth type selector
	rows = append(rows, m.authTypeSelector())

	// Auth-specific inputs (only the ones relevant to the current selection).
	authFields := []struct {
		label string
		field configField
	}{
		{"Token", fieldAuthToken},
		{"Username", fieldAuthUser},
		{"Password", fieldAuthPass},
		{"Header", fieldAuthKeyName},
		{"Value", fieldAuthKeyValue},
	}
	for _, af := range authFields {
		if !m.isFieldVisible(af.field) {
			continue
		}
		label := styleLabel.Render(af.label)
		input := m.inputs[af.field].View()
		rows = append(rows, lipgloss.JoinHorizontal(lipgloss.Top, label, input))
	}

	rows = append(rows, "")
	hint := styleMuted.Render("↑↓ navigate  •  enter run  •  ctrl+s save  •  ctrl+l load  •  tab switch panel")
	rows = append(rows, hint)

	content := lipgloss.JoinVertical(lipgloss.Left, rows...)

	borderStyle := styleBorderActive
	if m.activePanel != panelConfig {
		borderStyle = styleBorder
	}

	return borderStyle.
		Width(m.width - 4).
		Height(availH - 2).
		Render(content)
}

// authTypeSelector renders the auth kind picker line.
func (m Model) authTypeSelector() string {
	label := styleLabel.Render("Auth")

	options := make([]string, len(authKindLabels))
	for i, name := range authKindLabels {
		if authKind(i) == m.authKind {
			options[i] = styleSuccess.Render("● " + name)
		} else {
			options[i] = styleMuted.Render("○ " + name)
		}
	}

	selector := strings.Join(options, styleMuted.Render("  "))

	if m.activeField == fieldAuthType {
		hint := styleMuted.Render("  ←/→ change")
		selector = lipgloss.NewStyle().
			Foreground(colorAccent).
			Render("[ "+selector+" ]") + hint
	}

	return lipgloss.JoinHorizontal(lipgloss.Top, label, selector)
}

// ── Metrics panel ─────────────────────────────────────────────────────────────

func (m Model) metricsView(availH int) string {
	snap := m.lastSnap

	leftW := (m.width - 6) / 2
	rightW := m.width - 6 - leftW

	left := m.metricsSummary(snap, leftW, availH-2)
	right := m.histogramView(snap, rightW, availH-2)

	row := lipgloss.JoinHorizontal(lipgloss.Top, left, right)

	borderStyle := styleBorderActive
	if m.activePanel != panelMetrics {
		borderStyle = styleBorder
	}

	header := styleTitle.Render("Live Metrics")
	if m.running {
		header += "  " + styleWarn.Render("● RUNNING")
	} else if m.done {
		if m.stopped {
			header += "  " + styleError.Render("■ STOPPED")
		} else {
			header += "  " + styleSuccess.Render("✓ DONE")
		}
	}

	return borderStyle.
		Width(m.width - 4).
		Height(availH - 2).
		Render(lipgloss.JoinVertical(lipgloss.Left, header, row))
}

func (m Model) metricsSummary(snap runner.Snapshot, w, h int) string {
	lines := []string{
		styleLabel.Render("Elapsed") + styleValue.Render(fmtDuration(snap)),
		styleLabel.Render("Total req") + styleValue.Render(fmt.Sprintf("%d", snap.Total)),
		styleLabel.Render("Success") + styleSuccess.Render(fmt.Sprintf("%d", snap.Success)),
		styleLabel.Render("Errors") + styleError.Render(fmt.Sprintf("%d", snap.Errors)),
		"",
		styleLabel.Render("RPS") + styleValue.Render(fmt.Sprintf("%.2f", snap.RPS)),
		"",
		styleLabel.Render("Fastest") + styleSuccess.Render(fmtLatency(snap.Fastest)),
		styleLabel.Render("Average") + styleValue.Render(fmtLatency(snap.Average)),
		styleLabel.Render("Slowest") + styleError.Render(fmtLatency(snap.Slowest)),
		"",
		styleLabel.Render("p50") + styleValue.Render(fmtLatency(snap.P50)),
		styleLabel.Render("p75") + styleValue.Render(fmtLatency(snap.P75)),
		styleLabel.Render("p90") + styleValue.Render(fmtLatency(snap.P90)),
		styleLabel.Render("p95") + styleWarn.Render(fmtLatency(snap.P95)),
		styleLabel.Render("p99") + styleWarn.Render(fmtLatency(snap.P99)),
		"",
	}

	// Status codes breakdown (sorted for deterministic rendering)
	if len(snap.StatusCodes) > 0 {
		codes := make([]int, 0, len(snap.StatusCodes))
		for code := range snap.StatusCodes {
			codes = append(codes, code)
		}
		sort.Ints(codes)

		lines = append(lines, styleLabel.Render("Status codes"))
		for _, code := range codes {
			count := snap.StatusCodes[code]
			style := styleSuccess
			if code >= 400 && code < 500 {
				style = styleWarn
			} else if code >= 500 || code == 0 {
				style = styleError
			}
			lines = append(lines, "  "+style.Render(fmt.Sprintf("%d: %d", code, count)))
		}
	}

	// Sparkline (latency over time)
	if len(snap.LatencyOverTime) > 1 {
		lines = append(lines, "")
		lines = append(lines, styleLabel.Render("Latency/sec"))
		lines = append(lines, sparkline(snap.LatencyOverTime, w-16))
	}

	return lipgloss.NewStyle().Width(w).Render(
		lipgloss.JoinVertical(lipgloss.Left, lines...),
	)
}

func (m Model) histogramView(snap runner.Snapshot, w, h int) string {
	if len(snap.Histogram) == 0 {
		return lipgloss.NewStyle().Width(w).Render(styleMuted.Render("No data yet"))
	}

	barMaxW := w - 20
	if barMaxW < 5 {
		barMaxW = 5
	}

	maxFreq := 0.0
	for _, b := range snap.Histogram {
		if b.Frequency > maxFreq {
			maxFreq = b.Frequency
		}
	}

	rows := []string{styleMuted.Render("Latency histogram")}
	for _, b := range snap.Histogram {
		if b.Count == 0 {
			continue
		}
		barW := 1
		if maxFreq > 0 {
			barW = int(b.Frequency / maxFreq * float64(barMaxW))
		}
		if barW < 1 {
			barW = 1
		}

		bar := styleAccent.Render(strings.Repeat("█", barW))
		label := lipgloss.NewStyle().Width(8).Render(b.Label)
		count := styleMuted.Render(fmt.Sprintf(" %d", b.Count))
		rows = append(rows, label+bar+count)
	}

	return lipgloss.NewStyle().Width(w).Render(
		lipgloss.JoinVertical(lipgloss.Left, rows...),
	)
}

// ── Profiles panel ────────────────────────────────────────────────────────────

func (m Model) profilesView(availH int) string {
	borderStyle := styleBorderActive
	if m.activePanel != panelProfiles {
		borderStyle = styleBorder
	}

	rows := []string{styleTitle.Render("Saved Profiles")}

	if len(m.profiles) == 0 {
		rows = append(rows, styleMuted.Render("No profiles saved yet."))
		rows = append(rows, styleMuted.Render("Press ctrl+s in Config to save the current settings."))
	} else {
		for i, p := range m.profiles {
			desc := ""
			if p.Description != "" {
				desc = "  " + truncate(p.Description, 35)
			}
			line := fmt.Sprintf("%-30s  %s  n=%d c=%d%s",
				truncate(p.Name, 30), p.Method, p.Requests, p.Concurrency, desc)
			if i == m.profileCursor {
				line = styleTabActive.Render("> " + line)
			} else {
				line = "  " + styleValue.Render(line)
			}
			rows = append(rows, line)
		}
		rows = append(rows, "")
		rows = append(rows, styleMuted.Render("enter load  •  ctrl+d delete  •  esc back"))
	}

	if m.profileErr != "" {
		rows = append(rows, styleError.Render(m.profileErr))
	}

	return borderStyle.
		Width(m.width - 4).
		Height(availH - 2).
		Render(lipgloss.JoinVertical(lipgloss.Left, rows...))
}

// ── Status bar ────────────────────────────────────────────────────────────────

func (m Model) statusBarView() string {
	left := m.statusMsg
	if m.err != "" {
		left = styleError.Render("✗ " + m.err)
	}

	right := styleMuted.Render("tab panels  •  ? help  •  q quit")

	gap := m.width - lipgloss.Width(left) - lipgloss.Width(right)
	if gap < 1 {
		gap = 1
	}
	line := left + strings.Repeat(" ", gap) + right

	return styleStatusBar.Width(m.width).Render(line)
}

// ── Help overlay ──────────────────────────────────────────────────────────────

func (m Model) helpView() string {
	rows := []string{
		styleTitle.Render("lazybomb — keyboard shortcuts"),
		"",
		styleLabel.Render("tab / shift+tab") + styleValue.Render("cycle panels / fields"),
		styleLabel.Render("enter / ctrl+r") + styleValue.Render("run test"),
		styleLabel.Render("esc") + styleValue.Render("stop test / go back"),
		styleLabel.Render("ctrl+s") + styleValue.Render("save profile"),
		styleLabel.Render("ctrl+l") + styleValue.Render("load profile panel"),
		styleLabel.Render("ctrl+d") + styleValue.Render("delete selected profile"),
		styleLabel.Render("↑ / k") + styleValue.Render("move up"),
		styleLabel.Render("↓ / j") + styleValue.Render("move down"),
		styleLabel.Render("?") + styleValue.Render("toggle help"),
		styleLabel.Render("q / ctrl+c") + styleValue.Render("quit"),
	}

	content := lipgloss.JoinVertical(lipgloss.Left, rows...)
	return styleBorderActive.
		Width(m.width - 4).
		Height(m.height - 4).
		Render(content)
}

// ── Helpers ───────────────────────────────────────────────────────────────────

func fmtLatency(secs float64) string {
	if secs == 0 {
		return "—"
	}
	ms := secs * 1000
	if ms >= 1000 {
		return fmt.Sprintf("%.2fs", secs)
	}
	return fmt.Sprintf("%.2fms", ms)
}

func fmtDuration(snap runner.Snapshot) string {
	d := snap.Elapsed
	if d == 0 {
		return "—"
	}
	if d.Hours() >= 1 {
		return fmt.Sprintf("%.0fh%.0fm", d.Hours(), d.Minutes())
	}
	if d.Minutes() >= 1 {
		return fmt.Sprintf("%.0fm%.0fs", d.Minutes(), float64(d.Seconds())-(d.Minutes()*60))
	}
	return fmt.Sprintf("%.1fs", d.Seconds())
}

func sparkline(vals []float64, width int) string {
	const chars = "▁▂▃▄▅▆▇█"
	runes := []rune(chars)

	if width <= 0 || len(vals) == 0 {
		return ""
	}

	// Trim to fit width.
	if len(vals) > width {
		vals = vals[len(vals)-width:]
	}

	min, max := vals[0], vals[0]
	for _, v := range vals {
		if v < min {
			min = v
		}
		if v > max {
			max = v
		}
	}

	var sb strings.Builder
	for _, v := range vals {
		idx := 0
		if max > min {
			idx = int((v - min) / (max - min) * float64(len(runes)-1))
		}
		sb.WriteRune(runes[idx])
	}
	return styleAccent.Render(sb.String())
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n-1] + "…"
}
