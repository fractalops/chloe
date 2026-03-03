package tui

import (
	"fmt"
	"strings"

	"github.com/fractalops/chloe/internal/claude"
)

// BubbleRegion maps a line range in the rendered detail content to a message index.
type BubbleRegion struct {
	StartLine int // inclusive
	EndLine   int // exclusive
	MsgIndex  int // index into the conversation messages slice
}

// renderDetailContent builds a single string with metadata, stats, and
// conversation bubbles, suitable for viewport.SetContent().
// It returns the rendered string and a slice of BubbleRegion for navigation.
func renderDetailContent(s claude.Session, msgs []claude.ConversationMessage, width int, selectedBubble int, burnRate float64) (string, []BubbleRegion) {
	var b strings.Builder
	var regions []BubbleRegion

	type field struct {
		label string
		value string
	}

	// Metadata
	fields := []field{
		{"ID", s.ID},
		{"Project", claude.ShortenPath(s.Project)},
		{"CWD", s.CWD},
		{"Version", s.Version},
		{"Git Branch", s.GitBranch},
		{"Slug", s.Slug},
		{"Status", s.Status},
		{"Started", s.StartedAt.Format("2006-01-02 15:04:05")},
		{"Last Active", claude.RelativeTime(s.LastActive)},
		{"Messages", fmt.Sprintf("%d lines", s.MessageCount)},
	}
	if s.PID > 0 {
		fields = append(fields,
			field{"PID", fmt.Sprintf("%d", s.PID)},
			field{"CPU", fmt.Sprintf("%.1f%%", s.CPUPct)},
			field{"Memory", fmt.Sprintf("%.1f%% (%d MB)", s.MemPct, s.RSSKB/1024)},
			field{"Open Files", fmt.Sprintf("%d", s.OpenFiles)},
		)
		if burnRate > 0 {
			fields = append(fields, field{"Burn Rate", fmt.Sprintf("%.0f tok/min", burnRate)})
		}
	}

	for _, f := range fields {
		label := detailLabelStyle.Render(f.label + ":")
		var value string
		if f.label == "Burn Rate" {
			value = burnRateStyle.Render(f.value)
		} else {
			value = detailValueStyle.Render(f.value)
		}
		b.WriteString(fmt.Sprintf("  %s %s\n", label, value))
	}

	// Stats section
	if st := s.Stats; st != nil {
		b.WriteString("\n")
		b.WriteString(strings.Repeat("─", width) + "\n")

		modelStr := ""
		if st.Model != "" {
			modelStr = detailValueStyle.Render(st.Model)
		}
		b.WriteString(fmt.Sprintf("  %s %s\n",
			detailLabelStyle.Render("Model:"), modelStr))

		durationStr := formatDuration(st.TotalDurationMs)
		b.WriteString(fmt.Sprintf("  %s %s    %s %s\n",
			detailLabelStyle.Render("Turns:"), detailValueStyle.Render(fmt.Sprintf("%d", st.TurnCount)),
			detailLabelStyle.Render("Duration:"), detailValueStyle.Render(durationStr)))

		b.WriteString(fmt.Sprintf("  %s %s in / %s out / %s cache read / %s cache write\n",
			detailLabelStyle.Render("Tokens:"),
			detailValueStyle.Render(formatTokens(st.InputTokens)),
			detailValueStyle.Render(formatTokens(st.OutputTokens)),
			detailValueStyle.Render(formatTokens(st.CacheReadTokens)),
			detailValueStyle.Render(formatTokens(st.CacheCreateTokens))))
	}

	b.WriteString("\n")
	b.WriteString(strings.Repeat("─", width) + "\n\n")

	// Track line count for bubble regions
	currentLine := strings.Count(b.String(), "\n")

	// Conversation bubbles
	for i, msg := range msgs {
		selected := i == selectedBubble
		startLine := currentLine
		bubbleStr := renderChatBubble(msg.Role, msg.Content, width, selected)
		b.WriteString(bubbleStr)
		currentLine += strings.Count(bubbleStr, "\n")
		regions = append(regions, BubbleRegion{
			StartLine: startLine,
			EndLine:   currentLine,
			MsgIndex:  i,
		})
	}

	return b.String(), regions
}

// formatTokens formats a token count with K/M suffixes.
func formatTokens(n int64) string {
	if n >= 1_000_000 {
		return fmt.Sprintf("%.1fM", float64(n)/1_000_000)
	}
	if n >= 1_000 {
		return fmt.Sprintf("%.1fK", float64(n)/1_000)
	}
	return fmt.Sprintf("%d", n)
}

// formatDuration formats milliseconds as a human-readable duration.
func formatDuration(ms int64) string {
	if ms == 0 {
		return "—"
	}
	secs := ms / 1000
	if secs < 60 {
		return fmt.Sprintf("%ds", secs)
	}
	mins := secs / 60
	remainSecs := secs % 60
	if mins < 60 {
		return fmt.Sprintf("%dm %ds", mins, remainSecs)
	}
	hours := mins / 60
	remainMins := mins % 60
	return fmt.Sprintf("%dh %dm", hours, remainMins)
}
