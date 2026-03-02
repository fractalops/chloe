package tui

import (
	"strings"

	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/lipgloss"
)

var (
	userBubbleStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("#00AAFF")).
			Padding(0, 1).
			Foreground(lipgloss.Color("#FFFFFF"))

	assistantBubbleStyle = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(lipgloss.Color("#FF6600")).
				Padding(0, 1).
				Foreground(lipgloss.Color("#CCCCCC"))

	userLabelStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#00AAFF"))

	assistantLabelStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color("#FF6600"))
)

var (
	glamourCache      *glamour.TermRenderer
	glamourCacheWidth int
)

// getGlamourRenderer returns a cached glamour renderer, recreating only when width changes.
func getGlamourRenderer(width int) *glamour.TermRenderer {
	if glamourCache != nil && glamourCacheWidth == width {
		return glamourCache
	}
	r, err := glamour.NewTermRenderer(
		glamour.WithAutoStyle(),
		glamour.WithWordWrap(width),
	)
	if err != nil {
		return nil
	}
	glamourCache = r
	glamourCacheWidth = width
	return r
}

// renderGlamourMarkdown renders markdown content using glamour.
func renderGlamourMarkdown(content string, width int) string {
	r := getGlamourRenderer(width)
	if r == nil {
		return content
	}
	out, err := r.Render(content)
	if err != nil {
		return content
	}
	return strings.TrimRight(out, "\n")
}

// renderChatBubble renders a single message as an aligned chat bubble.
// User messages align left, assistant messages align right.
func renderChatBubble(role, content string, totalWidth int) string {
	bubbleMaxWidth := totalWidth * 3 / 4
	if bubbleMaxWidth < 20 {
		bubbleMaxWidth = 20
	}
	if bubbleMaxWidth > totalWidth-4 {
		bubbleMaxWidth = totalWidth - 4
	}

	contentWidth := bubbleMaxWidth - 4
	if contentWidth < 10 {
		contentWidth = 10
	}

	// Render markdown via glamour
	rendered := renderGlamourMarkdown(content, contentWidth)

	// Truncate very long messages
	lines := strings.Split(rendered, "\n")
	maxLines := 40
	if len(lines) > maxLines {
		lines = lines[:maxLines]
		lines = append(lines, "… (truncated)")
	}
	bubbleContent := strings.Join(lines, "\n")

	// Measure the widest line to fit the bubble to content
	maxLineWidth := 0
	for _, l := range lines {
		w := lipgloss.Width(l)
		if w > maxLineWidth {
			maxLineWidth = w
		}
	}
	// bubble border(2) + padding(2) = 4 extra chars
	fitWidth := maxLineWidth + 4
	if fitWidth > bubbleMaxWidth {
		fitWidth = bubbleMaxWidth
	}
	if fitWidth < 10 {
		fitWidth = 10
	}

	var result strings.Builder

	if role == "user" {
		label := userLabelStyle.Render("▶ You")
		bubble := userBubbleStyle.Width(fitWidth - 2).Render(bubbleContent)

		result.WriteString("  " + label + "\n")
		for _, l := range strings.Split(bubble, "\n") {
			result.WriteString("  " + l + "\n")
		}
	} else {
		label := assistantLabelStyle.Render("Claude ◀")
		bubble := assistantBubbleStyle.Width(fitWidth - 2).Render(bubbleContent)

		padding := totalWidth - fitWidth - 2
		if padding < 0 {
			padding = 0
		}
		pad := strings.Repeat(" ", padding)

		result.WriteString(pad + label + "\n")
		for _, l := range strings.Split(bubble, "\n") {
			result.WriteString(pad + l + "\n")
		}
	}

	result.WriteString("\n") // spacing between bubbles
	return result.String()
}
