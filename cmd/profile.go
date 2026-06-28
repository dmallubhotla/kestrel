package cmd

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"regexp"
	"strings"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/deepak-science/kestrel/internal/config"
	"github.com/deepak-science/kestrel/internal/kubeconfig"
	"github.com/deepak-science/kestrel/internal/profile"
	"github.com/deepak-science/kestrel/internal/runner"
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
	Use:   "use [target|fuzzy-substring]",
	Short: "Select a target profile (TUI when no arg, fuzzy match when given)",
	Long: `Without an argument, opens an interactive picker over .kestconfig targets.

With an argument, matches (case-insensitive substring) against target names and
the kube-context short name (e.g. the cluster name in an EKS ARN). If exactly
one target matches, it is selected immediately. If multiple match, the picker
is opened pre-filtered to those.

Selecting a target persists it as the active profile AND runs
'kubectl config use-context' for the resolved kube context.`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		targetNames := cfg.TargetNames()
		if len(targetNames) == 0 {
			return fmt.Errorf("no targets configured in .kestconfig")
		}

		if len(args) == 1 {
			matches := matchTargets(cfg, args[0])
			switch len(matches) {
			case 0:
				return fmt.Errorf("no targets match %q (tried target names and kube-context short names)", args[0])
			case 1:
				return selectProfile(matches[0])
			default:
				targetNames = matches
			}
		}

		choice, err := runPicker(targetNames)
		if err != nil {
			return err
		}
		if choice == "" {
			return nil // user cancelled
		}
		return selectProfile(choice)
	},
}

// matchTargets returns target names whose name or kube-context short name
// contains the given query (case-insensitive). An exact target-name match
// short-circuits to that single result.
func matchTargets(c *config.Config, query string) []string {
	if query == "" {
		return nil
	}
	if _, ok := c.Targets[query]; ok {
		return []string{query}
	}
	q := strings.ToLower(query)
	var matches []string
	for _, name := range c.TargetNames() {
		if strings.Contains(strings.ToLower(name), q) {
			matches = append(matches, name)
			continue
		}
		resolved, err := c.ResolveTarget(name)
		if err != nil {
			continue
		}
		if resolved.KubeContext == "" {
			continue
		}
		short := strings.ToLower(kubeconfig.ShortName(resolved.KubeContext))
		if strings.Contains(short, q) {
			matches = append(matches, name)
		}
	}
	return matches
}

// runPicker shows the bubbletea list and returns the chosen target name, or
// "" if the user cancelled.
func runPicker(targetNames []string) (string, error) {
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
		return "", err
	}
	return result.(pickerModel).choice, nil
}

// selectProfile persists the active profile and switches kubectl context.
// The kubectl switch is best-effort: a failure logs a warning but does not
// roll back the profile write (the user can retry kubectl themselves).
func selectProfile(name string) error {
	resolved, err := cfg.ResolveTarget(name)
	if err != nil {
		return err
	}
	if err := profile.Write(name); err != nil {
		return err
	}
	fmt.Printf("Active profile set to %q\n", name)

	if resolved.KubeContext == "" {
		return nil
	}
	if err := runner.Run("kubectl", "config", "use-context", resolved.KubeContext); err != nil {
		slog.Warn("kubectl use-context failed", "context", resolved.KubeContext, "err", err)
		fmt.Fprintf(os.Stderr, "warning: kubectl config use-context %s failed: %v\n", resolved.KubeContext, err)
	}
	return nil
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
	_, _ = fmt.Fprint(w, style.Render(line))
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
