package tui

import (
	"fmt"
	"io"
	"strings"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/fractalops/chloe/internal/claude"
)

// Session status values.
const (
	statusActive    = "active"
	statusSuspended = "suspended"
	statusInactive  = "inactive"
)

// Layout column widths for session rows.
const (
	colStatusWidth  = 5
	colProjectWidth = 22
	colTokenWidth   = 6
	colOutcomeWidth = 2
	colAgeWidth     = 4
	colPadding      = 14 // " " + 4× " │ " + " " = 1+12+1
)

// wideDescWidth calculates the description column width for a given total width.
func wideDescWidth(totalWidth int) int {
	w := totalWidth - colStatusWidth - colProjectWidth - colTokenWidth - colOutcomeWidth - colAgeWidth - colPadding
	if w < 10 {
		w = 10
	}
	return w
}

// wideRow assembles a wide-mode row from pre-padded column strings.
// Each column string must already be padded/truncated to its column width.
func wideRow(status, project, desc, tokens, outcome, age string) string {
	return fmt.Sprintf(" %s │ %s │ %s │ %s %s │ %s", status, project, desc, tokens, outcome, age)
}

type groupMode int

const (
	groupAll groupMode = iota
	groupActiveOnly
)

func (g groupMode) String() string {
	switch g {
	case groupActiveOnly:
		return "active-only"
	default:
		return "all"
	}
}

func (g groupMode) next() groupMode {
	if g == groupAll {
		return groupActiveOnly
	}
	return groupAll
}

// sessionItem wraps a claude.Session for the list.Model.
type sessionItem struct {
	session claude.Session
}

func (i sessionItem) Title() string {
	return i.session.FirstMsg
}

func (i sessionItem) Description() string {
	return claude.ShortenPath(i.session.Project)
}

func (i sessionItem) FilterValue() string {
	return i.session.Project + " " + i.session.UserMsgs + " " + i.session.ID + " " + sessionDisplayMsg(i.session)
}

// sessionDisplayMsg returns the best available description for a session.
// Prefers Facets.BriefSummary, then falls back to FirstMsg.
func sessionDisplayMsg(s claude.Session) string {
	if s.Facets != nil && s.Facets.BriefSummary != "" {
		return s.Facets.BriefSummary
	}
	return s.FirstMsg
}

// sessionDelegate renders session rows in the list.
type sessionDelegate struct {
	compact bool
}

func (d sessionDelegate) Height() int {
	if d.compact {
		return 2
	}
	return 1
}
func (d sessionDelegate) Spacing() int { return 0 }
func (d sessionDelegate) Update(_ tea.Msg, _ *list.Model) tea.Cmd {
	return nil
}

func (d sessionDelegate) Render(w io.Writer, m list.Model, index int, item list.Item) {
	si, ok := item.(sessionItem)
	if !ok {
		return
	}

	s := si.session
	selected := index == m.Index()
	width := m.Width()

	if d.compact {
		d.renderCompact(w, s, selected, width)
	} else {
		d.renderWide(w, s, selected, width)
	}
}

func (d sessionDelegate) renderWide(w io.Writer, s claude.Session, selected bool, width int) {
	// Status indicator
	var status string
	switch s.Status {
	case statusActive:
		status = activeStatusStyle.Render("● act")
	case statusSuspended:
		status = suspendedStatusStyle.Render("◐ sus")
	default:
		status = inactiveStatusStyle.Render("○ off")
	}

	// Project (shortened)
	proj := claude.ShortenPath(s.Project)
	if runes := []rune(proj); len(runes) > colProjectWidth {
		proj = string(runes[:colProjectWidth-1]) + "…"
	}
	projStr := projectStyle.Render(fmt.Sprintf("%-*s", colProjectWidth, proj))

	// Description (prefer enrichment summary over first message)
	msg := sessionDisplayMsg(s)
	descW := wideDescWidth(width)
	if runes := []rune(msg); len(runes) > descW {
		msg = string(runes[:descW-1]) + "…"
	}
	msgStr := messageStyle.Render(fmt.Sprintf("%-*s", descW, msg))

	// Tokens
	var tokStr string
	if s.Meta != nil {
		tokStr = tokenColumnStyle.Render(fmt.Sprintf("%*s", colTokenWidth, formatTokens(s.Meta.TotalTokens())))
	} else {
		tokStr = ageStyle.Render(fmt.Sprintf("%*s", colTokenWidth, "—"))
	}

	// Outcome
	outcomeStr := outcomeSymbol(s)

	// Age
	age := claude.RelativeTime(s.LastActive)
	ageStr := ageStyle.Render(fmt.Sprintf("%*s", colAgeWidth, age))

	row := wideRow(status, projStr, msgStr, tokStr, outcomeStr, ageStr)

	if selected {
		row = selectedStyle.Width(width).Render(row)
	}

	fmt.Fprint(w, row) //nolint:errcheck // list delegate writer
}

