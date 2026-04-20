package cmd

import (
	"fmt"
	"io"
	"regexp"
	"strings"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/example/kestrel/internal/config"
	"github.com/example/kestrel/internal/profile"
	"github.com/spf13/cobra"
)

var profileCmd = &cobra.Command{
	Use:     "profile",
	Short:   "Manage the active kest target profile",
	GroupID: "config",
}

var profileCurrentCmd = &cobra.Command{
	Use:   "current",
	Short: "Show the active target profile",
	RunE: func(cmd *cobra.Command, args []string) error {
		target, err := profile.Read()
		if err != nil {
			return err
		}
		if target == "" {
			fmt.Println("No active profile set. Use 'kest profile use' or 'kest profile set <target>'.")
			return nil
		}

		fmt.Printf("target: %s\n", target)
		resolved, err := cfg.ResolveTarget(target)
		if err != nil {
			fmt.Printf("  (warning: %v)\n", err)
			return nil
		}
		if resolved.KubeContext != "" {
			fmt.Printf("kube_context: %s\n", resolved.KubeContext)
		}
		if resolved.AwsProfile != "" {
			fmt.Printf("aws_profile:  %s\n", resolved.AwsProfile)
		}
		return nil
	},
}

var profileSetCmd = &cobra.Command{
	Use:   "set <target>",
	Short: "Set the active target profile (non-interactive)",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		target := args[0]
		if _, err := cfg.ResolveTarget(target); err != nil {
			return err
		}
		if err := profile.Write(target); err != nil {
			return err
		}
		fmt.Printf("Active profile set to %q\n", target)
		return nil
	},
}

// safeShellValue matches strings that are safe to use unquoted in shell.
var safeShellValue = regexp.MustCompile(`^[a-zA-Z0-9._:/@-]+$`)

func shellQuote(s string) string {
	if safeShellValue.MatchString(s) {
		return s
	}
	quoted := "'"
	for _, c := range s {
		if c == '\'' {
			quoted += `'\''`
		} else {
			quoted += string(c)
		}
	}
	quoted += "'"
	return quoted
}

var profileExportCmd = &cobra.Command{
	Use:   "export",
	Short: "Print shell commands to configure the active profile (use with eval)",
	Long: `Outputs export and kubectl commands for the active profile.

Usage:
  eval "$(kest profile export)"

Add to your shell rc for automatic activation:
  # ~/.bashrc or ~/.zshrc
  eval "$(kest profile export)"`,
	RunE: func(cmd *cobra.Command, args []string) error {
		target, err := profile.Read()
		if err != nil {
			return err
		}
		if target == "" {
			return fmt.Errorf("no active profile set — use 'kest profile use' first")
		}

		resolved, err := cfg.ResolveTarget(target)
		if err != nil {
			return err
		}

		if resolved.AwsProfile != "" {
			fmt.Printf("export AWS_PROFILE=%s\n", shellQuote(resolved.AwsProfile))
		}
		if resolved.KubeContext != "" {
			fmt.Printf("kubectl config use-context %s\n", shellQuote(resolved.KubeContext))
		}
		return nil
	},
}

var profileUseCmd = &cobra.Command{
	Use:   "use",
	Short: "Interactively select a target profile",
	RunE: func(cmd *cobra.Command, args []string) error {
		targetNames := cfg.TargetNames()
		if len(targetNames) == 0 {
			return fmt.Errorf("no targets configured in .kestconfig")
		}

		current, _ := profile.Read()

		items := make([]list.Item, len(targetNames))
		initialIdx := 0
		for i, name := range targetNames {
			tc := cfg.Targets[name]
			items[i] = targetItem{name: name, target: tc, cfg: cfg, active: name == current}
			if name == current {
				initialIdx = i
			}
		}

		delegate := targetItemDelegate{}
		l := list.New(items, delegate, 40, min(len(items)+6, 20))
		l.Title = "Select target"
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

// --- bubbletea model ---

type targetItem struct {
	name   string
	target config.TargetConfig
	cfg    *config.Config
	active bool
}

func (i targetItem) FilterValue() string { return i.name }

type targetItemDelegate struct{}

func (d targetItemDelegate) Height() int                             { return 1 }
func (d targetItemDelegate) Spacing() int                            { return 0 }
func (d targetItemDelegate) Update(_ tea.Msg, _ *list.Model) tea.Cmd { return nil }
func (d targetItemDelegate) Render(w io.Writer, m list.Model, index int, item list.Item) {
	i := item.(targetItem)
	cursor := "  "
	if index == m.Index() {
		cursor = "> "
	}
	marker := "  "
	if i.active {
		marker = "* "
	}

	var details []string
	if i.target.Cluster != "" {
		details = append(details, i.target.Cluster)
	}
	if resolved, err := i.cfg.ResolveTarget(i.name); err == nil && resolved.AwsProfile != "" {
		details = append(details, fmt.Sprintf("aws: %s", resolved.AwsProfile))
	}
	var detail string
	if len(details) > 0 {
		detail = fmt.Sprintf("  (%s)", strings.Join(details, ", "))
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
			if item, ok := m.list.SelectedItem().(targetItem); ok {
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
	profileSetCmd.ValidArgsFunction = completeTargetNames

	profileCmd.AddCommand(profileCurrentCmd)
	profileCmd.AddCommand(profileSetCmd)
	profileCmd.AddCommand(profileExportCmd)
	profileCmd.AddCommand(profileUseCmd)
	rootCmd.AddCommand(profileCmd)
}
