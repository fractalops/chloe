package tui

import (
	"fmt"
	"sort"
	"time"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/textinput"
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

// tokenSnapshot holds a point-in-time token count for burn rate calculation.
type tokenSnapshot struct {
	total     int64
	timestamp time.Time
}

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
	detailLoaded  bool

	// Bubble navigation in detail pane
	bubbleRegions  []BubbleRegion
	selectedBubble int

	// Full-content overlay
	overlayActive bool
	overlayKind   string // "message" or "files"
	overlayVP     viewport.Model
	overlayMsgIdx int

	tokenSnapshots map[string]tokenSnapshot
	burnRates      map[string]float64

	searchMode   bool
	searchInput  textinput.Model
	searchActive bool // true when displaying search results
	searchQuery  string

	width, height int
	ready         bool
}

// sessionLoadedMsg is sent when sessions are loaded.
type sessionLoadedMsg struct {
	sessions    []claude.Session
	tokenCounts map[string]int64
}

// detailLoadedMsg is sent when a session's detail has been loaded.
type detailLoadedMsg struct {
	session claude.Session
	msgs    []claude.ConversationMessage
}

// searchResultMsg is sent when search completes.
type searchResultMsg struct {
	query    string
	sessions []claude.Session
}

// openFilesMsg is sent when open files for a PID have been loaded.
type openFilesMsg struct {
	files []claude.OpenFile
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

	ovp := viewport.New(0, 0)

	ti := textinput.New()
	ti.Prompt = "Search: "
	ti.PromptStyle = footerStyle
	ti.TextStyle = footerStyle
	ti.CharLimit = 256

	return Model{
		list:           l,
		viewport:       vp,
		overlayVP:      ovp,
		searchInput:    ti,
		focusPane:      paneList,
		selectedBubble: -1,
		group:          groupAll,
		tokenSnapshots: make(map[string]tokenSnapshot),
		burnRates:      make(map[string]float64),
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

	// Collect token counts for active sessions only (for burn rate calculation).
	// QuickTokenCount does a full file scan, so we only call it for sessions
	// with a running process — not during bulk loading.
	tokenCounts := make(map[string]int64)
	for _, s := range sessions {
		if s.PID > 0 {
			tokenCounts[s.ID] = claude.QuickTokenCount(s.FilePath)
		}
	}

	return sessionLoadedMsg{sessions: sessions, tokenCounts: tokenCounts}
}

func searchCmd(sessions []claude.Session, query string) tea.Cmd {
	return func() tea.Msg {
		results := claude.SearchSessions(sessions, query)
		return searchResultMsg{query: query, sessions: results}
	}
}

func loadOpenFilesCmd(pid int) tea.Cmd {
	return func() tea.Msg {
		files := claude.ListOpenFiles(pid)
		return openFilesMsg{files: files}
	}
}

func loadDetailCmd(s claude.Session) tea.Cmd {
	return func() tea.Msg {
		msgs, stats, _ := claude.LoadSessionDetail(s.FilePath, 50)
		s.Stats = stats
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
		if m.overlayActive {
			m.resizeOverlay()
		}
		m.ready = true
		return m, nil

	case searchResultMsg:
		m.searchActive = true
		m.searchQuery = msg.query
		items := sessionsToItems(msg.sessions)
		m.list.SetItems(items)
		return m, nil

	case tickMsg:
		// Don't refresh while filtering, searching, or viewing search results
		if m.list.FilterState() != list.Unfiltered || m.searchMode || m.searchActive {
			return m, tickCmd()
		}
		return m, tea.Batch(loadSessions, tickCmd())

	case sessionLoadedMsg:
		if msg.sessions == nil {
			// Load failed (e.g. no ~/.claude/projects) — don't retry, wait for next tick
			return m, nil
		}
		m.sessions = msg.sessions

		// Compute burn rates from token count deltas
		now := time.Now()
		activeIDs := make(map[string]bool)
		for id, count := range msg.tokenCounts {
			activeIDs[id] = true
			if prev, ok := m.tokenSnapshots[id]; ok {
				dt := now.Sub(prev.timestamp).Minutes()
				if dt > 0 {
					delta := count - prev.total
					if delta > 0 {
						m.burnRates[id] = float64(delta) / dt
					} else {
						m.burnRates[id] = 0
					}
				}
			}
			m.tokenSnapshots[id] = tokenSnapshot{total: count, timestamp: now}
		}
		// Clean up stale entries for sessions no longer active
		for id := range m.tokenSnapshots {
			if !activeIDs[id] {
				delete(m.tokenSnapshots, id)
				delete(m.burnRates, id)
			}
		}

		// Don't rebuild list while filtering or viewing filter results
		if m.list.FilterState() == list.Unfiltered {
			m.rebuildList()
		}
		return m, nil

	case openFilesMsg:
		m.openFilesOverlay(msg.files)
		return m, nil

	case detailLoadedMsg:
		m.detailSession = &msg.session
		m.detailMsgs = msg.msgs
		m.detailLoaded = true
		m.selectedBubble = -1
		if len(msg.msgs) > 0 {
			m.selectedBubble = 0
		}
		m.updateLayout()
		m.refreshDetailViewport()
		m.viewport.GotoTop()
		return m, nil

	case tea.KeyMsg:
		// Overlay takes priority over everything
		if m.overlayActive {
			return m.handleOverlayKey(msg)
		}

		// If list is filtering, let it handle keys
		if m.list.FilterState() == list.Filtering {
			var cmd tea.Cmd
			m.list, cmd = m.list.Update(msg)
			return m, cmd
		}

		// Search mode: intercept all keys for input
		if m.searchMode {
			return m.handleSearchKey(msg)
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
			// Tabbing back to list closes detail and restores wide layout
			m.focusPane = paneList
			m.detailLoaded = false
			m.detailSession = nil
			m.detailMsgs = nil
			m.detailContent = ""
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

	case key.Matches(msg, keys.Search):
		m.searchMode = true
		m.searchInput.Reset()
		m.searchInput.Focus()
		return m, nil

	case key.Matches(msg, keys.Escape):
		if m.searchActive {
			m.searchActive = false
			m.searchQuery = ""
			m.rebuildList()
			return m, nil
		}
		if m.detailLoaded {
			m.detailLoaded = false
			m.detailSession = nil
			m.detailMsgs = nil
			m.detailContent = ""
			m.focusPane = paneList
			m.updateLayout()
			return m, nil
		}
		return m, nil

	case key.Matches(msg, keys.Enter):
		if m.focusPane == paneList {
			s := m.selectedSession()
			if s != nil {
				return m, loadDetailCmd(*s)
			}
		}
		if m.focusPane == paneDetail && m.selectedBubble >= 0 && m.selectedBubble < len(m.detailMsgs) {
			m.openOverlay(m.selectedBubble)
			return m, nil
		}
		return m, nil
	}

	// Route navigation to focused pane
	if m.focusPane == paneList {
		var cmd tea.Cmd
		m.list, cmd = m.list.Update(msg)
		return m, cmd
	}

	// Detail pane: j/k for bubble-jump navigation, o for open files
	if m.focusPane == paneDetail {
		return m.handleDetailKey(msg)
	}

	// Fallback: forward to viewport
	var cmd tea.Cmd
	m.viewport, cmd = m.viewport.Update(msg)
	return m, cmd
}

func (m Model) handleDetailKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, keys.OpenFiles):
		if m.detailSession != nil && m.detailSession.PID > 0 {
			return m, loadOpenFilesCmd(m.detailSession.PID)
		}
		return m, nil
	case key.Matches(msg, keys.Down):
		if m.selectedBubble == -1 && len(m.bubbleRegions) > 0 {
			m.selectedBubble = 0
		} else if m.selectedBubble < len(m.bubbleRegions)-1 {
			m.selectedBubble++
		} else {
			return m, nil
		}
		m.refreshDetailViewport()
		m.scrollToBubble(m.selectedBubble)
		return m, nil
	case key.Matches(msg, keys.Up):
		if m.selectedBubble == 0 {
			m.selectedBubble = -1
			m.refreshDetailViewport()
			m.viewport.GotoTop()
		} else if m.selectedBubble > 0 {
			m.selectedBubble--
			m.refreshDetailViewport()
			m.scrollToBubble(m.selectedBubble)
		}
		return m, nil
	}
	var cmd tea.Cmd
	m.viewport, cmd = m.viewport.Update(msg)
	return m, cmd
}

