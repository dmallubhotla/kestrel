package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/example/kestrel/internal/awsconfig"
	"github.com/example/kestrel/internal/config"
	"github.com/example/kestrel/internal/kubeconfig"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

var configAutoconfigureCmd = &cobra.Command{
	Use:   "autoconfigure",
	Short: "Auto-discover AWS profiles and kube contexts, map them to kest environments",
	Long: `Reads AWS profiles from ~/.aws/config and kube contexts from
~/.kube/config. Select which kube contexts to include as environments, then
associate each with an AWS profile. Unmatched AWS profiles also become
environments. The result is written to the global kest config.`,
	RunE: runAutoconfigure,
}

func init() {
	configCmd.AddCommand(configAutoconfigureCmd)
}

func runAutoconfigure(cmd *cobra.Command, args []string) error {
	// Read both sources.
	profiles, err := awsconfig.ReadProfileDetails()
	if err != nil {
		return fmt.Errorf("reading AWS profiles: %w", err)
	}

	kubeContexts, kubeErr := kubeconfig.ReadContexts()

	if len(profiles) == 0 && (kubeErr != nil || len(kubeContexts) == 0) {
		return fmt.Errorf("no AWS profiles or kube contexts found")
	}

	if len(profiles) > 0 {
		names := make([]string, len(profiles))
		for i, p := range profiles {
			names[i] = p.Name
		}
		fmt.Printf("Found %d AWS profile(s): %s\n", len(profiles), strings.Join(names, ", "))
	}

	if kubeErr != nil {
		fmt.Printf("Note: could not read kubeconfig: %v\n", kubeErr)
	} else if len(kubeContexts) > 0 {
		names := make([]string, len(kubeContexts))
		for i, c := range kubeContexts {
			names[i] = kubeconfig.ShortName(c.Name)
		}
		fmt.Printf("Found %d kube context(s): %s\n", len(kubeContexts), strings.Join(names, ", "))
	}
	fmt.Println()

	// Load existing global config to preserve non-environment fields.
	global, err := config.LoadGlobal()
	if err != nil {
		global = &config.Config{}
	}
	if global.Environments == nil {
		global.Environments = make(map[string]config.EnvConfig)
	}

	// Step 1: Select AWS profiles.
	var selectedProfiles []awsconfig.Profile
	if len(profiles) > 0 {
		preselected := make(map[int]bool)
		for i, p := range profiles {
			for _, env := range global.Environments {
				if env.AwsProfile == p.Name {
					preselected[i] = true
					break
				}
			}
		}

		m := multiSelectModel{
			title:    "Select AWS profiles",
			items:    make([]selectItem, len(profiles)),
			selected: preselected,
		}
		for i, p := range profiles {
			m.items[i] = selectItem{
				name:    p.Name,
				preview: formatProfilePreview(p),
			}
		}

		result, err := runTUI(m)
		if err != nil {
			return err
		}
		ms := result.(multiSelectModel)
		if ms.cancelled {
			fmt.Println("Cancelled.")
			return nil
		}

		for i, p := range profiles {
			if ms.selected[i] {
				selectedProfiles = append(selectedProfiles, p)
			}
		}
	}

	// Step 2: Select kube contexts.
	var selectedContexts []kubeconfig.Context
	if kubeErr == nil && len(kubeContexts) > 0 {
		preselected := make(map[int]bool)
		for i, c := range kubeContexts {
			for _, env := range global.Environments {
				if env.KubeContext == c.Name {
					preselected[i] = true
					break
				}
			}
		}

		m := multiSelectModel{
			title:    "Select kube contexts",
			items:    make([]selectItem, len(kubeContexts)),
			selected: preselected,
		}
		for i, c := range kubeContexts {
			m.items[i] = selectItem{
				name:    kubeconfig.ShortName(c.Name),
				preview: formatContextPreview(c),
			}
		}

		result, err := runTUI(m)
		if err != nil {
			return err
		}
		ms := result.(multiSelectModel)
		if ms.cancelled {
			fmt.Println("Cancelled.")
			return nil
		}

		for i, c := range kubeContexts {
			if ms.selected[i] {
				selectedContexts = append(selectedContexts, c)
			}
		}
	}

	if len(selectedProfiles) == 0 && len(selectedContexts) == 0 {
		fmt.Println("Nothing selected.")
		return nil
	}

	// Step 3: For each selected kube context, associate an AWS profile.
	// Track which profiles get matched so unmatched ones become standalone envs.
	matchedProfiles := make(map[string]bool)
	type envEntry struct {
		name        string
		awsProfile  string
		kubeContext string
	}
	var envs []envEntry

	if len(selectedContexts) > 0 && len(selectedProfiles) > 0 {
		fmt.Println()
		for _, ctx := range selectedContexts {
			shortName := kubeconfig.ShortName(ctx.Name)

			// Auto-match: try account ID from ARN, then substring.
			bestIdx := bestProfileForContext(ctx, selectedProfiles)

			// Check existing config for a prior association.
			preselect := 0 // 0 = (none)
			if ec, ok := global.Environments[shortName]; ok && ec.AwsProfile != "" {
				for i, p := range selectedProfiles {
					if p.Name == ec.AwsProfile {
						preselect = i + 1
						break
					}
				}
			} else if bestIdx >= 0 {
				preselect = bestIdx + 1
			}

			m := singleSelectModel{
				title:  fmt.Sprintf("AWS profile for [%s]", shortName),
				cursor: preselect,
			}
			m.items = make([]selectItem, 0, len(selectedProfiles)+1)
			m.items = append(m.items, selectItem{name: "(none)"})
			for _, p := range selectedProfiles {
				m.items = append(m.items, selectItem{
					name:    p.Name,
					preview: formatProfilePreview(p),
				})
			}

			result, err := runTUI(m)
			if err != nil {
				return err
			}
			sm := result.(singleSelectModel)
			if sm.cancelled {
				fmt.Println("Cancelled.")
				return nil
			}

			entry := envEntry{
				name:        shortName,
				kubeContext: ctx.Name,
			}
			if sm.cursor > 0 {
				prof := selectedProfiles[sm.cursor-1]
				entry.awsProfile = prof.Name
				matchedProfiles[prof.Name] = true
				fmt.Printf("  %s → aws:%s  kube:%s\n", shortName, prof.Name, ctx.Name)
			} else {
				fmt.Printf("  %s → kube:%s\n", shortName, ctx.Name)
			}
			envs = append(envs, entry)
		}
	} else if len(selectedContexts) > 0 {
		// Kube contexts only, no AWS profiles to associate.
		for _, ctx := range selectedContexts {
			envs = append(envs, envEntry{
				name:        kubeconfig.ShortName(ctx.Name),
				kubeContext: ctx.Name,
			})
		}
	}

	// Add unmatched AWS profiles as standalone environments.
	for _, p := range selectedProfiles {
		if !matchedProfiles[p.Name] {
			envs = append(envs, envEntry{
				name:       p.Name,
				awsProfile: p.Name,
			})
		}
	}

	// Build the new environments map, preserving extra fields from existing config.
	newEnvs := make(map[string]config.EnvConfig)
	for _, e := range envs {
		ec := global.Environments[e.name]
		if e.awsProfile != "" {
			ec.AwsProfile = e.awsProfile
		}
		if e.kubeContext != "" {
			ec.KubeContext = e.kubeContext
		}
		newEnvs[e.name] = ec
	}
	global.Environments = newEnvs

	toWrite := global

	for {
		// Show proposed config.
		fmt.Println("\nProposed config:")
		fmt.Println(strings.Repeat("─", 40))
		out, _ := yaml.Marshal(toWrite)
		fmt.Print(string(out))
		fmt.Println(strings.Repeat("─", 40))

		// Write / Edit / Cancel.
		cm := confirmModel{}
		result, err := runTUI(cm)
		if err != nil {
			return err
		}
		choice := result.(confirmModel).choice

		switch choice {
		case choiceCancel:
			fmt.Println("Cancelled.")
			return nil
		case choiceEdit:
			edited, err := editInEditor(toWrite)
			if err != nil {
				return err
			}
			if edited == nil {
				fmt.Println("Editor returned empty file, keeping previous version.")
				continue
			}
			toWrite = edited
			continue // loop back to show updated preview
		case choiceWrite:
			// fall through
		}
		break
	}

	if err := config.WriteGlobal(toWrite); err != nil {
		return err
	}
	fmt.Printf("\nConfig written to %s\n", config.GlobalConfigPath())
	return nil
}

