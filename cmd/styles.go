package cmd

import "github.com/charmbracelet/lipgloss"

// Shared color palette used across TUI and CLI output.
var (
	colorAccent  = lipgloss.Color("12") // blue — titles, selections, accents
	colorSuccess = lipgloss.Color("10") // green — pass, checkmarks
	colorDim     = lipgloss.Color("8")  // gray — secondary text, details
	colorHelp    = lipgloss.Color("7")  // white — help text
	colorKey     = lipgloss.Color("14") // teal — key labels in previews
	colorDanger  = lipgloss.Color("9")  // red — errors, failures
	colorWarn    = lipgloss.Color("11") // yellow — warnings
)
