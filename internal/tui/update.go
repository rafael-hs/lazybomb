package tui

import (
	"encoding/base64"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/rafael-hs/lazybomb/internal/profile"
	"github.com/rafael-hs/lazybomb/internal/runner"
)

func (m Model) Init() tea.Cmd {
	return textinputBlink(m)
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case tickMsg:
		m.lastSnap = msg.snap
		return m, nil

	case doneMsg:
		m.lastSnap = msg.snap
		m.running = false
		m.done = true
		m.stopped = msg.stopped
		if msg.stopped {
			m.statusMsg = "Test stopped."
		} else {
			m.statusMsg = fmt.Sprintf("Done — %d requests in %.2fs",
				msg.snap.Total, msg.snap.Elapsed.Seconds())
		}
		return m, nil

	case tea.KeyMsg:
		return m.handleKey(msg)
	}

	return m, nil
}

func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, keys.Quit):
		if m.running {
			m.runner.Stop()
		}
		return m, tea.Quit

	case key.Matches(msg, keys.Help):
		m.showHelp = !m.showHelp
		return m, nil
	}

	switch m.activePanel {
	case panelConfig:
		return m.handleConfigKey(msg)
	case panelProfiles:
		return m.handleProfilesKey(msg)
	case panelMetrics:
		return m.handleMetricsKey(msg)
	}
	return m, nil
}

func (m Model) handleConfigKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// Left/Right only act on the auth type selector.
	if m.activeField == fieldAuthType {
		switch {
		case key.Matches(msg, keys.Left):
			m.authKind = authKind((int(m.authKind) + len(authKindLabels) - 1) % len(authKindLabels))
			return m, nil
		case key.Matches(msg, keys.Right):
			m.authKind = authKind((int(m.authKind) + 1) % len(authKindLabels))
			return m, nil
		}
	}

	switch {
	case key.Matches(msg, keys.Enter):
		if m.running {
			return m, nil
		}
		return m.startTest()

	case key.Matches(msg, keys.Stop):
		if m.running {
			m.runner.Stop()
		}
		return m, nil

	case key.Matches(msg, keys.Save):
		return m.saveProfile()

	case key.Matches(msg, keys.Load):
		m.activePanel = panelProfiles
		return m, nil

	case key.Matches(msg, keys.Tab):
		m = m.cyclePanel(1)
		return m, nil

	case key.Matches(msg, keys.ShiftTab):
		m = m.cyclePanel(-1)
		return m, nil

	case key.Matches(msg, keys.Down):
		m = m.nextField(1)
		return m, textinputBlink(m)

	case key.Matches(msg, keys.Up):
		m = m.nextField(-1)
		return m, textinputBlink(m)
	}

	return m.updateFocusedInput(msg)
}

func (m Model) handleProfilesKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, keys.Up):
		if m.profileCursor > 0 {
			m.profileCursor--
		}
		return m, nil

	case key.Matches(msg, keys.Down):
		if m.profileCursor < len(m.profiles)-1 {
			m.profileCursor++
		}
		return m, nil

	case key.Matches(msg, keys.Enter):
		if len(m.profiles) > 0 {
			m = m.loadProfile(m.profiles[m.profileCursor])
		}
		m.activePanel = panelConfig
		return m, textinputBlink(m)

	case key.Matches(msg, keys.Delete):
		return m.deleteProfile()

	case key.Matches(msg, keys.Stop):
		m.activePanel = panelConfig
		return m, nil

	case key.Matches(msg, keys.Tab):
		m = m.cyclePanel(1)
		return m, nil

	case key.Matches(msg, keys.ShiftTab):
		m = m.cyclePanel(-1)
		return m, nil
	}
	return m, nil
}

func (m Model) handleMetricsKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, keys.Stop):
		if m.running {
			m.runner.Stop()
		}
		return m, nil

	case key.Matches(msg, keys.Tab):
		m = m.cyclePanel(1)
		return m, nil

	case key.Matches(msg, keys.ShiftTab):
		m = m.cyclePanel(-1)
		return m, nil
	}
	return m, nil
}