// editInEditor writes the config to a temp file, opens $EDITOR, and reads
// back the result. Returns nil if the user empties the file.
func editInEditor(cfg *config.Config) (*config.Config, error) {
	editor := os.Getenv("EDITOR")
	if editor == "" {
		editor = os.Getenv("VISUAL")
	}
	if editor == "" {
		editor = "vi"
	}

	data, err := yaml.Marshal(cfg)
	if err != nil {
		return nil, fmt.Errorf("marshalling config: %w", err)
	}

	tmpDir, err := os.MkdirTemp("", "kest-autoconfigure-*")
	if err != nil {
		return nil, fmt.Errorf("creating temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	tmpFile := filepath.Join(tmpDir, "config.yaml")
	if err := os.WriteFile(tmpFile, data, 0o644); err != nil {
		return nil, fmt.Errorf("writing temp file: %w", err)
	}

	fmt.Println("\nOpening config in editor for review...")
	cmd := exec.Command(editor, tmpFile)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("editor exited with error: %w", err)
	}

	edited, err := os.ReadFile(tmpFile)
	if err != nil {
		return nil, fmt.Errorf("reading edited file: %w", err)
	}
	if len(strings.TrimSpace(string(edited))) == 0 {
		return nil, nil
	}

	var result config.Config
	if err := yaml.Unmarshal(edited, &result); err != nil {
		return nil, fmt.Errorf("parsing edited config: %w", err)
	}
	return &result, nil
}

// bestProfileForContext finds the best AWS profile match for a kube context,
// using account ID from EKS ARNs and name substring matching.
func bestProfileForContext(ctx kubeconfig.Context, profiles []awsconfig.Profile) int {
	bestIdx := -1
	bestScore := 0

	ctxLower := strings.ToLower(ctx.Name)
	ctxAccountID := kubeconfig.ExtractAccountID(ctx.Name)

	for i, prof := range profiles {
		profLower := strings.ToLower(prof.Name)
		accountID := profileField(prof, "sso_account_id")

		// Account ID match is the strongest signal.
		if ctxAccountID != "" && accountID != "" && ctxAccountID == accountID {
			score := 10
			if strings.Contains(ctxLower, profLower) || strings.Contains(profLower, ctxLower) {
				score += 5
			}
			if score > bestScore {
				bestScore = score
				bestIdx = i
			}
			continue
		}

		// Substring match on names.
		if strings.Contains(ctxLower, profLower) || strings.Contains(profLower, ctxLower) {
			score := 5
			if ctxLower == profLower {
				score += 3
			}
			if score > bestScore {
				bestScore = score
				bestIdx = i
			}
		}
	}

	return bestIdx
}

func profileField(p awsconfig.Profile, key string) string {
	for _, f := range p.Fields {
		if f.Key == key {
			return f.Value
		}
	}
	return ""
}

func formatProfilePreview(p awsconfig.Profile) string {
	if len(p.Fields) == 0 {
		return ""
	}
	var b strings.Builder
	for _, f := range p.Fields {
		fmt.Fprintf(&b, "%s = %s\n", acPreviewKey.Render(f.Key), f.Value)
	}
	return b.String()
}

func formatContextPreview(c kubeconfig.Context) string {
	var b strings.Builder
	if c.Cluster != "" {
		fmt.Fprintf(&b, "%s = %s\n", acPreviewKey.Render("cluster"), c.Cluster)
	}
	if c.Namespace != "" {
		fmt.Fprintf(&b, "%s = %s\n", acPreviewKey.Render("namespace"), c.Namespace)
	}
	return b.String()
}

func runTUI(m tea.Model) (tea.Model, error) {
	p := tea.NewProgram(m)
	return p.Run()
}

// ── styles ──

var (
	acTitleStyle  = lipgloss.NewStyle().Bold(true).Padding(0, 1)
	acCursorStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("12"))
	acCheckStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("10"))
	acPreviewHead = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("12")).Padding(1, 0, 0, 2)
	acPreviewKey  = lipgloss.NewStyle().Foreground(lipgloss.Color("14")).PaddingLeft(4)
	acHelpStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("7")).Padding(1, 0, 0, 2)
)

