package claude

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	roleUser      = "user"
	roleAssistant = "assistant"
	blockTypeText = "text"
)

// Session represents a Claude Code session parsed from a JSONL file.
type Session struct {
	ID           string
	Project      string // Decoded project path
	ProjectKey   string // Raw directory name
	CWD          string
	Version      string
	GitBranch    string
	Slug         string
	FirstMsg     string    // First user message (truncated)
	StartedAt    time.Time // First message timestamp
	LastActive   time.Time // File mtime
	MessageCount int
	FilePath     string
	PID          int    // 0 if inactive
	Status       string // "active" | "suspended" | "inactive"
	CPUPct       float64
	MemPct       float64
	RSSKB        int64
	OpenFiles    int

	// Stats (populated by LoadSessionDetail)
	Stats *SessionStats
}

// jsonlLine represents the minimal fields we parse from each JSONL line.
type jsonlLine struct {
	Type      string          `json:"type"`
	SessionID string          `json:"sessionId"`
	CWD       string          `json:"cwd"`
	Version   string          `json:"version"`
	GitBranch string          `json:"gitBranch"`
	Slug      string          `json:"slug"`
	Timestamp string          `json:"timestamp"`
	Message   json.RawMessage `json:"message"`
	UserType  string          `json:"userType"`
}

type messageBody struct {
	Role    string          `json:"role"`
	Content json.RawMessage `json:"content"`
}

// ParseSessionFile reads a JSONL file and extracts session metadata.
// It reads until the first matching user message is found, then counts remaining lines.
func ParseSessionFile(filePath string, project ProjectInfo) (*Session, error) {
	f, err := os.Open(filePath) //nolint:gosec // trusted local session files
	if err != nil {
		return nil, err
	}
	defer f.Close() //nolint:errcheck // read-only file

	info, err := f.Stat()
	if err != nil {
		return nil, err
	}

	// Session ID is always derived from the filename — it's authoritative.
	// The JSONL may contain messages from prior sessions (via --resume/--continue),
	// so the sessionId field inside the file can differ from the filename.
	base := filepath.Base(filePath)
	fileID := strings.TrimSuffix(base, ".jsonl")

	sess := &Session{
		ID:         fileID,
		FilePath:   filePath,
		Project:    project.Path,
		ProjectKey: project.Key,
		LastActive: info.ModTime(),
		Status:     "inactive",
	}

	scanner := bufio.NewScanner(f)
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 1024*1024)

	lineCount := 0
	foundMeta := false

	for scanner.Scan() {
		lineCount++

		if foundMeta {
			continue // just counting lines
		}

		var line jsonlLine
		if err := json.Unmarshal(scanner.Bytes(), &line); err != nil {
			continue
		}

		if line.Type != roleUser {
			continue
		}

		// Prefer the message matching our file ID; accept any if none match yet
		if line.SessionID != fileID && (sess.CWD != "" || line.SessionID == "") {
			continue
		}
		sess.CWD = line.CWD
		sess.Version = line.Version
		sess.GitBranch = line.GitBranch
		sess.Slug = line.Slug
		if t, parseErr := time.Parse(time.RFC3339Nano, line.Timestamp); parseErr == nil {
			sess.StartedAt = t
		}
		sess.FirstMsg = extractMessageText(line.Message)
		if line.SessionID == fileID {
			foundMeta = true
		}
	}

	sess.MessageCount = lineCount
	return sess, nil
}

// QuickTokenCount does a lightweight scan of a session file summing only
// input_tokens + output_tokens from assistant lines. Used for burn rate
// calculation on active sessions only — not called during bulk session loading.
func QuickTokenCount(filePath string) int64 {
	f, err := os.Open(filePath) //nolint:gosec // trusted local session files
	if err != nil {
		return 0
	}
	defer f.Close() //nolint:errcheck // read-only file

	scanner := bufio.NewScanner(f)
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 1024*1024)

	var total int64
	for scanner.Scan() {
		var sl statsLine
		if json.Unmarshal(scanner.Bytes(), &sl) != nil {
			continue
		}
		if sl.Type == roleAssistant && sl.Message != nil {
			if u := sl.Message.Usage; u != nil {
				total += u.InputTokens + u.OutputTokens
			}
		}
	}
	return total
}

// extractMessageText pulls the text content from a message JSON.
func extractMessageText(raw json.RawMessage) string {
	if raw == nil {
		return ""
	}

	var body messageBody
	if err := json.Unmarshal(raw, &body); err != nil {
		return ""
	}

	// Content can be a string or array of content blocks
	var text string
	if err := json.Unmarshal(body.Content, &text); err == nil {
		return truncate(text, 100)
	}

	// Try array of content blocks
	var blocks []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if err := json.Unmarshal(body.Content, &blocks); err == nil {
		for _, b := range blocks {
			if b.Type == blockTypeText && b.Text != "" {
				return truncate(b.Text, 100)
			}
		}
	}

	return ""
}

func truncate(s string, maxLen int) string {
	if idx := strings.IndexByte(s, '\n'); idx >= 0 {
		s = s[:idx]
	}
	s = strings.TrimSpace(s)
	if len(s) > maxLen {
		return s[:maxLen-1] + "…"
	}
	return s
}

// LoadAllSessions discovers and parses all sessions across all projects.
func LoadAllSessions() ([]Session, error) {
	baseDir, err := ClaudeProjectsDir()
	if err != nil {
		return nil, err
	}

	projects, err := DiscoverProjects(baseDir)
	if err != nil {
		return nil, err
	}

	var sessions []Session
	for _, proj := range projects {
		files, err := FindSessionFiles(proj.Dir)
		if err != nil {
			continue
		}
		for _, f := range files {
			sess, err := ParseSessionFile(f, proj)
			if err != nil {
				continue
			}
			sessions = append(sessions, *sess)
		}
	}

	return sessions, nil
}