func (d sessionDelegate) renderCompact(w io.Writer, s claude.Session, selected bool, width int) {
	// Card-style: no │ separators, space-aligned
	// Line 1: " ● act  ~/me/tmp/chloe                  6h"
	// Line 2: "        Implement the following…   2.5K ✓"

	var status string
	switch s.Status {
	case statusActive:
		status = activeStatusStyle.Render("● act")
	case statusSuspended:
		status = suspendedStatusStyle.Render("◐ sus")
	default:
		status = inactiveStatusStyle.Render("○ off")
	}

	const indent = 7 // len("● act") + 2 spaces

	// Line 1: status  project ... age (right-aligned)
	age := claude.RelativeTime(s.LastActive)
	ageStr := ageStyle.Render(age)
	ageW := lipgloss.Width(ageStr)

	projMax := width - indent - ageW - 3 // 1 leading space + 2 gap before age
	if projMax < 8 {
		projMax = 8
	}
	proj := claude.ShortenPath(s.Project)
	if runes := []rune(proj); len(runes) > projMax {
		proj = string(runes[:projMax-1]) + "…"
	}
	projStr := projectStyle.Render(proj)
	projW := lipgloss.Width(projStr)

	gap1 := width - 1 - colStatusWidth - 1 - projW - ageW - 1
	if gap1 < 1 {
		gap1 = 1
	}
	line1 := fmt.Sprintf(" %s  %s%s%s", status, projStr, strings.Repeat(" ", gap1), ageStr)

	// Line 2: indent + message ... tokens outcome (right-aligned)
	var tokStr string
	if s.Meta != nil {
		tokStr = tokenColumnStyle.Render(formatTokens(s.Meta.TotalTokens()))
	} else {
		tokStr = ageStyle.Render("—")
	}
	outcomeStr := outcomeSymbol(s)
	rightSide := tokStr + " " + outcomeStr
	rightW := lipgloss.Width(rightSide)

	msgMax := width - indent - rightW - 3 // 1 leading space + 2 gap
	if msgMax < 5 {
		msgMax = 5
	}
	msg := sessionDisplayMsg(s)
	if runes := []rune(msg); len(runes) > msgMax {
		msg = string(runes[:msgMax-1]) + "…"
	}
	msgStr := messageStyle.Render(msg)
	msgW := lipgloss.Width(msgStr)

	gap2 := width - 1 - indent - msgW - rightW - 1
	if gap2 < 1 {
		gap2 = 1
	}
	line2 := fmt.Sprintf(" %s%s%s%s", strings.Repeat(" ", indent), msgStr, strings.Repeat(" ", gap2), rightSide)

	row := line1 + "\n" + line2

	if selected {
		row = selectedStyle.Width(width).Render(row)
	}

	fmt.Fprint(w, row) //nolint:errcheck // list delegate writer
}

// filterSessions returns sessions matching the group filter.
func filterSessions(sessions []claude.Session, group groupMode) []claude.Session {
	if group == groupAll {
		return sessions
	}

	var result []claude.Session
	for _, s := range sessions {
		if group == groupActiveOnly && s.Status == statusInactive {
			continue
		}
		result = append(result, s)
	}
	return result
}

// sessionsToItems converts sessions to list items.
func sessionsToItems(sessions []claude.Session) []list.Item {
	items := make([]list.Item, len(sessions))
	for i, s := range sessions {
		items[i] = sessionItem{session: s}
	}
	return items
}

// statusHeader builds the status count line.
func statusHeader(sessions []claude.Session, width int, sc *claude.StatsCache) string {
	activeCount := 0
	suspendedCount := 0
	for _, s := range sessions {
		switch s.Status {
		case statusActive:
			activeCount++
		case statusSuspended:
			suspendedCount++
		}
	}

	title := titleStyle.Render("chloe")
	var countParts []string
	if activeCount > 0 {
		countParts = append(countParts, activeCountStyle.Render(fmt.Sprintf("● %d active", activeCount)))
	}
	if suspendedCount > 0 {
		countParts = append(countParts, suspendedStatusStyle.Render(fmt.Sprintf("◐ %d suspended", suspendedCount)))
	}

	if today := sc.TodayStats(); today != nil {
		todayStr := inactiveStatusStyle.Render(fmt.Sprintf("│ Today: %d msgs  %d sessions  %d tools", today.MessageCount, today.SessionCount, today.ToolCallCount))
		countParts = append(countParts, todayStr)
	}

	countStr := strings.Join(countParts, "  ")
	if countStr == "" {
		countStr = inactiveStatusStyle.Render("no active sessions")
	}

	gap := width - lipgloss.Width(title) - lipgloss.Width(countStr) - 2
	if gap < 1 {
		gap = 1
	}
	return title + strings.Repeat(" ", gap) + countStr
}

// outcomeSymbol returns a colored badge for the session outcome.
func outcomeSymbol(s claude.Session) string {
	if s.Facets == nil {
		return ageStyle.Render(fmt.Sprintf("%-*s", colOutcomeWidth, "—"))
	}
	switch s.Facets.Outcome {
	case "fully_achieved", "mostly_achieved":
		return outcomeGoodStyle.Render(fmt.Sprintf("%-*s", colOutcomeWidth, "✓"))
	case "partially_achieved":
		return outcomeMidStyle.Render(fmt.Sprintf("%-*s", colOutcomeWidth, "~"))
	case "not_achieved":
		return outcomeBadStyle.Render(fmt.Sprintf("%-*s", colOutcomeWidth, "✗"))
	default:
		return ageStyle.Render(fmt.Sprintf("%-*s", colOutcomeWidth, "—"))
	}
}
