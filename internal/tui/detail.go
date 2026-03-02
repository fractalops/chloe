package tui

import (
	"fmt"
	"strings"

	"github.com/fractalops/chloe/internal/claude"
	"github.com/fractalops/chloe/internal/util"
)

// renderDetailContent builds a single string with metadata, stats, and
// conversation bubbles, suitable for viewport.SetContent().
func renderDetailContent(s claude.Session, msgs []claude.ConversationMessage, width int) string {
	var b strings.Builder

	// Metadata
	fields := []struct {
		label string
		value string
	}{
		{"ID", s.ID},
		{"Project", claude.ShortenPath(s.Project)},
		{"CWD", s.CWD},
		{"Version", s.Version},
		{"Git Branch", s.GitBranch},
		{"Slug", s.Slug},
		{"Status", s.Status},
		{"Started", s.StartedAt.Format("2006-01-02 15:04:05")},
		{"Last Active", util.RelativeTime(s.LastActive)},
		{"Messages", fmt.Sprintf("%d lines", s.MessageCount)},
	}
	if s.PID > 0 {
		fields = append(fields, struct {
			label string
			value string
		}{"PID", fmt.Sprintf("%d", s.PID)})
	}

	for _, f := range fields {
		label := detailLabelStyle.Render(f.label + ":")
		value := detailValueStyle.Render(f.value)
		b.WriteString(fmt.Sprintf("  %s %s\n", label, value))
	}

	// Stats section
	if st := s.Stats; st != nil {
		b.WriteString("\n")
		b.WriteString(strings.Repeat("─", width) + "\n")

		costStr := fmt.Sprintf("$%.2f", st.CostUSD)
		if st.CostUSD >= 1.0 {
			costStr = costStyle.Render(costStr)
		} else {
			costStr = detailValueStyle.Render(costStr)
		}
		modelStr := ""
		if st.Model != "" {
			modelStr = detailValueStyle.Render(st.Model)
		}
		b.WriteString(fmt.Sprintf("  %s %s    %s %s\n",
			detailLabelStyle.Render("Cost:"), costStr,
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

	// Conversation bubbles
	for _, msg := range msgs {
		b.WriteString(renderChatBubble(msg.Role, msg.Content, width))
	}

	return b.String()
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
