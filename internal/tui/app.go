package tui

import (
	"fmt"
	"sort"
	"time"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/fractalops/chloe/internal/claude"
)

const autoRefreshInterval = 5 * time.Second

const (
	paneList = iota
	paneDetail
)

// Model is the root Bubble Tea model.
type Model struct {
	list      list.Model
	viewport  viewport.Model
	focusPane int

	sessions      []claude.Session
	group         groupMode
	detailSession *claude.Session
	detailMsgs    []claude.ConversationMessage
	detailContent string

	width, height int
	ready         bool
}

// sessionLoadedMsg is sent when sessions are loaded.
type sessionLoadedMsg struct {
	sessions []claude.Session
}

// detailLoadedMsg is sent when a session's detail has been loaded.
type detailLoadedMsg struct {
	session claude.Session
	msgs    []claude.ConversationMessage
}

// tickMsg triggers auto-refresh.
type tickMsg time.Time

// NewModel creates a new TUI model.
func NewModel() Model {
	delegate := sessionDelegate{}
	l := list.New(nil, delegate, 0, 0)
	l.Title = "Sessions"
	l.SetShowHelp(false)
	l.SetShowStatusBar(false)
	l.SetShowTitle(false)
	l.SetFilteringEnabled(true)
	l.DisableQuitKeybindings()

	vp := viewport.New(0, 0)

	return Model{
		list:      l,
		viewport:  vp,
		focusPane: paneList,
		group:     groupAll,
	}
}

// Init loads sessions on startup and starts the auto-refresh ticker.
func (m Model) Init() tea.Cmd {
	return tea.Batch(loadSessions, tickCmd())
}

func tickCmd() tea.Cmd {
	return tea.Tick(autoRefreshInterval, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

func loadSessions() tea.Msg {
	sessions, err := claude.LoadAllSessions()
	if err != nil {
		return sessionLoadedMsg{sessions: nil}
	}

	pidMap := claude.DetectActiveProcesses()
	claude.ApplyPIDMappings(sessions, pidMap)

	sort.Slice(sessions, func(i, j int) bool {
		if sessions[i].Status != sessions[j].Status {
			return statusOrder(sessions[i].Status) < statusOrder(sessions[j].Status)
		}
		return sessions[i].LastActive.After(sessions[j].LastActive)
	})

	return sessionLoadedMsg{sessions: sessions}
}

func loadDetailCmd(s claude.Session) tea.Cmd {
	return func() tea.Msg {
		s.Stats = claude.LoadSessionStats(s.FilePath)
		msgs, _ := claude.LoadConversation(s.FilePath, 50)
		return detailLoadedMsg{session: s, msgs: msgs}
	}
}

// Update handles messages.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.updateLayout()
		m.ready = true
		return m, nil

	case tickMsg:
		// Don't refresh while filtering — SetItems resets filter state
		if m.list.FilterState() != list.Unfiltered {
			return m, tickCmd()
		}
		return m, tea.Batch(loadSessions, tickCmd())

	case sessionLoadedMsg:
		if msg.sessions == nil {
			// Load failed (e.g. no ~/.claude/projects) — don't retry, wait for next tick
			return m, nil
		}
		m.sessions = msg.sessions
		// Don't rebuild list while filtering or viewing filter results
		if m.list.FilterState() == list.Unfiltered {
			m.rebuildList()
		}
		return m, nil

	case detailLoadedMsg:
		m.detailSession = &msg.session
		m.detailMsgs = msg.msgs
		vpWidth := m.detailPaneWidth() - 2 // account for border
		m.detailContent = renderDetailContent(msg.session, msg.msgs, vpWidth)
		m.viewport.SetContent(m.detailContent)
		m.viewport.GotoTop()
		return m, nil

	case tea.KeyMsg:
		// If list is filtering, let it handle keys
		if m.list.FilterState() == list.Filtering {
			var cmd tea.Cmd
			m.list, cmd = m.list.Update(msg)
			return m, cmd
		}

		return m.handleKey(msg)
	}

	// Forward all other messages to the list (FilterMatchesMsg, spinner, etc.)
	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	return m, cmd
}

