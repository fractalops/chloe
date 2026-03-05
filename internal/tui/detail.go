package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/progress"
	"github.com/charmbracelet/bubbles/table"
	"github.com/charmbracelet/lipgloss"
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

	// Tasks (surfaced early so they're visible without scrolling)
	if len(s.Tasks) > 0 {
		b.WriteString("\n")
		b.WriteString(renderTasks(s.Tasks, width))
	}

	// Stats section
	if st := s.Stats; st != nil {
		b.WriteString("\n")
		b.WriteString(strings.Repeat("─", width) + "\n")

		valueWidth := width - 20
		if valueWidth < 10 {
			valueWidth = 10
		}

		rows := []table.Row{
			{"Model", st.Model},
			{"Turns", fmt.Sprintf("%d", st.TurnCount)},
			{"Duration", formatDuration(st.TotalDurationMs)},
			{"Input Tokens", formatTokens(st.InputTokens)},
			{"Output Tokens", formatTokens(st.OutputTokens)},
			{"Cache Read", formatTokens(st.CacheReadTokens)},
			{"Cache Write", formatTokens(st.CacheCreateTokens)},
		}

		columns := []table.Column{
			{Title: "Metric", Width: 14},
			{Title: "Value", Width: valueWidth},
		}

		t := table.New(
			table.WithColumns(columns),
			table.WithRows(rows),
			table.WithHeight(len(rows)),
		)
		ts := table.DefaultStyles()
		ts.Header = ts.Header.
			BorderStyle(lipgloss.NormalBorder()).
			BorderBottom(true).
			BorderForeground(lipgloss.Color("#444444")).
			Bold(true).
			Foreground(lipgloss.Color("#88AAFF"))
		ts.Cell = ts.Cell.Foreground(lipgloss.Color("#CCCCCC"))
		ts.Selected = ts.Cell // hide selection cursor
		t.SetStyles(ts)
		t.Blur()

		b.WriteString(t.View())
		b.WriteString("\n")

		// Cache hit rate progress bar
		totalInput := st.InputTokens + st.CacheReadTokens
		if totalInput > 0 {
			cacheRate := float64(st.CacheReadTokens) / float64(totalInput)
			p := progress.New(
				progress.WithSolidFill("#FF6600"),
				progress.WithoutPercentage(),
				progress.WithWidth(valueWidth),
			)
			b.WriteString(fmt.Sprintf("  %-14s %s %d%%\n", "Cache Hit:", p.ViewAs(cacheRate), int(cacheRate*100)))
		}
	}

	// Enrichment sections
	if s.Meta != nil {
		b.WriteString("\n")
		b.WriteString(renderSessionMeta(s.Meta, width))
	}
	if s.Facets != nil {
		b.WriteString("\n")
		b.WriteString(renderSessionFacets(s.Facets, width))
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

// renderSessionMeta renders code changes, tool usage, and language breakdown.
func renderSessionMeta(meta *claude.SessionMeta, width int) string {
	var b strings.Builder

	b.WriteString(strings.Repeat("─", width) + "\n")
	b.WriteString("  " + sectionTitleStyle.Render("Code Changes") + "\n")
	b.WriteString(fmt.Sprintf("  %-16s +%d\n", "Lines Added:", meta.LinesAdded))
	b.WriteString(fmt.Sprintf("  %-16s -%d\n", "Lines Removed:", meta.LinesRemoved))
	b.WriteString(fmt.Sprintf("  %-16s %d\n", "Files Modified:", meta.FilesModified))
	b.WriteString(fmt.Sprintf("  %-16s %d\n", "Git Commits:", meta.GitCommits))

	if len(meta.ToolCounts) > 0 {
		b.WriteString("\n  " + sectionTitleStyle.Render("Tool Usage") + "\n")
		lineLen := 0
		for tool, count := range meta.ToolCounts {
			entry := fmt.Sprintf("%s: %d", tool, count)
			if lineLen > 0 && lineLen+2+len(entry) > width-2 {
				b.WriteString("\n")
				lineLen = 0
			}
			b.WriteString("  " + entry)
			lineLen += 2 + len(entry)
		}
		b.WriteString("\n")
	}

	if len(meta.Languages) > 0 {
		b.WriteString("\n  " + sectionTitleStyle.Render("Languages") + "\n")
		lineLen := 0
		for lang, count := range meta.Languages {
			entry := fmt.Sprintf("%s: %d", lang, count)
			if lineLen > 0 && lineLen+2+len(entry) > width-2 {
				b.WriteString("\n")
				lineLen = 0
			}
			b.WriteString("  " + entry)
			lineLen += 2 + len(entry)
		}
		b.WriteString("\n")
	}

	return b.String()
}

// renderSessionFacets renders session quality metrics.
func renderSessionFacets(facets *claude.SessionFacets, width int) string {
	var b strings.Builder

	b.WriteString(strings.Repeat("─", width) + "\n")
	b.WriteString("  " + sectionTitleStyle.Render("Session Quality") + "\n")

	// Outcome with color
	var outcomeRendered string
	switch facets.Outcome {
	case "fully_achieved", "mostly_achieved":
		outcomeRendered = outcomeGoodStyle.Render(facets.Outcome)
	case "partially_achieved":
		outcomeRendered = outcomeMidStyle.Render(facets.Outcome)
	case "not_achieved":
		outcomeRendered = outcomeBadStyle.Render(facets.Outcome)
	default:
		outcomeRendered = detailValueStyle.Render(facets.Outcome)
	}
	b.WriteString(fmt.Sprintf("  %-16s %s\n", "Outcome:", outcomeRendered))

	if facets.ClaudeHelpfulness != "" {
		b.WriteString(fmt.Sprintf("  %-16s %s\n", "Helpfulness:", detailValueStyle.Render(facets.ClaudeHelpfulness)))
	}
	if facets.SessionType != "" {
		b.WriteString(fmt.Sprintf("  %-16s %s\n", "Type:", detailValueStyle.Render(facets.SessionType)))
	}
	if facets.BriefSummary != "" {
		summary := facets.BriefSummary
		maxLen := width - 18
		if runes := []rune(summary); maxLen > 0 && len(runes) > maxLen {
			summary = string(runes[:maxLen-1]) + "…"
		}
		b.WriteString(fmt.Sprintf("  %-16s %s\n", "Summary:", detailValueStyle.Render(summary)))
	}

	return b.String()
}

// renderTasks renders the task list for a session.
func renderTasks(tasks []claude.Task, width int) string {
	var b strings.Builder

	b.WriteString(strings.Repeat("─", width) + "\n")
	b.WriteString("  " + sectionTitleStyle.Render("Tasks") + "\n")

	for _, t := range tasks {
		var icon string
		var style lipgloss.Style
		switch t.Status {
		case "completed":
			icon = "✓"
			style = taskCompleteStyle
		case "in_progress":
			icon = "→"
			style = taskInProgressStyle
		default:
			icon = " "
			style = taskPendingStyle
		}
		subject := t.Subject
		maxLen := width - 8
		if runes := []rune(subject); maxLen > 0 && len(runes) > maxLen {
			subject = string(runes[:maxLen-1]) + "…"
		}
		b.WriteString("  " + style.Render(fmt.Sprintf("[%s] %s", icon, subject)) + "\n")
	}

	return b.String()
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