// startTest builds a runner.Config and launches the test.
func (m Model) startTest() (Model, tea.Cmd) {
	cfg, err := m.buildConfig()
	if err != nil {
		m.err = err.Error()
		return m, nil
	}

	m.err = ""
	m.statusMsg = "Running..."
	m.running = true
	m.done = false
	m.stopped = false
	m.lastSnap = runner.Snapshot{}

	p := currentProgram
	err = m.runner.Start(cfg, 200*time.Millisecond,
		func(snap runner.Snapshot) {
			if p != nil {
				p.Send(tickMsg{snap: snap})
			}
		},
		func(snap runner.Snapshot, stopped bool) {
			if p != nil {
				p.Send(doneMsg{snap: snap, stopped: stopped})
			}
		},
	)
	if err != nil {
		m.running = false
		m.err = err.Error()
		return m, nil
	}

	m.activePanel = panelMetrics
	return m, nil
}

func (m Model) buildConfig() (runner.Config, error) {
	url := strings.TrimSpace(m.inputs[fieldURL].Value())

	requests, _ := strconv.Atoi(m.inputs[fieldRequests].Value())
	if requests <= 0 {
		requests = 200
	}
	concurrency, _ := strconv.Atoi(m.inputs[fieldConcurrency].Value())
	if concurrency <= 0 {
		concurrency = 10
	}
	timeout, _ := strconv.Atoi(m.inputs[fieldTimeout].Value())
	if timeout <= 0 {
		timeout = 20
	}
	rateLimit, _ := strconv.ParseFloat(m.inputs[fieldRateLimit].Value(), 64)

	headers := parseHeaders(m.inputs[fieldHeaders].Value())

	// Inject auth header — auth always wins over a manually typed Authorization header.
	switch m.authKind {
	case authBearer:
		token := strings.TrimSpace(m.inputs[fieldAuthToken].Value())
		if token != "" {
			headers["Authorization"] = "Bearer " + token
		}
	case authBasic:
		user := m.inputs[fieldAuthUser].Value()
		pass := m.inputs[fieldAuthPass].Value()
		encoded := base64.StdEncoding.EncodeToString([]byte(user + ":" + pass))
		headers["Authorization"] = "Basic " + encoded
	case authAPIKey:
		keyName := strings.TrimSpace(m.inputs[fieldAuthKeyName].Value())
		keyValue := m.inputs[fieldAuthKeyValue].Value()
		if keyName != "" {
			headers[keyName] = keyValue
		}
	}

	return runner.Config{
		URL:         url,
		Method:      m.inputs[fieldMethod].Value(),
		Headers:     headers,
		Body:        m.inputs[fieldBody].Value(),
		Requests:    requests,
		Concurrency: concurrency,
		Duration:    strings.TrimSpace(m.inputs[fieldDuration].Value()),
		RateLimit:   rateLimit,
		Timeout:     timeout,
	}, nil
}

func (m Model) saveProfile() (Model, tea.Cmd) {
	name := strings.TrimSpace(m.inputs[fieldName].Value())
	if name == "" {
		m.err = "cannot save: name is required"
		return m, nil
	}
	p := profile.Profile{
		Name:         name,
		Description:  strings.TrimSpace(m.inputs[fieldDescription].Value()),
		URL:          m.inputs[fieldURL].Value(),
		Method:       m.inputs[fieldMethod].Value(),
		Body:         m.inputs[fieldBody].Value(),
		Headers:      parseHeaders(m.inputs[fieldHeaders].Value()),
		Timeout:      intVal(m.inputs[fieldTimeout].Value()),
		Concurrency:  intVal(m.inputs[fieldConcurrency].Value()),
		Requests:     intVal(m.inputs[fieldRequests].Value()),
		Duration:     m.inputs[fieldDuration].Value(),
		RateLimit:    floatVal(m.inputs[fieldRateLimit].Value()),
		AuthKind:     int(m.authKind),
		AuthToken:    m.inputs[fieldAuthToken].Value(),
		AuthUser:     m.inputs[fieldAuthUser].Value(),
		AuthPass:     m.inputs[fieldAuthPass].Value(),
		AuthKeyName:  m.inputs[fieldAuthKeyName].Value(),
		AuthKeyValue: m.inputs[fieldAuthKeyValue].Value(),
	}
	if err := m.store.Save(p); err != nil {
		m.err = "save failed: " + err.Error()
		return m, nil
	}
	profiles, _ := m.store.Load()
	m.profiles = profiles
	m.statusMsg = "Profile saved: " + name
	return m, nil
}

