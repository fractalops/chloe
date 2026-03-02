package tui

import (
	"fmt"
	"os/exec"

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
