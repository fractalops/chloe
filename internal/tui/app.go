package tui

import (
	"fmt"
	"sort"
	"time"

	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/spinner"
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

	statsCache     *claude.StatsCache
	tokenSnapshots map[string]tokenSnapshot
	burnRates      map[string]float64

	help          help.Model
	spinner       spinner.Model
	detailLoading bool

	width, height int
	ready         bool
}

// sessionLoadedMsg is sent when sessions are loaded.
type sessionLoadedMsg struct {
	sessions    []claude.Session
	tokenCounts map[string]int64
	statsCache  *claude.StatsCache
}

// detailLoadedMsg is sent when a session's detail has been loaded.
type detailLoadedMsg struct {
	session claude.Session
	msgs    []claude.ConversationMessage
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

	sp := spinner.New()
	sp.Spinner = spinner.MiniDot
	sp.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("#FF6600"))

	h := help.New()
	h.Styles.ShortKey = lipgloss.NewStyle().Foreground(lipgloss.Color("#888888"))
	h.Styles.ShortDesc = lipgloss.NewStyle().Foreground(lipgloss.Color("#666666"))
	h.Styles.ShortSeparator = lipgloss.NewStyle().Foreground(lipgloss.Color("#444444"))
	h.Styles.FullKey = h.Styles.ShortKey
	h.Styles.FullDesc = h.Styles.ShortDesc
	h.Styles.FullSeparator = h.Styles.ShortSeparator

	return Model{
		list:           l,
		viewport:       vp,
		overlayVP:      ovp,
		help:           h,
		spinner:        sp,
		focusPane:      paneList,
		selectedBubble: -1,
		group:          groupAll,
		tokenSnapshots: make(map[string]tokenSnapshot),
		burnRates:      make(map[string]float64),
	}
}

// Init loads sessions on startup and starts the auto-refresh ticker.
func (m Model) Init() tea.Cmd {
	return tea.Batch(loadSessions, tickCmd(), m.spinner.Tick)
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

	statsCache := claude.LoadStatsCache()

	return sessionLoadedMsg{sessions: sessions, tokenCounts: tokenCounts, statsCache: statsCache}
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
		m.help.Width = m.width - 2
		m.updateLayout()
		if m.overlayActive {
			m.resizeOverlay()
		}
		m.ready = true
		return m, nil

	case tickMsg:
		// Don't refresh while filtering
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
		m.statsCache = msg.statsCache

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
		wasLoaded := m.detailLoaded
		m.detailSession = &msg.session
		m.detailMsgs = msg.msgs
		m.detailLoaded = true
		m.detailLoading = false
		m.selectedBubble = -1
		// First load: switch focus to detail pane
		if !wasLoaded {
			m.focusPane = paneDetail
		}
		m.updateLayout()
		m.refreshDetailViewport()
		m.viewport.SetYOffset(0)
		return m, nil

	case spinner.TickMsg:
		if !m.ready || m.detailLoading || len(m.sessions) == 0 {
			var cmd tea.Cmd
			m.spinner, cmd = m.spinner.Update(msg)
			return m, cmd
		}
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
		if m.detailLoaded {
			if m.focusPane == paneList {
				m.focusPane = paneDetail
			} else {
				m.focusPane = paneList
			}
			m.updateLayout()
			m.refreshDetailViewport()
		}
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
			cmd := launchSession(*s)
			return m, cmd
		}
		return m, nil

	case key.Matches(msg, keys.New):
		cwd := ""
		if s := m.selectedSession(); s != nil {
			cwd = s.CWD
		}
		cmd := launchNewSession(cwd)
		return m, cmd

	case key.Matches(msg, keys.Escape):
		if m.detailLoaded {
			m.detailLoaded = false
			m.detailSession = nil
			m.detailMsgs = nil
			m.detailContent = ""
			m.focusPane = paneList
			m.setCompactMode(false)
			m.updateLayout()
			return m, nil
		}
		return m, nil

	case key.Matches(msg, keys.Enter):
		if m.focusPane == paneDetail {
			// Delegate to detail key handler for bubble mode / overlay
			return m.handleDetailKey(msg)
		}
		if m.focusPane == paneList {
			if m.detailLoaded {
				// Detail already open — just switch focus to detail
				m.focusPane = paneDetail
				m.updateLayout()
				m.refreshDetailViewport()
				return m, nil
			}
			// Detail closed — load detail and switch to compact mode
			s := m.selectedSession()
			if s != nil {
				m.detailLoading = true
				m.setCompactMode(true)
				return m, tea.Batch(loadDetailCmd(*s), m.spinner.Tick)
			}
		}
		return m, nil
	}

	// Route navigation to focused pane
	if m.focusPane == paneList {
		prevIdx := m.list.Index()
		var cmd tea.Cmd
		m.list, cmd = m.list.Update(msg)
		// Auto-update detail when selection changes and detail is open
		if m.detailLoaded && !m.detailLoading && m.list.Index() != prevIdx {
			if s := m.selectedSession(); s != nil {
				m.detailLoading = true
				cmd = tea.Batch(cmd, loadDetailCmd(*s), m.spinner.Tick)
			}
		}
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

	case key.Matches(msg, keys.Enter):
		// Enter activates bubble mode or opens overlay
		if m.selectedBubble == -1 && len(m.bubbleRegions) > 0 {
			m.selectedBubble = 0
			m.refreshDetailViewport()
			m.scrollToBubble(0)
			return m, nil
		}
		if m.selectedBubble >= 0 && m.selectedBubble < len(m.detailMsgs) {
			m.openOverlay(m.selectedBubble)
			return m, nil
		}
		return m, nil

	case key.Matches(msg, keys.Down):
		// In bubble mode: hop between bubbles
		if m.selectedBubble >= 0 {
			if m.selectedBubble < len(m.bubbleRegions)-1 {
				m.selectedBubble++
				m.refreshDetailViewport()
				m.scrollToBubble(m.selectedBubble)
			}
			return m, nil
		}
		// Free-scroll mode: fall through to viewport

	case key.Matches(msg, keys.Up):
		// In bubble mode: hop between bubbles, k from first exits to free-scroll
		if m.selectedBubble > 0 {
			m.selectedBubble--
			m.refreshDetailViewport()
			m.scrollToBubble(m.selectedBubble)
			return m, nil
		}
		if m.selectedBubble == 0 {
			m.selectedBubble = -1
			m.refreshDetailViewport()
			m.viewport.SetYOffset(0)
			return m, nil
		}
		// Free-scroll mode: fall through to viewport
	}

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

