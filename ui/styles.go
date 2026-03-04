package ui

import (
	"github.com/charmbracelet/lipgloss"
)

// Colors
var (
	Primary   = lipgloss.Color("#7C3AED") // Purple
	Secondary = lipgloss.Color("#06B6D4") // Cyan
	Success   = lipgloss.Color("#10B981") // Green
	Warning   = lipgloss.Color("#F59E0B") // Amber
	Error     = lipgloss.Color("#EF4444") // Red
	Muted     = lipgloss.Color("#6B7280") // Gray
)

// Text styles
var (
	TitleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(Primary)

	SubtitleStyle = lipgloss.NewStyle().
			Foreground(Secondary).
			Italic(true)

	MutedStyle = lipgloss.NewStyle().
			Foreground(Muted)

	SuccessStyle = lipgloss.NewStyle().
			Foreground(Success)

	WarningStyle = lipgloss.NewStyle().
			Foreground(Warning)

	ErrorStyle = lipgloss.NewStyle().
			Foreground(Error)
)

// Component styles
var (
	// Box for containing content
	BoxStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(Primary).
			Padding(1, 2)

	// Progress bar styles
	ProgressBarComplete = lipgloss.NewStyle().
				Foreground(Success)

	ProgressBarIncomplete = lipgloss.NewStyle().
				Foreground(Muted)

	// Track item styles
	TrackNumberStyle = lipgloss.NewStyle().
				Foreground(Muted).
				Width(6)

	TrackTitleStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#F9FAFB")).
			Bold(true)

	// Spinner
	SpinnerStyle = lipgloss.NewStyle().
			Foreground(Primary)

	// Help text
	HelpStyle = lipgloss.NewStyle().
			Foreground(Muted).
			MarginTop(1)
)

// Icons/Symbols
const (
	IconCheck    = "✓"
	IconCross    = "✗"
	IconSkip     = "→"
	IconMusic    = "♫"
	IconFolder   = "📁"
	IconDot      = "•"
	IconProgress = "█"
	IconEmpty    = "░"
	IconWarning  = "⚠"
)

// RenderProgressBar renders a progress bar with the given percentage and width.
func RenderProgressBar(percent float64, width int) string {
	filled := int(percent / 100 * float64(width))
	if filled > width {
		filled = width
	}
	if filled < 0 {
		filled = 0
	}

	bar := ""
	for i := 0; i < width; i++ {
		if i < filled {
			bar += ProgressBarComplete.Render(IconProgress)
		} else {
			bar += ProgressBarIncomplete.Render(IconEmpty)
		}
	}
	return bar
}
