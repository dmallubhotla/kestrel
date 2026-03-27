package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
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
	Short: "Auto-discover AWS profiles and kube contexts, map them to accounts and clusters",
	Long: `Reads AWS profiles from ~/.aws/config and kube contexts from
~/.kube/config, then maps them to kest accounts and clusters.

When a project .kestconfig defines targets (with cluster names), autoconfigure
matches kube contexts to those clusters.

When directories or targets reference AWS accounts, autoconfigure matches
AWS profiles by sso_account_id.`,
	RunE: runAutoconfigure,
}

func init() {
	configCmd.AddCommand(configAutoconfigureCmd)
}

// accountGroup groups directories that share the same AWS account ID.
type accountGroup struct {
	accountID string
	dirNames  []string // sorted
}

// collectAccountIDs gathers unique account IDs from directories and targets.
func collectAccountIDs() []accountGroup {
	byAccount := make(map[string][]string)

	// From directories map.
	for dir, accountID := range cfg.Directories {
		byAccount[accountID] = append(byAccount[accountID], dir)
	}

	groups := make([]accountGroup, 0, len(byAccount))
	for id, dirs := range byAccount {
		sort.Strings(dirs)
		groups = append(groups, accountGroup{accountID: id, dirNames: dirs})
	}
	sort.Slice(groups, func(i, j int) bool {
		return groups[i].dirNames[0] < groups[j].dirNames[0]
	})
	return groups
}

// collectClusters gathers unique cluster names from targets.
func collectClusters() []string {
	seen := make(map[string]bool)
	var clusters []string
	for _, tc := range cfg.Targets {
		if tc.Cluster != "" && !seen[tc.Cluster] {
			seen[tc.Cluster] = true
			clusters = append(clusters, tc.Cluster)
		}
	}
	sort.Strings(clusters)
	return clusters
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

	// Load existing global config to preserve fields.
	global, err := config.LoadGlobal()
	if err != nil {
		global = &config.Config{}
	}
	if global.Accounts == nil {
		global.Accounts = make(map[string]config.AccountConfig)
	}
	if global.Contexts == nil {
		global.Contexts = make(map[string]string)
	}

	// --- AWS profiles: one prompt per account ID ---
	accountGroups := collectAccountIDs()
	if len(accountGroups) > 0 && len(profiles) > 0 {
		fmt.Println("Matching AWS profiles to accounts...")
		fmt.Println()

		for _, grp := range accountGroups {
			dirList := strings.Join(grp.dirNames, ", ")
			fmt.Printf("Account %s  (dirs: %s)\n", grp.accountID, dirList)

			preselect := 0

			// Check existing accounts config.
			if acct, ok := global.Accounts[grp.accountID]; ok && acct.AwsProfile != "" {
				for i, p := range profiles {
					if p.Name == acct.AwsProfile {
						preselect = i + 1
						break
					}
				}
			}

			if preselect == 0 {
				// Try sso_account_id matching.
				for i, p := range profiles {
					ssoID := profileField(p, "sso_account_id")
					if ssoID != "" && ssoID == grp.accountID {
						preselect = i + 1
						break
					}
				}
			}

			m := singleSelectModel{
				title:  fmt.Sprintf("AWS profile for account %s (%s)", grp.accountID, dirList),
				cursor: preselect,
			}
			m.items = make([]selectItem, 0, len(profiles)+1)
			m.items = append(m.items, selectItem{name: "(none)"})
			for _, p := range profiles {
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

			if sm.cursor > 0 {
				selectedProfile := profiles[sm.cursor-1].Name
				global.Accounts[grp.accountID] = config.AccountConfig{
					AwsProfile: selectedProfile,
				}
				fmt.Printf("  aws_profile: %s\n", selectedProfile)
			}
			fmt.Println()
		}
	}

	// --- Kube contexts: one prompt per unique cluster ---
	clusters := collectClusters()
	if len(clusters) > 0 && kubeErr == nil && len(kubeContexts) > 0 {
		fmt.Println("Matching kube contexts to clusters...")
		fmt.Println()

		for _, cluster := range clusters {
			fmt.Printf("Cluster: %s\n", cluster)

			preselect := 0

			// Check existing contexts config.
			if existingCtx, ok := global.Contexts[cluster]; ok {
				for i, c := range kubeContexts {
					if c.Name == existingCtx {
						preselect = i + 1
						break
					}
				}
			}

			if preselect == 0 {
				// Try cluster name matching.
				clusterLower := strings.ToLower(cluster)
				for i, c := range kubeContexts {
					ctxLower := strings.ToLower(c.Name)
					shortLower := strings.ToLower(kubeconfig.ShortName(c.Name))
					if shortLower == clusterLower || strings.Contains(ctxLower, clusterLower) {
						preselect = i + 1
						break
					}
				}
			}

			m := singleSelectModel{
				title:  fmt.Sprintf("Kube context for cluster %s", cluster),
				cursor: preselect,
			}
			m.items = make([]selectItem, 0, len(kubeContexts)+1)
			m.items = append(m.items, selectItem{name: "(none)"})
			for _, c := range kubeContexts {
				m.items = append(m.items, selectItem{
					name:    c.Name,
					preview: formatContextPreview(c),
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

			if sm.cursor > 0 {
				global.Contexts[cluster] = kubeContexts[sm.cursor-1].Name
				fmt.Printf("  kube_context: %s\n", global.Contexts[cluster])
			}
			fmt.Println()
		}
	}

	// Clean config for writing: only accounts and contexts.
	toWrite := &config.Config{
		Accounts: global.Accounts,
		Contexts: global.Contexts,
	}

	for {
		fmt.Println("\nProposed global config:")
		fmt.Println(strings.Repeat("─", 40))
		out, _ := yaml.Marshal(toWrite)
		fmt.Print(string(out))
		fmt.Println(strings.Repeat("─", 40))

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
			continue
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
	preview string
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
	key   string
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
