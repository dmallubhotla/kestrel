package cmd

import (
	"fmt"
	"io"
	"log/slog"
	"sort"
	"strings"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/dmallubhotla/kestrel/internal/config"
	"github.com/dmallubhotla/kestrel/internal/kubeconfig"
	"github.com/dmallubhotla/kestrel/internal/runner"
	"github.com/spf13/cobra"
)

var contextCmd = &cobra.Command{
	Use:     "context",
	Aliases: []string{"ctx"},
	Short:   "Switch the current kubectl context",
	GroupID: "config",
}

// ContextEntry is one row in the union of global-config + kubeconfig contexts.
type ContextEntry struct {
	DisplayName string // cluster key from kest global config, or ShortName(context)
	Context     string // the kubectl context string passed to use-context
	InGlobalCfg bool
	InKubeCfg   bool
}

// mergeContextSources unions kest global config contexts with kubeconfig
// contexts. Global-config entries keep their friendly display names; entries
// only in kubeconfig get ShortName-derived names. Results are sorted by
// display name.
func mergeContextSources(kestContexts map[string]string, kubeContexts []kubeconfig.Context) []ContextEntry {
	byContext := map[string]*ContextEntry{}
	for cluster, ctxName := range kestContexts {
		byContext[ctxName] = &ContextEntry{
			DisplayName: cluster,
			Context:     ctxName,
			InGlobalCfg: true,
		}
	}
	for _, kc := range kubeContexts {
		if existing, ok := byContext[kc.Name]; ok {
			existing.InKubeCfg = true
			continue
		}
		byContext[kc.Name] = &ContextEntry{
			DisplayName: kubeconfig.ShortName(kc.Name),
			Context:     kc.Name,
			InKubeCfg:   true,
		}
	}
	entries := make([]ContextEntry, 0, len(byContext))
	for _, e := range byContext {
		entries = append(entries, *e)
	}
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].DisplayName < entries[j].DisplayName
	})
	return entries
}

func buildContextEntries(c *config.Config) []ContextEntry {
	kubeCtxs, err := kubeconfig.ReadContexts()
	if err != nil {
		slog.Debug("could not read kubeconfig, continuing with global config only", "err", err)
		kubeCtxs = nil
	}
	return mergeContextSources(c.Kubernetes.Contexts, kubeCtxs)
}

// matchContexts returns entries whose display name or context short name
// contains the query (case-insensitive substring). An exact match on display
// name or full context short-circuits to a single result.
func matchContexts(entries []ContextEntry, query string) []ContextEntry {
	if query == "" {
		return nil
	}
	for _, e := range entries {
		if e.DisplayName == query || e.Context == query {
			return []ContextEntry{e}
		}
	}
	q := strings.ToLower(query)
	var matches []ContextEntry
	for _, e := range entries {
		if strings.Contains(strings.ToLower(e.DisplayName), q) {
			matches = append(matches, e)
			continue
		}
		if strings.Contains(strings.ToLower(kubeconfig.ShortName(e.Context)), q) {
			matches = append(matches, e)
		}
	}
	return matches
}

var contextUseCmd = &cobra.Command{
	Use:   "use [name|fuzzy-substring]",
	Short: "Switch kubectl context (TUI when no arg, fuzzy match when given)",
	Long: `Lists contexts from kest's global config (kubernetes.contexts) merged with
contexts from ~/.kube/config.

With no argument, opens an interactive picker. With an argument, matches
(case-insensitive substring) against the display name and the context's
short name. Unique match → switches immediately. Multiple matches → picker
pre-filtered to those entries.

This does NOT touch the active kest profile (see 'kest profile use' for
project-target selection).`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		entries := buildContextEntries(cfg)
		if len(entries) == 0 {
			return fmt.Errorf("no contexts found in kest global config or ~/.kube/config")
		}

		if len(args) == 1 {
			matches := matchContexts(entries, args[0])
			switch len(matches) {
			case 0:
				return fmt.Errorf("no contexts match %q", args[0])
			case 1:
				return switchContext(matches[0])
			default:
				entries = matches
			}
		}

		chosen, err := runContextPicker(entries)
		if err != nil {
			return err
		}
		if chosen.Context == "" {
			return nil // cancelled
		}
		return switchContext(chosen)
	},
}