func (m *Model) setCompactMode(compact bool) {
	delegate := sessionDelegate{compact: compact}
	m.list.SetDelegate(delegate)
	// Re-set items to force height recalculation
	m.list.SetItems(m.list.Items())
}

func (m *Model) updateLayout() {
	listW := m.listPaneWidth()
	detailW := m.detailPaneWidth()
	// Subtract 2 for border on each pane
	innerH := m.height - 4 // top border + bottom border + header + footer
	if innerH < 1 {
		innerH = 1
	}

	m.list.SetSize(listW-2, innerH-1) // -1 for column header row
	m.viewport.Width = detailW - 2
	m.viewport.Height = innerH
}

func (m Model) listPaneWidth() int {
	if !m.detailLoaded {
		return int(float64(m.width) * 0.90)
	}
	return int(float64(m.width) * 0.50)
}

func (m Model) detailPaneWidth() int {
	return m.width - m.listPaneWidth()
}

// View renders the split-pane layout.
func (m Model) View() string {
	if !m.ready {
		loading := m.spinner.View() + " Loading sessions…"
		if m.width > 0 && m.height > 0 {
			return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, loading)
		}
		return loading
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
	header := statusHeader(m.sessions, m.width, m.statsCache)

	// List pane
	var listStyle, detailStyle lipgloss.Style
	if m.focusPane == paneList {
		listStyle = focusedPaneStyle.Width(listW - 2).Height(innerH)
		detailStyle = blurredPaneStyle.Width(detailW - 2).Height(innerH)
	} else {
		listStyle = blurredPaneStyle.Width(listW - 2).Height(innerH)
		detailStyle = focusedPaneStyle.Width(detailW - 2).Height(innerH)
	}

	var listContent string
	if len(m.sessions) == 0 {
		loadingText := m.spinner.View() + " Loading sessions…"
		listContent = lipgloss.Place(listW-2, innerH, lipgloss.Center, lipgloss.Center, loadingText)
	} else {
		colHeader := m.listColumnHeader(listW - 2)
		listContent = colHeader + "\n" + m.list.View()
	}
	listView := listStyle.Render(listContent)

	// Detail pane
	var detailContent string
	if m.detailLoading {
		loadingText := m.spinner.View() + " Loading..."
		detailContent = lipgloss.Place(detailW-2, innerH, lipgloss.Center, lipgloss.Center, loadingText)
	} else if m.detailSession != nil {
		detailContent = m.viewport.View()
	} else {
		hint := normalStyle.Foreground(lipgloss.Color("#666666")).Render("Select a session and press Enter")
		detailContent = lipgloss.Place(detailW-2, innerH, lipgloss.Center, lipgloss.Center, hint)
	}
	detailView := detailStyle.Render(detailContent)

	body := lipgloss.JoinHorizontal(lipgloss.Top, listView, detailView)

	// Footer
	var footer string
	if m.focusPane == paneDetail && m.detailLoaded {
		dk := detailKeys()
		if m.detailSession != nil && m.detailSession.PID > 0 {
			dk.OpenFiles.SetEnabled(true)
		} else {
			dk.OpenFiles.SetEnabled(false)
		}
		footer = footerStyle.Render(m.help.View(dk))
	} else {
		footer = footerStyle.Render(m.help.View(listKeys()))
	}

	return header + "\n" + body + "\n" + footer
}

// listColumnHeader returns a styled column header for the list pane.
func (m Model) listColumnHeader(width int) string {
	hs := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#888888")).
		Bold(true)

	if m.detailLoaded {
		// Compact mode header
		ageW := len("AGE")
		projW := width - 1 - colStatusWidth - 2 - ageW // " STS  PROJECT...AGE"
		if projW < 4 {
			projW = 4
		}
		header := fmt.Sprintf(" %s  %s%s",
			hs.Render(fmt.Sprintf("%-*s", colStatusWidth, " STS")),
			hs.Render(fmt.Sprintf("%-*s", projW, "PROJECT")),
			hs.Render(fmt.Sprintf("%*s", ageW, "AGE")),
		)
		return header
	}

	// Wide mode: style each column individually (matching data row ANSI structure)
	descW := wideDescWidth(width)
	header := wideRow(
		hs.Render(fmt.Sprintf("%-*s", colStatusWidth, " STS")),
		hs.Render(fmt.Sprintf("%-*s", colProjectWidth, "PROJECT")),
		hs.Render(fmt.Sprintf("%-*s", descW, "DESCRIPTION")),
		hs.Render(fmt.Sprintf("%*s", colTokenWidth, "TOKENS")),
		hs.Render(fmt.Sprintf("%-*s", colOutcomeWidth, "")),
		hs.Render(fmt.Sprintf("%*s", colAgeWidth, "AGE")),
	)
	return header
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
