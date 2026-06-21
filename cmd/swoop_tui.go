package cmd

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/dmallubhotla/kestrel/internal/swoop"
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

var swoopActions = []string{"plan", "init", "apply", "edit"}

// swoopTUIModel is the bubbletea model for the interactive root picker.
type swoopTUIModel struct {
	allRoots []swoop.Root
	filtered []swoop.Root
	state    *swoop.State
	filter   string
	cursor   int
	result   swoopAction
	width    int

	phase      swoopPhase
	actionIdx  int // index into swoopActions
	pickedRoot swoop.Root

	// profiles maps root.Path to its effective AWS profile, precomputed once
	// (EffectiveProfiles reads backend .tf files) rather than per render.
	profiles map[string]string
}

func newSwoopTUI(roots []swoop.Root, state *swoop.State) swoopTUIModel {
	profiles := make(map[string]string, len(roots))
	for _, r := range roots {
		profiles[r.Path] = swoop.EffectiveProfiles(cfg, r, environment).Effective
	}
	return swoopTUIModel{
		allRoots: roots,
		filtered: roots,
		state:    state,
		width:    80,
		profiles: profiles,
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

	case "e":
		if m.filter == "" && len(m.filtered) > 0 {
			m.result = swoopAction{root: m.filtered[m.cursor], action: "edit"}
			return m, tea.Quit
		}
	case "c":
		if m.filter == "" && len(m.filtered) > 0 {
			m.result = swoopAction{root: m.filtered[m.cursor], action: "cd"}
			return m, tea.Quit
		}

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
	fmt.Fprintf(&b, "%s  %s\n\n", title, filterStr)

	// Root list.
	if len(m.filtered) == 0 {
		b.WriteString(swoopDimStyle.Render("  No matching roots\n"))
	} else {
		windowSize := 15
		start, end := listWindow(m.cursor, len(m.filtered), windowSize)

		if start > 0 {
			b.WriteString(swoopDimStyle.Render(fmt.Sprintf("  ... %d more above\n", start)))
		}

		// Pre-compute row data for column width calculation.
		type tuiRow struct {
			path, dirTag, ver, activity, modified string
			initialized, dirty                    bool
		}
		rows := make([]tuiRow, end-start)
		maxPath, maxDirTag, maxVer, maxActivity := 0, 0, 0, 0
		for i := start; i < end; i++ {
			r := m.filtered[i]
			row := tuiRow{
				path:        r.Path,
				initialized: r.Initialized,
				dirty:       r.GitDirty,
				ver:         "-",
				activity:    lastActivityStr(m.state, r.Path),
				modified:    "-",
			}
			if r.TFVersion != "" {
				row.ver = r.TFVersion
			}
			if !r.TFModified.IsZero() {
				row.modified = relativeTime(r.TFModified)
			}
			// Show dir and AWS profile. Arrow indicates the AWS resolution.
			tag := r.Dir
			if aws := m.profiles[r.Path]; aws != "" && aws != r.Dir {
				tag = fmt.Sprintf("%s → %s", r.Dir, aws)
			}
			row.dirTag = tag
			rows[i-start] = row
			if len(row.path) > maxPath {
				maxPath = len(row.path)
			}
			bracketed := fmt.Sprintf("[%s]", row.dirTag)
			if len(bracketed) > maxDirTag {
				maxDirTag = len(bracketed)
			}
			if len(row.ver) > maxVer {
				maxVer = len(row.ver)
			}
			if len(row.activity) > maxActivity {
				maxActivity = len(row.activity)
			}
		}

		for i, row := range rows {
			idx := start + i
			cursor := "  "
			if idx == m.cursor {
				cursor = "> "
			}

			init := swoopDimStyle.Render("-")
			if row.initialized {
				init = swoopGreenStyle.Render("✓")
			}

			dirty := " "
			if row.dirty {
				dirty = swoopWarnStyle.Render("*")
			}

			paddedPath := fmt.Sprintf("%-*s", maxPath, row.path)
			dirLabel := fmt.Sprintf("[%s]", row.dirTag)
			paddedDir := fmt.Sprintf("%-*s", maxDirTag, dirLabel)
			paddedVer := fmt.Sprintf("%-*s", maxVer, row.ver)
			paddedActivity := fmt.Sprintf("%-*s", maxActivity, row.activity)

			ver := paddedVer
			if row.ver == "-" {
				ver = swoopDimStyle.Render(paddedVer)
			}

			activity := paddedActivity
			if row.activity == "-" {
				activity = swoopDimStyle.Render(paddedActivity)
			}

			modified := row.modified
			if row.modified == "-" {
				modified = swoopDimStyle.Render(row.modified)
			}

			line := fmt.Sprintf("%s%s%s %s  %s  %s  %s  %s",
				cursor,
				init,
				dirty,
				paddedPath,
				paddedDir,
				ver,
				activity,
				modified,
			)

			if idx == m.cursor {
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
	b.WriteString(swoopHelpStyle.Render("enter: select · e: edit · c: cd · esc: quit · type to filter"))
	b.WriteString("\n")

	return b.String()
}

func (m swoopTUIModel) viewActionPicker() string {
	var b strings.Builder

	b.WriteString(swoopTitleStyle.Render(fmt.Sprintf("Action for %s", m.pickedRoot.Path)))
	b.WriteString("\n\n")

	for i, action := range swoopActions {
		if i == m.actionIdx {
			fmt.Fprintf(&b, "  %s ", swoopActionActive.Render(action))
		} else {
			fmt.Fprintf(&b, "  %s ", swoopActionInactive.Render(action))
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
		fmt.Fprintf(&b, "  %s %s\n", swoopPreviewKey.Render(key+":"), val)
	}

	writeField("dir", r.Dir)
	if r.AccountID != "" {
		writeField("account", r.AccountID)
	}
	if r.TFVersion != "" {
		writeField("terraform", r.TFVersion)
	}
	if r.Initialized {
		writeField("init", "yes")
	} else {
		writeField("init", "no")
	}
	if !r.TFModified.IsZero() {
		writeField("modified", relativeTime(r.TFModified))
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

	if awsProfile := m.profiles[r.Path]; awsProfile != "" {
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
	swoopTitleStyle    = lipgloss.NewStyle().Bold(true).Foreground(colorAccent)
	swoopSelectedStyle = lipgloss.NewStyle().Bold(true).Foreground(colorAccent)
	swoopDimStyle      = lipgloss.NewStyle().Foreground(colorDim)
	swoopGreenStyle    = lipgloss.NewStyle().Foreground(colorSuccess)
	swoopWarnStyle     = lipgloss.NewStyle().Foreground(colorWarn)
	swoopFilterDim     = lipgloss.NewStyle().Foreground(colorDim).Italic(true)
	swoopHelpStyle     = lipgloss.NewStyle().Foreground(colorHelp)
	swoopPreviewHead   = lipgloss.NewStyle().Bold(true).Foreground(colorAccent)
	swoopPreviewKey    = lipgloss.NewStyle().Foreground(colorKey)

	swoopActionActive   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("0")).Background(colorAccent).Padding(0, 1)
	swoopActionInactive = lipgloss.NewStyle().Foreground(colorHelp).Padding(0, 1)
)
