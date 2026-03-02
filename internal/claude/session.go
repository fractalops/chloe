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

	// Stats (populated by LoadSessionStats)
	Stats *SessionStats
}

// SessionStats holds token usage, cost, and timing info.
type SessionStats struct {
	Model             string
	InputTokens       int64
	OutputTokens      int64
	CacheReadTokens   int64
	CacheCreateTokens int64
	TurnCount         int
	TotalDurationMs   int64
	CostUSD           float64
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
// It reads the first few user messages for metadata and counts total lines.
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
	// Allow large lines (some assistant messages can be very long)
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 1024*1024)

	lineCount := 0
	foundMeta := false

	for scanner.Scan() {
		lineCount++
		if foundMeta && lineCount > 50 {
			// After metadata found, just count remaining lines quickly
			for scanner.Scan() {
				lineCount++
			}
			break
		}

		var line jsonlLine
		if err := json.Unmarshal(scanner.Bytes(), &line); err != nil {
			continue
		}

		// Skip non-message lines
		if line.Type != roleUser && line.Type != roleAssistant {
			continue
		}

		// Extract metadata from the first user message that belongs to this session.
		// Files may contain messages from prior sessions (via --resume/--continue),
		// so we match on sessionId == filename-derived ID.
		if foundMeta || line.Type != roleUser {
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
			if b.Type == "text" && b.Text != "" {
				return truncate(b.Text, 100)
			}
		}
	}

	return ""
}

func truncate(s string, maxLen int) string {
	// Take first line only
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

// statsLine is a lightweight struct for extracting usage/duration without parsing full content.
type statsLine struct {
	Type       string   `json:"type"`
	Subtype    string   `json:"subtype"`
	DurationMs int64    `json:"durationMs"`
	Message    *statMsg `json:"message"`
}

type statMsg struct {
	Model string     `json:"model"`
	Usage *statUsage `json:"usage"`
}

type statUsage struct {
	InputTokens              int64 `json:"input_tokens"`
	OutputTokens             int64 `json:"output_tokens"`
	CacheReadInputTokens     int64 `json:"cache_read_input_tokens"`
	CacheCreationInputTokens int64 `json:"cache_creation_input_tokens"`
}

// LoadSessionStats scans a session JSONL file for token usage, cost, and timing.
func LoadSessionStats(filePath string) *SessionStats {
	f, err := os.Open(filePath) //nolint:gosec // trusted local session files
	if err != nil {
		return nil
	}
	defer f.Close() //nolint:errcheck // read-only file

	stats := &SessionStats{}
	scanner := bufio.NewScanner(f)
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 1024*1024)

	for scanner.Scan() {
		var line statsLine
		if err := json.Unmarshal(scanner.Bytes(), &line); err != nil {
			continue
		}

		// Turn durations from system messages
		if line.Type == "system" && line.Subtype == "turn_duration" && line.DurationMs > 0 {
			stats.TurnCount++
			stats.TotalDurationMs += line.DurationMs
			continue
		}

		// Token usage from assistant messages
		if line.Type == roleAssistant && line.Message != nil {
			if stats.Model == "" && line.Message.Model != "" {
				stats.Model = line.Message.Model
			}
			if u := line.Message.Usage; u != nil {
				stats.InputTokens += u.InputTokens
				stats.OutputTokens += u.OutputTokens
				stats.CacheReadTokens += u.CacheReadInputTokens
				stats.CacheCreateTokens += u.CacheCreationInputTokens
			}
		}
	}

	stats.CostUSD = estimateCost(stats)
	return stats
}

// estimateCost calculates USD cost based on model pricing.
// Prices from https://platform.claude.com/docs/en/about-claude/pricing
// Cache write = 1.25x base input, cache read = 0.1x base input.
func estimateCost(s *SessionStats) float64 {
	var inputPrice, outputPrice float64

	switch {
	case strings.Contains(s.Model, "opus-4-6"),
		strings.Contains(s.Model, "opus-4-5"):
		inputPrice = 5.0
		outputPrice = 25.0
	case strings.Contains(s.Model, "opus-4-1"),
		strings.Contains(s.Model, "opus-4-2"),
		strings.Contains(s.Model, "opus-4"):
		inputPrice = 15.0
		outputPrice = 75.0
	case strings.Contains(s.Model, "opus-3"):
		inputPrice = 15.0
		outputPrice = 75.0
	case strings.Contains(s.Model, "sonnet"):
		inputPrice = 3.0
		outputPrice = 15.0
	case strings.Contains(s.Model, "haiku-4"):
		inputPrice = 1.0
		outputPrice = 5.0
	case strings.Contains(s.Model, "haiku-3-5"):
		inputPrice = 0.80
		outputPrice = 4.0
	case strings.Contains(s.Model, "haiku"):
		inputPrice = 0.25
		outputPrice = 1.25
	default:
		inputPrice = 3.0
		outputPrice = 15.0
	}

	cacheWritePrice := inputPrice * 1.25
	cacheReadPrice := inputPrice * 0.10

	return (float64(s.InputTokens)*inputPrice +
		float64(s.OutputTokens)*outputPrice +
		float64(s.CacheReadTokens)*cacheReadPrice +
		float64(s.CacheCreateTokens)*cacheWritePrice) / 1_000_000
}

// ConversationMessage holds a parsed message for the detail view.
type ConversationMessage struct {
	Role      string
	Content   string
	Timestamp time.Time
}

// LoadConversation reads the full conversation from a session file.
// Returns up to maxMessages user/assistant messages.
func LoadConversation(filePath string, maxMessages int) ([]ConversationMessage, error) {
	f, err := os.Open(filePath) //nolint:gosec // trusted local session files
	if err != nil {
		return nil, err
	}
	defer f.Close() //nolint:errcheck // read-only file

	scanner := bufio.NewScanner(f)
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 1024*1024)

	var msgs []ConversationMessage
	for scanner.Scan() {
		if len(msgs) >= maxMessages {
			break
		}

		var line jsonlLine
		if err := json.Unmarshal(scanner.Bytes(), &line); err != nil {
			continue
		}

		if line.Type != roleUser && line.Type != roleAssistant {
			continue
		}

		text := extractFullMessageText(line.Message)
		if text == "" {
			continue
		}

		var ts time.Time
		if line.Timestamp != "" {
			ts, _ = time.Parse(time.RFC3339Nano, line.Timestamp)
		}

		msgs = append(msgs, ConversationMessage{
			Role:      line.Type,
			Content:   text,
			Timestamp: ts,
		})
	}

	return msgs, nil
}

// extractFullMessageText pulls full text content (not truncated).
func extractFullMessageText(raw json.RawMessage) string {
	if raw == nil {
		return ""
	}

	var body messageBody
	if err := json.Unmarshal(raw, &body); err != nil {
		return ""
	}

	var text string
	if err := json.Unmarshal(body.Content, &text); err == nil {
		return text
	}

	var blocks []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if err := json.Unmarshal(body.Content, &blocks); err == nil {
		var parts []string
		for _, b := range blocks {
			if b.Type == "text" && b.Text != "" {
				parts = append(parts, b.Text)
			}
		}
		return strings.Join(parts, "\n")
	}

	return ""
}
