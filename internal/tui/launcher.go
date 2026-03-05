package tui

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"unicode/utf8"
)

// LaunchStrategy determines how to open a new terminal context.
type LaunchStrategy int

const (
	StrategyTmux        LaunchStrategy = iota // inside tmux — new window
	StrategyWezTerm                           // new tab via wezterm cli
	StrategyKitty                             // new tab via kitty @ launch
	StrategyGhostty                           // new window via ghostty -e
	StrategyITerm2                            // new tab via AppleScript
	StrategyTerminalApp                       // new tab via AppleScript
	StrategyFallback                          // suspend TUI via tea.ExecProcess
)

// errFallback signals that no external terminal strategy is available.
var errFallback = errors.New("no external terminal strategy available")

// detectStrategy checks the environment to determine the best launch strategy.
// Priority: tmux (inside) > WezTerm > Kitty > Ghostty > iTerm2 > Terminal.app > fallback.
func detectStrategy() LaunchStrategy {
	if os.Getenv("TMUX") != "" {
		return StrategyTmux
	}
	switch os.Getenv("TERM_PROGRAM") {
	case "WezTerm":
		return StrategyWezTerm
	case "ghostty":
		return StrategyGhostty
	case "iTerm.app":
		return StrategyITerm2
	case "Apple_Terminal":
		return StrategyTerminalApp
	}
	// Kitty sets TERM_PROGRAM=xterm-kitty, but KITTY_PID is more reliable.
	if os.Getenv("KITTY_PID") != "" {
		return StrategyKitty
	}
	return StrategyFallback
}

// launchInNewContext opens a claude command in a new terminal context.
// Returns errFallback when no external terminal is detected so the caller
// can fall back to tea.ExecProcess.
func launchInNewContext(args []string, cwd string, label string) error {
	switch detectStrategy() {
	case StrategyTmux:
		return launchTmux(args, cwd, label)
	case StrategyWezTerm:
		return launchWezTerm(args, cwd)
	case StrategyKitty:
		return launchKitty(args, cwd)
	case StrategyGhostty:
		return launchGhostty(args, cwd)
	case StrategyITerm2:
		return launchITerm2(args, cwd)
	case StrategyTerminalApp:
		return launchTerminalApp(args, cwd)
	default:
		return errFallback
	}
}

// shellCommand builds a shell command string from claude args and cwd.
// Each component is individually single-quoted to prevent word splitting.
func shellCommand(args []string, cwd string) string {
	escapedCwd := shellQuote(cwd)
	parts := make([]string, len(args))
	for i, a := range args {
		parts[i] = shellQuote(a)
	}
	if len(parts) == 0 {
		return fmt.Sprintf("cd %s && claude", escapedCwd)
	}
	return fmt.Sprintf("cd %s && claude %s", escapedCwd, strings.Join(parts, " "))
}

// shellQuote wraps s in single quotes, escaping any internal single quotes.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'"
}

// escapeAppleScript escapes a string for embedding inside an AppleScript
// double-quoted string. Backslashes must be escaped before double quotes.
func escapeAppleScript(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	return s
}

// truncateUTF8 truncates s to at most maxLen runes without splitting multi-byte characters.
func truncateUTF8(s string, maxLen int) string {
	if utf8.RuneCountInString(s) <= maxLen {
		return s
	}
	runes := []rune(s)
	return string(runes[:maxLen])
}

// claudeArgs prepends "claude" to args for use with launchers that take a
// program and its arguments separately (WezTerm, Kitty).
func claudeArgs(args []string) []string {
	return append([]string{"claude"}, args...)
}

func launchTmux(args []string, cwd string, label string) error {
	cmd := shellCommand(args, cwd)
	label = truncateUTF8(label, 30)
	return exec.Command("tmux", "new-window", "-n", label, cmd).Run() //nolint:gosec // args validated by caller (UUID check)
}

// launchWezTerm opens a new tab via `wezterm cli spawn --cwd <dir> -- claude <args>`.
func launchWezTerm(args []string, cwd string) error {
	cmdArgs := []string{"cli", "spawn", "--cwd", cwd, "--"}
	cmdArgs = append(cmdArgs, claudeArgs(args)...)
	return exec.Command("wezterm", cmdArgs...).Run() //nolint:gosec // args validated by caller (UUID check)
}

// launchKitty opens a new tab via `kitty @ launch --type=tab --cwd=<dir> claude <args>`.
// Requires allow_remote_control=yes in kitty.conf.
func launchKitty(args []string, cwd string) error {
	cmdArgs := []string{"@", "launch", "--type=tab", "--cwd=" + cwd}
	cmdArgs = append(cmdArgs, claudeArgs(args)...)
	return exec.Command("kitty", cmdArgs...).Run() //nolint:gosec // args validated by caller (UUID check)
}

// launchGhostty opens a new window via `ghostty -e /bin/sh -c "<cmd>"`.
// Ghostty does not support programmatic tab creation.
func launchGhostty(args []string, cwd string) error {
	cmd := shellCommand(args, cwd)
	return exec.Command("ghostty", "-e", "/bin/sh", "-c", cmd).Start() //nolint:gosec // args validated by caller (UUID check)
}

func launchITerm2(args []string, cwd string) error {
	cmd := shellCommand(args, cwd)
	script := fmt.Sprintf(`tell application "iTerm2"
	tell current window
		create tab with default profile
		tell current session
			write text "%s"
		end tell
	end tell
end tell`, escapeAppleScript(cmd))
	return exec.Command("osascript", "-e", script).Run() //nolint:gosec // cmd is shell-quoted, script is AppleScript-escaped
}

func launchTerminalApp(args []string, cwd string) error {
	cmd := shellCommand(args, cwd)
	script := fmt.Sprintf(`tell application "Terminal"
	activate
	do script "%s"
end tell`, escapeAppleScript(cmd))
	return exec.Command("osascript", "-e", script).Run() //nolint:gosec // cmd is shell-quoted, script is AppleScript-escaped
}