func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, keys.Quit):
		return m, tea.Quit

	case key.Matches(msg, keys.Tab):
		if m.focusPane == paneList {
			m.focusPane = paneDetail
		} else {
			m.focusPane = paneList
		}
		m.updateLayout()
		m.refreshDetailViewport()
		return m, nil

	case key.Matches(msg, keys.Group):
		m.group = m.group.next()
		m.rebuildList()
		return m, nil

	case key.Matches(msg, keys.Refresh):
		return m, loadSessions

	case key.Matches(msg, keys.Resume):
		s := m.selectedSession()
		if s != nil {
			cmd, err := makeResumeCmd(*s)
			if err != nil {
				return m, nil
			}
			return m, tea.ExecProcess(cmd, func(err error) tea.Msg { return sessionLoadedMsg{} })
		}
		return m, nil

	case key.Matches(msg, keys.Enter):
		if m.focusPane == paneList {
			s := m.selectedSession()
			if s != nil {
				return m, loadDetailCmd(*s)
			}
		}
		return m, nil
	}

	// Route navigation to focused pane
	if m.focusPane == paneList {
		var cmd tea.Cmd
		m.list, cmd = m.list.Update(msg)
		return m, cmd
	}

	// Viewport pane
	var cmd tea.Cmd
	m.viewport, cmd = m.viewport.Update(msg)
	return m, cmd
}

func (m *Model) selectedSession() *claude.Session {
	item := m.list.SelectedItem()
	if item == nil {
		return nil
	}
	si, ok := item.(sessionItem)
	if !ok {
		return nil
	}
	return &si.session
}

func (m *Model) refreshDetailViewport() {
	if m.detailSession != nil {
		vpWidth := m.detailPaneWidth() - 2
		m.detailContent = renderDetailContent(*m.detailSession, m.detailMsgs, vpWidth)
		m.viewport.SetContent(m.detailContent)
	}
}

func (m *Model) rebuildList() {
	filtered := filterSessions(m.sessions, m.group)
	items := sessionsToItems(filtered)
	m.list.SetItems(items)
}

func (m *Model) updateLayout() {
	listW := m.listPaneWidth()
	detailW := m.detailPaneWidth()
	// Subtract 2 for border on each pane
	innerH := m.height - 4 // top border + bottom border + header + footer
	if innerH < 1 {
		innerH = 1
	}

	m.list.SetSize(listW-2, innerH)
	m.viewport.Width = detailW - 2
	m.viewport.Height = innerH
}

func (m Model) listPaneWidth() int {
	if m.focusPane == paneList {
		return int(float64(m.width) * 0.55)
	}
	return int(float64(m.width) * 0.30)
}

func (m Model) detailPaneWidth() int {
	return m.width - m.listPaneWidth()
}

// View renders the split-pane layout.
func (m Model) View() string {
	if !m.ready {
		return "Loading..."
	}

	listW := m.listPaneWidth()
	detailW := m.detailPaneWidth()
	innerH := m.height - 4
	if innerH < 1 {
		innerH = 1
	}

	// Header
	header := statusHeader(m.sessions, m.width)

	// List pane
	var listStyle, detailStyle lipgloss.Style
	if m.focusPane == paneList {
		listStyle = focusedPaneStyle.Width(listW - 2).Height(innerH)
		detailStyle = blurredPaneStyle.Width(detailW - 2).Height(innerH)
	} else {
		listStyle = blurredPaneStyle.Width(listW - 2).Height(innerH)
		detailStyle = focusedPaneStyle.Width(detailW - 2).Height(innerH)
	}

	listView := listStyle.Render(m.list.View())

	// Detail pane
	var detailContent string
	if m.detailSession != nil {
		detailContent = m.viewport.View()
	} else {
		detailContent = normalStyle.Render("\n  Select a session to view details.\n\n  Press Enter on a session in the list.")
	}
	detailView := detailStyle.Render(detailContent)

	body := lipgloss.JoinHorizontal(lipgloss.Top, listView, detailView)

	// Footer
	groupInfo := fmt.Sprintf("[%s]", m.group)
	footer := footerStyle.Render(fmt.Sprintf(
		"↑↓ navigate  tab switch pane  enter detail  r resume  / filter  g %s  R refresh  q quit", groupInfo))

	return header + "\n" + body + "\n" + footer
}

func statusOrder(status string) int {
	switch status {
	case "active":
		return 0
	case "suspended":
		return 1
	default:
		return 2
	}
}