func (m Model) handleSearchKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEnter:
		query := m.searchInput.Value()
		m.searchMode = false
		m.searchInput.Blur()
		if query == "" {
			return m, nil
		}
		return m, searchCmd(m.sessions, query)
	case tea.KeyEscape:
		m.searchMode = false
		m.searchInput.Blur()
		m.searchInput.Reset()
		return m, nil
	}
	var cmd tea.Cmd
	m.searchInput, cmd = m.searchInput.Update(msg)
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
		br := m.burnRates[m.detailSession.ID]
		content, regions := renderDetailContent(*m.detailSession, m.detailMsgs, vpWidth, m.selectedBubble, br)
		m.detailContent = content
		m.bubbleRegions = regions
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
	if !m.detailLoaded {
		return int(float64(m.width) * 0.90)
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

	if m.overlayActive {
		return m.renderOverlay()
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
		hint := normalStyle.Foreground(lipgloss.Color("#666666")).Render("Select a session and press Enter")
		detailContent = lipgloss.Place(detailW-2, innerH, lipgloss.Center, lipgloss.Center, hint)
	}
	detailView := detailStyle.Render(detailContent)

	body := lipgloss.JoinHorizontal(lipgloss.Top, listView, detailView)

	// Footer
	var footer string
	if m.searchMode {
		footer = footerStyle.Render(m.searchInput.View())
	} else if m.searchActive {
		groupInfo := fmt.Sprintf("[%s]", m.group)
		footer = footerStyle.Render(fmt.Sprintf(
			"[search: %q — esc clear]  ↑↓ navigate  tab switch pane  enter detail  r resume  / filter  g %s  R refresh  q quit",
			m.searchQuery, groupInfo))
	} else if m.focusPane == paneDetail && m.detailLoaded {
		footerText := "j/k bubble nav  enter expand  tab back  esc close  q quit"
		if m.detailSession != nil && m.detailSession.PID > 0 {
			footerText = "j/k bubble nav  enter expand  o open files  tab back  esc close  q quit"
		}
		footer = footerStyle.Render(footerText)
	} else {
		groupInfo := fmt.Sprintf("[%s]", m.group)
		footer = footerStyle.Render(fmt.Sprintf(
			"↑↓ navigate  tab switch pane  enter detail  r resume  / filter  s search  g %s  R refresh  q quit", groupInfo))
	}

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
