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
	PID       int
	Status    string // "active" | "suspended"
	CPUPct    float64
	MemPct    float64
	RSSKB     int64
	OpenFiles int
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
		sessionID, openFiles := findSessionIDForPID(proc.PID, projects)
		if sessionID != "" {
			proc.OpenFiles = openFiles
			mapping[sessionID] = proc
		}
	}

	return mapping
}

// findClaudeProcesses returns claude processes with their PIDs and states.
func findClaudeProcesses() []ProcessInfo {
	out, err := exec.Command("ps", "-eo", "pid,stat,comm,%cpu,%mem,rss").Output()
	if err != nil {
		return nil
	}

	var procs []ProcessInfo
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		fields := strings.Fields(line)
		if len(fields) < 6 {
			continue
		}
		if fields[2] != "claude" {
			continue
		}
		pid, err := strconv.Atoi(fields[0])
		if err != nil {
			continue
		}
		cpuPct, _ := strconv.ParseFloat(fields[3], 64)
		memPct, _ := strconv.ParseFloat(fields[4], 64)
		rssKB, _ := strconv.ParseInt(fields[5], 10, 64)
		procs = append(procs, ProcessInfo{
			PID:    pid,
			Status: statusFromStat(fields[1]),
			CPUPct: cpuPct,
			MemPct: memPct,
			RSSKB:  rssKB,
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
// Returns session ID and open file count.
func findSessionIDForPID(pid int, projects []ProjectInfo) (string, int) {
	out, err := exec.Command("lsof", "-p", strconv.Itoa(pid)).Output() //nolint:gosec // pid is from ps output
	if err != nil {
		return "", 0
	}

	lines := strings.Split(string(out), "\n")

	// First pass: count all open files and collect metadata.
	var cwd string
	var sessionID string
	openFiles := 0
	for i, line := range lines {
		if i == 0 || strings.TrimSpace(line) == "" {
			continue
		}
		openFiles++

		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		path := fields[len(fields)-1]

		// Strategy 1: .claude/tasks/<uuid> directory
		if sessionID == "" && strings.Contains(path, ".claude/tasks/") {
			base := filepath.Base(path)
			if IsUUID(base) {
				sessionID = base
			}
		}

		// Capture cwd for fallback
		if cwd == "" && len(fields) >= 4 && fields[3] == "cwd" {
			cwd = path
		}
	}

	if sessionID != "" {
		return sessionID, openFiles
	}

	// Strategy 2: match cwd → project → most recent JSONL
	if cwd != "" && len(projects) > 0 {
		return findSessionByCwd(cwd, projects), openFiles
	}

	return "", openFiles
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

// OpenFile holds a file path and its access mode.
type OpenFile struct {
	Path string
	Mode string // "r" (read), "w" (write), "rw" (read/write)
}

// ListOpenFiles returns deduplicated open files with access modes for the given PID.
// It filters to regular files and directories only (skips sockets, pipes, devices, etc.).
func ListOpenFiles(pid int) []OpenFile {
	out, err := exec.Command("lsof", "-p", strconv.Itoa(pid)).Output() //nolint:gosec // pid is from ps output
	if err != nil {
		return nil
	}

	seen := make(map[string]int) // path -> index in files
	var files []OpenFile
	for i, line := range strings.Split(string(out), "\n") {
		if i == 0 || strings.TrimSpace(line) == "" {
			continue // skip header and empty lines
		}
		fields := strings.Fields(line)
		if len(fields) < 5 {
			continue
		}
		// TYPE is typically column index 4
		typ := fields[4]
		if typ != "REG" && typ != "DIR" {
			continue
		}
		// FD column (index 3) ends with access mode: r=read, w=write, u=read+write
		fd := fields[3]
		mode := modeFromFD(fd)
		path := fields[len(fields)-1]
		if idx, ok := seen[path]; ok {
			// Upgrade mode if same file opened with broader access
			files[idx].Mode = mergeMode(files[idx].Mode, mode)
		} else {
			seen[path] = len(files)
			files = append(files, OpenFile{Path: path, Mode: mode})
		}
	}
	sort.Slice(files, func(i, j int) bool {
		return files[i].Path < files[j].Path
	})
	return files
}

// modeFromFD extracts the access mode from an lsof FD column value.
// Numeric FDs end with r/w/u (e.g. "3r", "4w", "5u").
// Special FDs like "cwd", "txt", "mem" are treated as read.
func modeFromFD(fd string) string {
	if len(fd) == 0 {
		return "r"
	}
	switch fd[len(fd)-1] {
	case 'w':
		return "w"
	case 'u':
		return "rw"
	default:
		return "r"
	}
}

// mergeMode returns the broadest mode from two mode strings.
func mergeMode(a, b string) string {
	if a == "rw" || b == "rw" {
		return "rw"
	}
	if (a == "r" && b == "w") || (a == "w" && b == "r") {
		return "rw"
	}
	if a == "w" || b == "w" {
		return "w"
	}
	return "r"
}

// ApplyPIDMappings updates sessions with active/suspended PID information.
func ApplyPIDMappings(sessions []Session, mapping PIDMapping) {
	for i := range sessions {
		if proc, ok := mapping[sessions[i].ID]; ok {
			sessions[i].PID = proc.PID
			sessions[i].Status = proc.Status
			sessions[i].CPUPct = proc.CPUPct
			sessions[i].MemPct = proc.MemPct
			sessions[i].RSSKB = proc.RSSKB
			sessions[i].OpenFiles = proc.OpenFiles
		}
	}
}