func (m Model) deleteProfile() (Model, tea.Cmd) {
	if len(m.profiles) == 0 {
		return m, nil
	}
	name := m.profiles[m.profileCursor].Name
	if err := m.store.Delete(name); err != nil {
		m.profileErr = err.Error()
		return m, nil
	}
	profiles, _ := m.store.Load()
	m.profiles = profiles
	if m.profileCursor >= len(m.profiles) {
		m.profileCursor = max(0, len(m.profiles)-1)
	}
	m.profileErr = ""
	return m, nil
}

func (m Model) loadProfile(p profile.Profile) Model {
	m.inputs[fieldName].SetValue(p.Name)
	m.inputs[fieldDescription].SetValue(p.Description)
	m.inputs[fieldURL].SetValue(p.URL)
	m.inputs[fieldMethod].SetValue(p.Method)
	m.inputs[fieldBody].SetValue(p.Body)
	m.inputs[fieldRequests].SetValue(strconv.Itoa(p.Requests))
	m.inputs[fieldConcurrency].SetValue(strconv.Itoa(p.Concurrency))
	m.inputs[fieldDuration].SetValue(p.Duration)
	m.inputs[fieldRateLimit].SetValue(strconv.FormatFloat(p.RateLimit, 'f', -1, 64))
	m.inputs[fieldTimeout].SetValue(strconv.Itoa(p.Timeout))

	if len(p.Headers) > 0 {
		parts := make([]string, 0, len(p.Headers))
		for k, v := range p.Headers {
			parts = append(parts, k+": "+v)
		}
		m.inputs[fieldHeaders].SetValue(strings.Join(parts, " | "))
	}

	m.authKind = authKind(p.AuthKind)
	m.inputs[fieldAuthToken].SetValue(p.AuthToken)
	m.inputs[fieldAuthUser].SetValue(p.AuthUser)
	m.inputs[fieldAuthPass].SetValue(p.AuthPass)
	m.inputs[fieldAuthKeyName].SetValue(p.AuthKeyName)
	m.inputs[fieldAuthKeyValue].SetValue(p.AuthKeyValue)
	return m
}

// nextField moves focus to the next (dir=+1) or previous (dir=-1) visible field.
func (m Model) nextField(dir int) Model {
	// Blur current input (virtual auth type field has nothing to blur).
	if m.activeField != fieldAuthType {
		m.inputs[m.activeField].Blur()
	}

	next := configField((int(m.activeField) + int(fieldCount) + dir) % int(fieldCount))
	for !m.isFieldVisible(next) {
		next = configField((int(next) + int(fieldCount) + dir) % int(fieldCount))
	}
	m.activeField = next

	if m.activeField != fieldAuthType {
		m.inputs[m.activeField].Focus()
	}
	return m
}

func (m Model) cyclePanel(dir int) Model {
	panels := []panel{panelConfig, panelMetrics, panelProfiles}
	current := 0
	for i, p := range panels {
		if p == m.activePanel {
			current = i
			break
		}
	}
	m.activePanel = panels[(current+len(panels)+dir)%len(panels)]
	return m
}

func (m Model) updateFocusedInput(msg tea.Msg) (Model, tea.Cmd) {
	// Virtual field — nothing to update.
	if m.activeField == fieldAuthType {
		return m, nil
	}
	var cmd tea.Cmd
	m.inputs[m.activeField], cmd = m.inputs[m.activeField].Update(msg)
	return m, cmd
}

func textinputBlink(m Model) tea.Cmd {
	// Virtual field has no cursor to blink.
	if m.activeField == fieldAuthType {
		return nil
	}
	return m.inputs[m.activeField].Focus()
}

func parseHeaders(raw string) map[string]string {
	headers := map[string]string{}
	for _, part := range strings.Split(raw, "|") {
		part = strings.TrimSpace(part)
		if kv := strings.SplitN(part, ":", 2); len(kv) == 2 {
			headers[strings.TrimSpace(kv[0])] = strings.TrimSpace(kv[1])
		}
	}
	return headers
}

func intVal(s string) int {
	v, _ := strconv.Atoi(s)
	return v
}

func floatVal(s string) float64 {
	v, _ := strconv.ParseFloat(s, 64)
	return v
}
