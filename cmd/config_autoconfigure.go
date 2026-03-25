package cmd

import (
	"fmt"
	"io"
	"strings"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/example/kestrel/internal/awsconfig"
	"github.com/example/kestrel/internal/config"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

var configAutoconfigureCmd = &cobra.Command{
	Use:   "autoconfigure",
	Short: "Auto-discover AWS profiles and map them to kest environments",
	Long: `Reads AWS profiles from ~/.aws/config and walks you through
assigning each environment (dev, stage, prod) an AWS profile.
The result is written to the global kest config.`,
	RunE: runAutoconfigure,
}

func init() {
	configCmd.AddCommand(configAutoconfigureCmd)
}

var standardEnvs = []string{"dev", "stage", "prod"}

func runAutoconfigure(cmd *cobra.Command, args []string) error {
	profiles, err := awsconfig.ReadProfiles()
	if err != nil {
		return fmt.Errorf("reading AWS profiles: %w", err)
	}
	if len(profiles) == 0 {
		return fmt.Errorf("no profiles found in ~/.aws/config")
	}

	fmt.Printf("Found %d AWS profile(s): %s\n\n", len(profiles), strings.Join(profiles, ", "))

	// Load existing global config to preserve non-environment fields
	global, err := config.LoadGlobal()
	if err != nil {
		global = &config.Config{}
	}
	if global.Environments == nil {
		global.Environments = make(map[string]config.EnvConfig)
	}

	// For each standard environment, let user pick a profile
	skipItem := "(skip)"
	for _, env := range standardEnvs {
		items := make([]list.Item, 0, len(profiles)+1)
		items = append(items, profileItem{name: skipItem})

		bestIdx := 0 // default to skip
		for i, p := range profiles {
			items = append(items, profileItem{name: p})
			if awsconfig.InferEnv(p) == env {
				bestIdx = i + 1 // +1 because skip is first
			}
		}

		// If env already configured, try to pre-select that profile
		if existing, ok := global.Environments[env]; ok && existing.AwsProfile != "" {
			for i, p := range profiles {
				if p == existing.AwsProfile {
					bestIdx = i + 1
					break
				}
			}
		}

		delegate := profileItemDelegate{}
		title := fmt.Sprintf("AWS profile for [%s]", env)
		l := list.New(items, delegate, 50, min(len(items)+6, 20))
		l.Title = title
		l.SetShowStatusBar(false)
		l.SetFilteringEnabled(false)
		l.Select(bestIdx)
		l.Styles.Title = lipgloss.NewStyle().Bold(true).Padding(0, 1)

		m := profilePickerModel{list: l}
		p := tea.NewProgram(m)
		result, err := p.Run()
		if err != nil {
			return err
		}

		pm := result.(profilePickerModel)
		if pm.cancelled {
			fmt.Println("Cancelled.")
			return nil
		}

		if pm.choice != "" && pm.choice != skipItem {
			ec := global.Environments[env]
			ec.AwsProfile = pm.choice
			global.Environments[env] = ec
			fmt.Printf("  %s → %s\n", env, pm.choice)
		} else {
			fmt.Printf("  %s → (skipped)\n", env)
		}
	}

	// Show preview
	fmt.Println("\nProposed global config:")
	fmt.Println(strings.Repeat("─", 40))
	out, _ := yaml.Marshal(global)
	fmt.Print(string(out))
	fmt.Println(strings.Repeat("─", 40))

	// Confirm
	confirmItems := []list.Item{
		profileItem{name: "Yes, write config"},
		profileItem{name: "No, cancel"},
	}
	delegate := profileItemDelegate{}
	l := list.New(confirmItems, delegate, 40, 8)
	l.Title = "Write to " + config.GlobalConfigPath() + "?"
	l.SetShowStatusBar(false)
	l.SetFilteringEnabled(false)
	l.Styles.Title = lipgloss.NewStyle().Bold(true).Padding(0, 1)

	m := profilePickerModel{list: l}
	p := tea.NewProgram(m)
	result, err := p.Run()
	if err != nil {
		return err
	}

	pm := result.(profilePickerModel)
	if pm.choice != "Yes, write config" {
		fmt.Println("Cancelled.")
		return nil
	}

	if err := config.WriteGlobal(global); err != nil {
		return err
	}
	fmt.Printf("\nConfig written to %s\n", config.GlobalConfigPath())
	return nil
}

// --- bubbletea model for profile picking ---

type profileItem struct {
	name string
}

func (i profileItem) FilterValue() string { return i.name }

type profileItemDelegate struct{}

func (d profileItemDelegate) Height() int                             { return 1 }
func (d profileItemDelegate) Spacing() int                            { return 0 }
func (d profileItemDelegate) Update(_ tea.Msg, _ *list.Model) tea.Cmd { return nil }
func (d profileItemDelegate) Render(w io.Writer, m list.Model, index int, item list.Item) {
	i := item.(profileItem)
	cursor := "  "
	if index == m.Index() {
		cursor = "> "
	}

	line := fmt.Sprintf("%s%s", cursor, i.name)
	style := lipgloss.NewStyle()
	if index == m.Index() {
		style = style.Bold(true).Foreground(lipgloss.Color("12"))
	}
	fmt.Fprint(w, style.Render(line))
}

type profilePickerModel struct {
	list      list.Model
	choice    string
	cancelled bool
}

func (m profilePickerModel) Init() tea.Cmd { return nil }

func (m profilePickerModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "enter":
			if item, ok := m.list.SelectedItem().(profileItem); ok {
				m.choice = item.name
			}
			return m, tea.Quit
		case "q", "esc", "ctrl+c":
			m.cancelled = true
			return m, tea.Quit
		}
	case tea.WindowSizeMsg:
		m.list.SetWidth(msg.Width)
	}

	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	return m, cmd
}

func (m profilePickerModel) View() string {
	return "\n" + m.list.View()
}
