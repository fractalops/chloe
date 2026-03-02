package claude

import (
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

// ProcessInfo holds a matched claude process's PID and state.
type ProcessInfo struct {
	PID    int
	Status string // "active" | "suspended"
}

// PIDMapping maps session IDs to process info.
type PIDMapping map[string]ProcessInfo

// DetectActiveProcesses finds running claude processes and maps them to session IDs.
//
// Strategy per PID:
//  1. lsof for .claude/tasks/<uuid> — reliable when the session uses the task system
//  2. Fallback: map process cwd → project directory → most recently modified JSONL
func DetectActiveProcesses() PIDMapping {
	mapping := make(PIDMapping)

	procs := findClaudeProcesses()
	if len(procs) == 0 {
		return mapping
	}

	// Pre-discover all projects for cwd fallback
	baseDir, _ := ClaudeProjectsDir()
	var projects []ProjectInfo
	if baseDir != "" {
		projects, _ = DiscoverProjects(baseDir)
	}

	for _, proc := range procs {
		sessionID, _ := findSessionIDForPID(proc.PID, projects)
		if sessionID != "" {
			mapping[sessionID] = proc
		}
	}

	return mapping
}

// findClaudeProcesses returns claude processes with their PIDs and states.
func findClaudeProcesses() []ProcessInfo {
	out, err := exec.Command("ps", "-eo", "pid,stat,comm").Output()
	if err != nil {
		return nil
	}

	var procs []ProcessInfo
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		fields := strings.Fields(line)
		if len(fields) < 3 {
			continue
		}
		if fields[2] != "claude" {
			continue
		}
		pid, err := strconv.Atoi(fields[0])
		if err != nil {
			continue
		}
		procs = append(procs, ProcessInfo{
			PID:    pid,
			Status: statusFromStat(fields[1]),
		})
	}

	return procs
}

// statusFromStat converts a ps STAT string to a session status.
// T or T+ means stopped/suspended (ctrl+z). Everything else is active.
func statusFromStat(stat string) string {
	// Strip modifiers like +, s, l, etc. The first char is the state.
	if len(stat) > 0 && stat[0] == 'T' {
		return "suspended"
	}
	return "active"
}

// findSessionIDForPID inspects a claude process to find its session ID.
func findSessionIDForPID(pid int, projects []ProjectInfo) (string, string) {
	out, err := exec.Command("lsof", "-p", strconv.Itoa(pid)).Output() //nolint:gosec // pid is from ps output
	if err != nil {
		return "", ""
	}

	var cwd string
	lines := strings.Split(string(out), "\n")

	for _, line := range lines {
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		path := fields[len(fields)-1]

		// Strategy 1: .claude/tasks/<uuid> directory
		if strings.Contains(path, ".claude/tasks/") {
			base := filepath.Base(path)
			if IsUUID(base) {
				return base, ""
			}
		}

		// Capture cwd for fallback
		if len(fields) >= 4 && fields[3] == "cwd" {
			cwd = path
		}
	}

	// Strategy 2: match cwd → project → most recent JSONL
	if cwd != "" && len(projects) > 0 {
		return findSessionByCwd(cwd, projects), cwd
	}

	return "", cwd
}

// findSessionByCwd matches a process cwd to a Claude project directory,
// then returns the session ID of the most recently modified JSONL file.
func findSessionByCwd(cwd string, projects []ProjectInfo) string {
	type match struct {
		dir     string
		pathLen int
	}
	var matches []match
	for _, p := range projects {
		if strings.HasPrefix(cwd, p.Path) {
			matches = append(matches, match{dir: p.Dir, pathLen: len(p.Path)})
		}
	}

	if len(matches) == 0 {
		return ""
	}

	sort.Slice(matches, func(i, j int) bool {
		return matches[i].pathLen > matches[j].pathLen
	})

	files, err := FindSessionFiles(matches[0].dir)
	if err != nil || len(files) == 0 {
		return ""
	}

	var bestFile string
	var bestTime time.Time
	for _, f := range files {
		info, err := os.Stat(f)
		if err != nil {
			continue
		}
		if info.ModTime().After(bestTime) {
			bestTime = info.ModTime()
			bestFile = f
		}
	}

	if bestFile == "" {
		return ""
	}

	base := filepath.Base(bestFile)
	return strings.TrimSuffix(base, ".jsonl")
}

// isUUID checks if a string looks like a UUID (xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx).
// IsUUID validates that a string is a valid UUID format (8-4-4-4-12 hex).
func IsUUID(s string) bool {
	if len(s) != 36 {
		return false
	}
	for i, c := range s {
		switch {
		case i == 8 || i == 13 || i == 18 || i == 23:
			if c != '-' {
				return false
			}
		case (c < '0' || c > '9') && (c < 'a' || c > 'f') && (c < 'A' || c > 'F'):
			return false
		}
	}
	return true
}

// ApplyPIDMappings updates sessions with active/suspended PID information.
func ApplyPIDMappings(sessions []Session, mapping PIDMapping) {
	for i := range sessions {
		if proc, ok := mapping[sessions[i].ID]; ok {
			sessions[i].PID = proc.PID
			sessions[i].Status = proc.Status
		}
	}
}
