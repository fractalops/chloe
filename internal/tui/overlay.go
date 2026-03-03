package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/fractalops/chloe/internal/claude"
)

const (
	overlayMessage = "message"
	overlayFiles   = "files"
)

func (m Model) handleOverlayKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, keys.Escape):
		m.overlayActive = false
		return m, nil
	case key.Matches(msg, keys.Quit):
		// q closes overlay too (don't quit app)
		m.overlayActive = false
		return m, nil
	}
	// Forward j/k/up/down to overlay viewport for scrolling
	var cmd tea.Cmd
	m.overlayVP, cmd = m.overlayVP.Update(msg)
	return m, cmd
}

func (m *Model) openOverlay(bubbleIdx int) {
	if bubbleIdx < 0 || bubbleIdx >= len(m.detailMsgs) {
		return
	}
	msg := m.detailMsgs[bubbleIdx]
	m.overlayActive = true
	m.overlayKind = overlayMessage
	m.overlayMsgIdx = bubbleIdx

	w, h := m.overlaySize()
	content := renderFullBubble(msg.Role, msg.RawContent, w-4) // account for border+padding
	m.overlayVP.Width = w - 4
	m.overlayVP.Height = h - 3 // title bar + border
	m.overlayVP.SetContent(content)
	m.overlayVP.GotoTop()
}

func (m *Model) openFilesOverlay(files []claude.OpenFile) {
	m.overlayActive = true
	m.overlayKind = overlayFiles

	w, h := m.overlaySize()
	var content string
	if len(files) == 0 {
		content = "(no open files)"
	} else {
		lines := make([]string, len(files))
		for i, f := range files {
			lines[i] = fmt.Sprintf("[%2s] %s", f.Mode, f.Path)
		}
		content = strings.Join(lines, "\n")
	}
	m.overlayVP.Width = w - 4
	m.overlayVP.Height = h - 3
	m.overlayVP.SetContent(content)
	m.overlayVP.GotoTop()
}

func (m *Model) resizeOverlay() {
	w, h := m.overlaySize()
	m.overlayVP.Width = w - 4
	m.overlayVP.Height = h - 3
}

func (m *Model) scrollToBubble(idx int) {
	if idx < 0 || idx >= len(m.bubbleRegions) {
		return
	}
	region := m.bubbleRegions[idx]
	m.viewport.SetYOffset(region.StartLine)
}

func (m Model) renderOverlay() string {
	w, h := m.overlaySize()

	titleText := " Full Message — Esc to close "
	if m.overlayKind == overlayFiles {
		titleText = " Open Files — Esc to close "
	}
	title := overlayTitleStyle.Render(titleText)
	vpView := m.overlayVP.View()

	inner := title + "\n" + vpView
	box := overlayStyle.Width(w - 4).Height(h - 2).Render(inner)

	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, box)
}

// overlaySize returns the overlay's outer width and height.
func (m Model) overlaySize() (int, int) {
	w := m.width - 8
	h := m.height - 6
	if w < 20 {
		w = 20
	}
	if h < 5 {
		h = 5
	}
	return w, h
}