var contextListCmd = &cobra.Command{
	Use:   "list",
	Short: "List contexts kest is aware of (union of kest config + kubeconfig)",
	RunE: func(cmd *cobra.Command, args []string) error {
		entries := buildContextEntries(cfg)
		if len(entries) == 0 {
			fmt.Println("(no contexts)")
			return nil
		}
		for _, e := range entries {
			src := sourceLabel(e)
			short := kubeconfig.ShortName(e.Context)
			if short == e.DisplayName {
				fmt.Printf("  %-30s [%s]\n", e.DisplayName, src)
			} else {
				fmt.Printf("  %-30s [%s]  (context: %s)\n", e.DisplayName, src, short)
			}
		}
		return nil
	},
}

func sourceLabel(e ContextEntry) string {
	switch {
	case e.InGlobalCfg && e.InKubeCfg:
		return "kest+kube"
	case e.InGlobalCfg:
		return "kest"
	default:
		return "kube"
	}
}

func switchContext(e ContextEntry) error {
	slog.Info("switching kube context", "context", e.Context, "display", e.DisplayName)
	if err := runner.Run("kubectl", "config", "use-context", e.Context); err != nil {
		return fmt.Errorf("kubectl config use-context %s: %w", e.Context, err)
	}
	return nil
}

// --- TUI ---

type contextItem struct {
	entry ContextEntry
}

func (i contextItem) FilterValue() string { return i.entry.DisplayName }

type contextItemDelegate struct{}

func (d contextItemDelegate) Height() int                             { return 1 }
func (d contextItemDelegate) Spacing() int                            { return 0 }
func (d contextItemDelegate) Update(_ tea.Msg, _ *list.Model) tea.Cmd { return nil }
func (d contextItemDelegate) Render(w io.Writer, m list.Model, index int, item list.Item) {
	i := item.(contextItem)
	cursor := "  "
	if index == m.Index() {
		cursor = "> "
	}
	line := fmt.Sprintf("%s%s  (%s)", cursor, i.entry.DisplayName, sourceLabel(i.entry))
	style := lipgloss.NewStyle()
	if index == m.Index() {
		style = style.Bold(true).Foreground(lipgloss.Color("12"))
	}
	fmt.Fprint(w, style.Render(line))
}

type contextPickerModel struct {
	list   list.Model
	choice ContextEntry
}

func (m contextPickerModel) Init() tea.Cmd { return nil }

func (m contextPickerModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "enter":
			if item, ok := m.list.SelectedItem().(contextItem); ok {
				m.choice = item.entry
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

func (m contextPickerModel) View() string { return "\n" + m.list.View() }

func runContextPicker(entries []ContextEntry) (ContextEntry, error) {
	items := make([]list.Item, len(entries))
	for i, e := range entries {
		items[i] = contextItem{entry: e}
	}
	l := list.New(items, contextItemDelegate{}, 60, min(len(items)+6, 20))
	l.Title = "Switch kubectl context"
	l.SetShowStatusBar(false)
	l.SetFilteringEnabled(false)
	l.Styles.Title = lipgloss.NewStyle().Bold(true).Padding(0, 1)

	p := tea.NewProgram(contextPickerModel{list: l})
	result, err := p.Run()
	if err != nil {
		return ContextEntry{}, err
	}
	return result.(contextPickerModel).choice, nil
}

func completeContextNames(_ *cobra.Command, _ []string, _ string) ([]string, cobra.ShellCompDirective) {
	if cfg == nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	entries := buildContextEntries(cfg)
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		names = append(names, e.DisplayName)
	}
	return names, cobra.ShellCompDirectiveNoFileComp
}

func init() {
	contextUseCmd.ValidArgsFunction = completeContextNames
	contextCmd.AddCommand(contextUseCmd)
	contextCmd.AddCommand(contextListCmd)
	rootCmd.AddCommand(contextCmd)
}
