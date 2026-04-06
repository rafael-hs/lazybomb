package tui

import (
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/rafael-hs/lazybomb/internal/profile"
	"github.com/rafael-hs/lazybomb/internal/runner"
)

// panel identifies which UI panel is active.
type panel int

const (
	panelConfig panel = iota
	panelMetrics
	panelProfiles
)

// configField identifies which input field is focused.
type configField int

const (
	fieldName        configField = iota // profile identifier
	fieldDescription                   // optional profile note
	fieldURL
	fieldMethod
	fieldHeaders
	fieldBody
	fieldRequests
	fieldConcurrency
	fieldDuration
	fieldRateLimit
	fieldTimeout

	// Auth section
	fieldAuthType     // virtual selector — no text input, Left/Right cycles authKind
	fieldAuthToken    // Bearer token
	fieldAuthUser     // Basic username
	fieldAuthPass     // Basic password
	fieldAuthKeyName  // API Key header name
	fieldAuthKeyValue // API Key value
	fieldCount
)

// authKind identifies which auth method is selected.
type authKind int

const (
	authNone   authKind = iota
	authBearer          // Authorization: Bearer <token>
	authBasic           // Authorization: Basic base64(user:pass)
	authAPIKey          // custom header: <name>: <value>
)

var authKindLabels = []string{"None", "Bearer", "Basic", "API Key"}

// Model is the root Bubble Tea model.
type Model struct {
	// layout
	width  int
	height int

	// active panel
	activePanel panel

	// config panel state
	activeField configField
	inputs      [fieldCount]textinput.Model
	authKind    authKind

	// profiles panel state
	store         *profile.Store
	profiles      []profile.Profile
	profileCursor int
	profileErr    string

	// runner
	runner   *runner.Runner
	running  bool
	lastSnap runner.Snapshot
	done     bool
	stopped  bool

	// status bar
	statusMsg string
	err       string

	// show help overlay
	showHelp bool
}

// InitialModel returns the default application state, or an error if the
// profile store cannot be initialised.
func InitialModel() (Model, error) {
	type spec struct {
		placeholder string
		width       int
		charLimit   int
		password    bool
	}

	specs := [fieldCount]spec{
		fieldName:        {"my-profile", 30, 128, false},
		fieldDescription: {"short description of this profile", 60, 256, false},
		fieldURL:         {"https://example.com", 60, 512, false},
		fieldMethod:      {"GET", 8, 16, false},
		fieldHeaders:     {"Content-Type: application/json | X-Custom: value", 60, 512, false},
		fieldBody:        {"", 60, 4096, false},
		fieldRequests:    {"200", 10, 10, false},
		fieldConcurrency: {"10", 10, 6, false},
		fieldDuration:    {"30s", 12, 16, false},
		fieldRateLimit:   {"0", 10, 10, false},
		fieldTimeout:     {"20", 10, 6, false},
		// Auth — fieldAuthType is virtual, skip it
		fieldAuthType:     {"", 0, 0, false},
		fieldAuthToken:    {"eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...", 70, 4096, false},
		fieldAuthUser:     {"username", 30, 256, false},
		fieldAuthPass:     {"password", 30, 256, true},
		fieldAuthKeyName:  {"X-API-Key", 20, 128, false},
		fieldAuthKeyValue: {"your-api-key-value", 50, 512, false},
	}

	var inputs [fieldCount]textinput.Model
	for i := range inputs {
		if configField(i) == fieldAuthType {
			continue // virtual field — no text input
		}
		t := textinput.New()
		t.Placeholder = specs[i].placeholder
		t.Width = specs[i].width
		t.CharLimit = specs[i].charLimit
		if specs[i].password {
			t.EchoMode = textinput.EchoPassword
			t.EchoCharacter = '•'
		}
		inputs[i] = t
	}

	inputs[fieldName].Focus()

	// Sensible defaults
	inputs[fieldMethod].SetValue("GET")
	inputs[fieldRequests].SetValue("200")
	inputs[fieldConcurrency].SetValue("10")
	inputs[fieldTimeout].SetValue("20")

	store, err := profile.NewStore()
	if err != nil {
		return Model{}, err
	}
	profiles, _ := store.Load()

	return Model{
		inputs:      inputs,
		activePanel: panelConfig,
		runner:      runner.New(),
		store:       store,
		profiles:    profiles,
	}, nil
}

// isFieldVisible reports whether a field should be shown/reachable given the
// current auth selection. Non-auth fields are always visible.
func (m Model) isFieldVisible(f configField) bool {
	switch f {
	case fieldAuthToken:
		return m.authKind == authBearer
	case fieldAuthUser, fieldAuthPass:
		return m.authKind == authBasic
	case fieldAuthKeyName, fieldAuthKeyValue:
		return m.authKind == authAPIKey
	default:
		return true
	}
}
