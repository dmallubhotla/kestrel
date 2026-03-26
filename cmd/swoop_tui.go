package cmd

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/example/kestrel/internal/swoop"
)

// swoopAction is the action the user selected from the TUI.
type swoopAction struct {
	root   swoop.Root
	action string // "plan", "apply", "init", or "" if cancelled
}

// phase tracks which screen the TUI is on.
type swoopPhase int

const (
	phasePickRoot swoopPhase = iota
	phasePickAction
)

var swoopActions = []string{"plan", "init", "apply"}

// swoopTUIModel is the bubbletea model for the interactive root picker.
type swoopTUIModel struct {
	allRoots []swoop.Root
	filtered []swoop.Root
	state    *swoop.State
	filter   string
	cursor   int
	result   swoopAction
	width    int

	phase       swoopPhase
	actionIdx   int // index into swoopActions
	pickedRoot  swoop.Root
}

func newSwoopTUI(roots []swoop.Root, state *swoop.State) swoopTUIModel {
	return swoopTUIModel{
		allRoots: roots,
		filtered: roots,
		state:    state,
		width:    80,
	}
}

func (m swoopTUIModel) Init() tea.Cmd { return nil }

func (m swoopTUIModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		return m, nil

	case tea.KeyMsg:
		if m.phase == phasePickAction {
			return m.updateActionPicker(msg)
		}
		return m.updateRootPicker(msg)
	}
	return m, nil
}

func (m swoopTUIModel) updateRootPicker(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()

	switch key {
	case "ctrl+c", "esc", "q":
		if key == "q" && m.filter != "" {
			break // fall through to filter append
		}
		return m, tea.Quit

	case "up", "ctrl+p":
		if m.cursor > 0 {
			m.cursor--
		}
		return m, nil
	case "down", "ctrl+n":
		if m.cursor < len(m.filtered)-1 {
			m.cursor++
		}
		return m, nil

	case "enter":
		if len(m.filtered) > 0 {
			m.pickedRoot = m.filtered[m.cursor]
			m.phase = phasePickAction
			m.actionIdx = 0
		}
		return m, nil

	case "backspace", "ctrl+h":
		if len(m.filter) > 0 {
			m.filter = m.filter[:len(m.filter)-1]
			m.applyFilter()
		}
		return m, nil

	case "ctrl+u":
		m.filter = ""
		m.applyFilter()
		return m, nil
	}

	// Append printable characters to filter.
	if msg.Type == tea.KeyRunes {
		m.filter += msg.String()
		m.applyFilter()
		return m, nil
	}

	return m, nil
}

func (m swoopTUIModel) updateActionPicker(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c", "esc", "q":
		return m, tea.Quit

	case "backspace":
		// Go back to root picker.
		m.phase = phasePickRoot
		return m, nil

	case "tab", "right", "down", "l", "j":
		m.actionIdx = (m.actionIdx + 1) % len(swoopActions)
		return m, nil

	case "shift+tab", "left", "up", "h", "k":
		m.actionIdx = (m.actionIdx - 1 + len(swoopActions)) % len(swoopActions)
		return m, nil

	case "enter":
		m.result = swoopAction{root: m.pickedRoot, action: swoopActions[m.actionIdx]}
		return m, tea.Quit
	}

	return m, nil
}

func (m *swoopTUIModel) applyFilter() {
	if m.filter == "" {
		m.filtered = m.allRoots
	} else {
		m.filtered = swoop.Resolve(m.allRoots, m.filter)
	}
	if m.cursor >= len(m.filtered) {
		m.cursor = max(0, len(m.filtered)-1)
	}
}

func (m swoopTUIModel) View() string {
	if m.phase == phasePickAction {
		return m.viewActionPicker()
	}
	return m.viewRootPicker()
}