// ── shared item type ──

type selectItem struct {
	name    string
	preview string // pre-formatted preview text
}

// ── multi-select model ──

type multiSelectModel struct {
	title     string
	items     []selectItem
	selected  map[int]bool
	cursor    int
	cancelled bool
}

func (m multiSelectModel) Init() tea.Cmd { return nil }

func (m multiSelectModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}
		case "down", "j":
			if m.cursor < len(m.items)-1 {
				m.cursor++
			}
		case " ":
			if m.selected[m.cursor] {
				delete(m.selected, m.cursor)
			} else {
				m.selected[m.cursor] = true
			}
		case "a":
			if len(m.selected) == len(m.items) {
				m.selected = make(map[int]bool)
			} else {
				for i := range m.items {
					m.selected[i] = true
				}
			}
		case "enter":
			return m, tea.Quit
		case "q", "esc", "ctrl+c":
			m.cancelled = true
			return m, tea.Quit
		}
	}
	return m, nil
}

func (m multiSelectModel) View() string {
	var b strings.Builder

	b.WriteString(acTitleStyle.Render(m.title))
	b.WriteString("\n\n")

	for i, item := range m.items {
		cursor := "  "
		if i == m.cursor {
			cursor = "> "
		}
		check := "[ ] "
		if m.selected[i] {
			check = acCheckStyle.Render("[✓]") + " "
		}
		name := item.name
		if i == m.cursor {
			name = acCursorStyle.Render(name)
		}
		fmt.Fprintf(&b, "%s%s%s\n", cursor, check, name)
	}

	// Preview for highlighted item.
	if preview := m.items[m.cursor].preview; preview != "" {
		b.WriteString(acPreviewHead.Render(fmt.Sprintf("── %s ──", m.items[m.cursor].name)))
		b.WriteString("\n")
		b.WriteString(preview)
	}

	b.WriteString(acHelpStyle.Render("space: toggle · a: all/none · enter: confirm · q: quit"))
	b.WriteString("\n")

	return b.String()
}

