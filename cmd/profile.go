package cmd

import (
	"fmt"
	"io"
	"sort"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/example/kestrel/internal/config"
	"github.com/example/kestrel/internal/profile"
	"github.com/spf13/cobra"
)

var profileCmd = &cobra.Command{
	Use:   "profile",
	Short: "Manage the active kest environment profile",
}

var profileCurrentCmd = &cobra.Command{
	Use:   "current",
	Short: "Show the active environment profile",
	RunE: func(cmd *cobra.Command, args []string) error {
		env, err := profile.Read()
		if err != nil {
			return err
		}
		if env == "" {
			fmt.Println("No active profile set. Use 'kest profile use' or 'kest profile set <env>'.")
			return nil
		}

		fmt.Printf("environment: %s\n", env)
		envCfg, err := cfg.ResolveEnv(env)
		if err != nil {
			fmt.Printf("  (warning: %v)\n", err)
			return nil
		}
		if envCfg.AwsProfile != "" {
			fmt.Printf("aws_profile: %s\n", envCfg.AwsProfile)
		}
		if envCfg.KubeContext != "" {
			fmt.Printf("kube_context: %s\n", envCfg.KubeContext)
		}
		return nil
	},
}

var profileSetCmd = &cobra.Command{
	Use:   "set <environment>",
	Short: "Set the active environment profile (non-interactive)",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		env := args[0]
		if _, err := cfg.ResolveEnv(env); err != nil {
			return err
		}
		if err := profile.Write(env); err != nil {
			return err
		}
		fmt.Printf("Active profile set to %q\n", env)
		return nil
	},
}

var profileExportCmd = &cobra.Command{
	Use:   "export",
	Short: "Print shell commands to configure the active profile (use with eval)",
	Long:  "Outputs export and kubectl commands for the active profile.\nUsage: eval $(kest profile export)",
	RunE: func(cmd *cobra.Command, args []string) error {
		env, err := profile.Read()
		if err != nil {
			return err
		}
		if env == "" {
			return fmt.Errorf("no active profile set — use 'kest profile use' first")
		}

		envCfg, err := cfg.ResolveEnv(env)
		if err != nil {
			return err
		}

		if envCfg.AwsProfile != "" {
			fmt.Printf("export AWS_PROFILE=%s\n", envCfg.AwsProfile)
		}
		if envCfg.KubeContext != "" {
			fmt.Printf("kubectl config use-context %s\n", envCfg.KubeContext)
		}
		return nil
	},
}

var profileUseCmd = &cobra.Command{
	Use:   "use",
	Short: "Interactively select an environment profile",
	RunE: func(cmd *cobra.Command, args []string) error {
		envNames := sortedEnvNames(cfg)
		if len(envNames) == 0 {
			return fmt.Errorf("no environments configured in .kestconfig")
		}

		current, _ := profile.Read()

		items := make([]list.Item, len(envNames))
		initialIdx := 0
		for i, name := range envNames {
			ec := cfg.Environments[name]
			items[i] = envItem{name: name, envCfg: ec, active: name == current}
			if name == current {
				initialIdx = i
			}
		}

		delegate := envItemDelegate{}
		l := list.New(items, delegate, 40, min(len(items)+6, 20))
		l.Title = "Select environment"
		l.SetShowStatusBar(false)
		l.SetFilteringEnabled(false)
		l.Select(initialIdx)
		l.Styles.Title = lipgloss.NewStyle().Bold(true).Padding(0, 1)

		m := pickerModel{list: l}
		p := tea.NewProgram(m)
		result, err := p.Run()
		if err != nil {
			return err
		}

		pm := result.(pickerModel)
		if pm.choice == "" {
			return nil // user cancelled
		}

		if err := profile.Write(pm.choice); err != nil {
			return err
		}
		fmt.Printf("Active profile set to %q\n", pm.choice)
		return nil
	},
}

func sortedEnvNames(cfg *config.Config) []string {
	names := make([]string, 0, len(cfg.Environments))
	for k := range cfg.Environments {
		names = append(names, k)
	}
	sort.Strings(names)
	return names
}

// --- bubbletea model ---

type envItem struct {
	name   string
	envCfg config.EnvConfig
	active bool
}

func (i envItem) FilterValue() string { return i.name }

type envItemDelegate struct{}

func (d envItemDelegate) Height() int                             { return 1 }
func (d envItemDelegate) Spacing() int                            { return 0 }
func (d envItemDelegate) Update(_ tea.Msg, _ *list.Model) tea.Cmd { return nil }
func (d envItemDelegate) Render(w io.Writer, m list.Model, index int, item list.Item) {
	i := item.(envItem)
	cursor := "  "
	if index == m.Index() {
		cursor = "> "
	}
	marker := "  "
	if i.active {
		marker = "* "
	}

	var detail string
	if i.envCfg.AwsProfile != "" {
		detail = fmt.Sprintf("  (aws: %s)", i.envCfg.AwsProfile)
	}
	line := fmt.Sprintf("%s%s%s%s", cursor, marker, i.name, detail)

	style := lipgloss.NewStyle()
	if index == m.Index() {
		style = style.Bold(true).Foreground(lipgloss.Color("12"))
	}
	fmt.Fprint(w, style.Render(line))
}

type pickerModel struct {
	list   list.Model
	choice string
}

func (m pickerModel) Init() tea.Cmd { return nil }

func (m pickerModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "enter":
			if item, ok := m.list.SelectedItem().(envItem); ok {
				m.choice = item.name
			}
			return m, tea.Quit
		case "q", "esc", "ctrl+c":
			return m, tea.Quit
		}
	case tea.WindowSizeMsg:
		m.list.SetWidth(msg.Width)
	}

	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	return m, cmd
}

func (m pickerModel) View() string {
	return "\n" + m.list.View()
}

func init() {
	profileCmd.AddCommand(profileCurrentCmd)
	profileCmd.AddCommand(profileSetCmd)
	profileCmd.AddCommand(profileExportCmd)
	profileCmd.AddCommand(profileUseCmd)
	rootCmd.AddCommand(profileCmd)
}