func (m swoopTUIModel) viewRootPicker() string {
	var b strings.Builder

	// Title and filter.
	title := swoopTitleStyle.Render("Terraform Roots")
	filterStr := m.filter
	if filterStr == "" {
		filterStr = swoopFilterDim.Render("type to filter...")
	}
	b.WriteString(fmt.Sprintf("%s  %s\n\n", title, filterStr))

	// Root list.
	if len(m.filtered) == 0 {
		b.WriteString(swoopDimStyle.Render("  No matching roots\n"))
	} else {
		windowSize := 15
		start, end := listWindow(m.cursor, len(m.filtered), windowSize)

		if start > 0 {
			b.WriteString(swoopDimStyle.Render(fmt.Sprintf("  ... %d more above\n", start)))
		}

		for i := start; i < end; i++ {
			r := m.filtered[i]
			cursor := "  "
			if i == m.cursor {
				cursor = "> "
			}

			init := swoopDimStyle.Render("-")
			if r.Initialized {
				init = swoopGreenStyle.Render("✓")
			}

			ver := swoopDimStyle.Render("-")
			if r.TFVersion != "" {
				ver = r.TFVersion
			}

			activity := lastActivityStr(m.state, r.Path)
			if activity == "-" {
				activity = swoopDimStyle.Render(activity)
			}

			line := fmt.Sprintf("%s%s %s  [%s]  %s  %s",
				cursor,
				init,
				r.Path,
				r.Profile,
				ver,
				activity,
			)

			if i == m.cursor {
				line = swoopSelectedStyle.Render(line)
			}
			b.WriteString(line + "\n")
		}

		if end < len(m.filtered) {
			b.WriteString(swoopDimStyle.Render(fmt.Sprintf("  ... %d more below\n", len(m.filtered)-end)))
		}
	}

	// Preview pane.
	if len(m.filtered) > 0 && m.cursor < len(m.filtered) {
		b.WriteString(m.renderPreview(m.filtered[m.cursor]))
	}

	b.WriteString("\n")
	b.WriteString(swoopHelpStyle.Render("enter: select · esc: quit · type to filter"))
	b.WriteString("\n")

	return b.String()
}

func (m swoopTUIModel) viewActionPicker() string {
	var b strings.Builder

	b.WriteString(swoopTitleStyle.Render(fmt.Sprintf("Action for %s", m.pickedRoot.Path)))
	b.WriteString("\n\n")

	for i, action := range swoopActions {
		if i == m.actionIdx {
			b.WriteString(fmt.Sprintf("  %s ", swoopActionActive.Render(action)))
		} else {
			b.WriteString(fmt.Sprintf("  %s ", swoopActionInactive.Render(action)))
		}
	}
	b.WriteString("\n")

	// Preview of selected root.
	b.WriteString(m.renderPreview(m.pickedRoot))

	b.WriteString("\n")
	b.WriteString(swoopHelpStyle.Render("tab/arrows: cycle · enter: confirm · backspace: back · esc: quit"))
	b.WriteString("\n")

	return b.String()
}

func (m swoopTUIModel) renderPreview(r swoop.Root) string {
	var b strings.Builder
	b.WriteString("\n")
	b.WriteString(swoopPreviewHead.Render(fmt.Sprintf("── %s ──", r.Path)))
	b.WriteString("\n")

	writeField := func(key, val string) {
		b.WriteString(fmt.Sprintf("  %s %s\n", swoopPreviewKey.Render(key+":"), val))
	}

	writeField("profile", r.Profile)
	if r.TFVersion != "" {
		writeField("terraform", r.TFVersion)
	}
	if r.Initialized {
		writeField("init", "yes")
	} else {
		writeField("init", "no")
	}

	if m.state != nil {
		if e, ok := m.state.Entries[r.Path]; ok {
			if e.LastPlan != nil {
				result := ""
				if e.PlanResult != "" {
					result = fmt.Sprintf(" (%s)", e.PlanResult)
				}
				writeField("last plan", relativeTime(*e.LastPlan)+result)
			}
			if e.LastApply != nil {
				writeField("last apply", relativeTime(*e.LastApply))
			}
			if e.LastInit != nil {
				writeField("last init", relativeTime(*e.LastInit))
			}
		}
	}

	awsProfile := swoop.ResolveAWSProfile(r, cfg, environment)
	if awsProfile != "" {
		writeField("aws", awsProfile)
	}

	return b.String()
}

func listWindow(cursor, total, windowSize int) (start, end int) {
	if total <= windowSize {
		return 0, total
	}
	half := windowSize / 2
	start = cursor - half
	if start < 0 {
		start = 0
	}
	end = start + windowSize
	if end > total {
		end = total
		start = end - windowSize
	}
	return start, end
}

// ── styles ──

var (
	swoopTitleStyle    = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("12"))
	swoopSelectedStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("12"))
	swoopDimStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	swoopGreenStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("10"))
	swoopFilterDim     = lipgloss.NewStyle().Foreground(lipgloss.Color("8")).Italic(true)
	swoopHelpStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("7"))
	swoopPreviewHead   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("12"))
	swoopPreviewKey    = lipgloss.NewStyle().Foreground(lipgloss.Color("14"))

	swoopActionActive   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("0")).Background(lipgloss.Color("12")).Padding(0, 1)
	swoopActionInactive = lipgloss.NewStyle().Foreground(lipgloss.Color("7")).Padding(0, 1)
)