// ── single-select model ──

type singleSelectModel struct {
	title     string
	items     []selectItem
	cursor    int
	cancelled bool
}

func (m singleSelectModel) Init() tea.Cmd { return nil }

func (m singleSelectModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}
		case "down", "j":
			if m.cursor < len(m.items)-1 {
				m.cursor++
			}
		case "enter":
			return m, tea.Quit
		case "q", "esc", "ctrl+c":
			m.cancelled = true
			return m, tea.Quit
		}
	}
	return m, nil
}

func (m singleSelectModel) View() string {
	var b strings.Builder

	b.WriteString(acTitleStyle.Render(m.title))
	b.WriteString("\n\n")

	for i, item := range m.items {
		cursor := "  "
		if i == m.cursor {
			cursor = "> "
		}
		name := item.name
		if i == m.cursor {
			name = acCursorStyle.Render(name)
		}
		fmt.Fprintf(&b, "%s%s\n", cursor, name)
	}

	// Preview for highlighted item.
	if preview := m.items[m.cursor].preview; preview != "" {
		b.WriteString(acPreviewHead.Render(fmt.Sprintf("── %s ──", m.items[m.cursor].name)))
		b.WriteString("\n")
		b.WriteString(preview)
	}

	b.WriteString(acHelpStyle.Render("enter: select · q: quit"))
	b.WriteString("\n")

	return b.String()
}

// ── confirm model (write / edit / cancel) ──

const (
	choiceWrite  = "write"
	choiceEdit   = "edit"
	choiceCancel = "cancel"
)

var confirmOptions = []struct {
	label string
	key   string // quick-key
	value string
}{
	{"Write config", "w", choiceWrite},
	{"Edit in $EDITOR", "e", choiceEdit},
	{"Cancel", "c", choiceCancel},
}

type confirmModel struct {
	cursor int
	choice string
}

func (m confirmModel) Init() tea.Cmd { return nil }

func (m confirmModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}
		case "down", "j":
			if m.cursor < len(confirmOptions)-1 {
				m.cursor++
			}
		case "enter":
			m.choice = confirmOptions[m.cursor].value
			return m, tea.Quit
		case "w":
			m.choice = choiceWrite
			return m, tea.Quit
		case "e":
			m.choice = choiceEdit
			return m, tea.Quit
		case "q", "esc", "ctrl+c", "c":
			m.choice = choiceCancel
			return m, tea.Quit
		}
	}
	return m, nil
}

func (m confirmModel) View() string {
	var b strings.Builder
	b.WriteString(acTitleStyle.Render("What next?"))
	b.WriteString("\n\n")

	for i, opt := range confirmOptions {
		cursor := "  "
		if i == m.cursor {
			cursor = "> "
		}
		label := opt.label
		if i == m.cursor {
			label = acCursorStyle.Render(label)
		}
		fmt.Fprintf(&b, "%s%s\n", cursor, label)
	}

	b.WriteString(acHelpStyle.Render("enter: select · w/e/c: quick pick"))
	b.WriteString("\n")
	return b.String()
}
