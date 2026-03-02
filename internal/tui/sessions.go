package tui

import (
	"fmt"
	"io"
	"strings"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/fractalops/chloe/internal/claude"
	"github.com/fractalops/chloe/internal/util"
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
	colAgeWidth     = 4
	colPadding      = 12 // separators + margins
)

type groupMode int

const (
	groupAll groupMode = iota
	groupByProject
	groupActiveOnly
)

func (g groupMode) String() string {
	switch g {
	case groupByProject:
		return "by-project"
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
	return i.session.Project + " " + i.session.FirstMsg + " " + i.session.ID
}

// sessionDelegate renders session rows in the list.
type sessionDelegate struct{}

func (d sessionDelegate) Height() int  { return 1 }
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

	// First message
	msg := s.FirstMsg
	msgWidth := width - colStatusWidth - colProjectWidth - colAgeWidth - colPadding
	if msgWidth < 10 {
		msgWidth = 10
	}
	if runes := []rune(msg); len(runes) > msgWidth {
		msg = string(runes[:msgWidth-1]) + "…"
	}
	msgStr := messageStyle.Render(fmt.Sprintf("%-*s", msgWidth, msg))

	// Age
	age := util.RelativeTime(s.LastActive)
	ageStr := ageStyle.Render(fmt.Sprintf("%*s", colAgeWidth, age))

	row := fmt.Sprintf(" %s │ %s │ %s │ %s", status, projStr, msgStr, ageStr)

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
func statusHeader(sessions []claude.Session, width int) string {
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
