package tui

import "github.com/charmbracelet/lipgloss"

var (
	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#FF6600")).
			Padding(0, 1)

	activeCountStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#00FF00")).
				Bold(true)

	selectedStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#FFFFFF")).
			Background(lipgloss.Color("#333366"))

	normalStyle = lipgloss.NewStyle()

	activeStatusStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#00FF00")).
				Bold(true)

	suspendedStatusStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#FFAA00")).
				Bold(true)

	inactiveStatusStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#666666"))

	projectStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#88AAFF"))

	messageStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#CCCCCC"))

	ageStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#888888"))

	footerStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#666666")).
			Padding(0, 1)

	detailLabelStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color("#88AAFF")).
				Width(14)

	detailValueStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#CCCCCC"))

	costStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FF4444")).
			Bold(true)

	focusedPaneStyle = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(lipgloss.Color("#FF6600"))

	blurredPaneStyle = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(lipgloss.Color("#444444"))
)
