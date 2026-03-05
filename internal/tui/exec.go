package tui

import (
	"fmt"
	"os"
	"os/exec"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/fractalops/chloe/internal/claude"
)

// makeResumeCmd creates an exec.Cmd for resuming a claude session.
// Returns an error if the session ID is not a valid UUID.
func makeResumeCmd(s claude.Session) (*exec.Cmd, error) {
	if !claude.IsUUID(s.ID) {
		return nil, fmt.Errorf("invalid session ID: %s", s.ID)
	}
	cmd := exec.Command("claude", "--resume", s.ID) //nolint:gosec // ID validated by IsUUID above
	cmd.Dir = s.CWD
	return cmd, nil
}

// makeNewSessionCmd creates an exec.Cmd for starting a new claude session.
func makeNewSessionCmd(cwd string) *exec.Cmd {
	cmd := exec.Command("claude")
	cmd.Dir = cwd
	return cmd
}

// sessionLabel returns a short label for a session (for tmux window / tab names).
func sessionLabel(s claude.Session) string {
	if s.FirstMsg != "" {
		return truncateUTF8(s.FirstMsg, 30)
	}
	if len(s.ID) >= 8 {
		return s.ID[:8]
	}
	return s.ID
}

// launchSession opens a resumed claude session in a new terminal context,
// falling back to tea.ExecProcess if no external terminal is available.
func launchSession(s claude.Session) tea.Cmd {
	if !claude.IsUUID(s.ID) {
		return nil
	}

	args := []string{"--resume", s.ID}
	label := sessionLabel(s)

	err := launchInNewContext(args, s.CWD, label)
	if err == nil {
		return nil
	}

	// External terminal detected but launch failed — fall back to suspended mode.
	cmd, cmdErr := makeResumeCmd(s)
	if cmdErr != nil {
		return nil
	}
	return tea.ExecProcess(cmd, func(err error) tea.Msg { return loadSessions() })
}

// launchNewSession opens a fresh claude session in a new terminal context,
// falling back to tea.ExecProcess if no external terminal is available.
func launchNewSession(cwd string) tea.Cmd {
	if cwd == "" {
		var err error
		cwd, err = os.Getwd()
		if err != nil {
			cwd = "/"
		}
	}

	err := launchInNewContext(nil, cwd, "claude")
	if err == nil {
		return nil
	}

	cmd := makeNewSessionCmd(cwd)
	return tea.ExecProcess(cmd, func(err error) tea.Msg { return loadSessions() })
}
