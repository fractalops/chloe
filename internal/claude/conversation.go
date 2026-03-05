package claude

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"
)

// ConversationMessage holds a parsed message for the detail view.
type ConversationMessage struct {
	Role       string
	Content    string
	RawContent string // Full untruncated content for overlay view
	Timestamp  time.Time
}

// LoadConversation reads the full conversation from a session file.
// Returns up to maxMessages user/assistant messages.
func LoadConversation(filePath string, maxMessages int) ([]ConversationMessage, error) {
	msgs, _, err := LoadSessionDetail(filePath, maxMessages)
	return msgs, err
}

// LoadSessionDetail reads a session JSONL file in a single pass,
// returning both conversation messages and session stats.
func LoadSessionDetail(filePath string, maxMessages int) ([]ConversationMessage, *SessionStats, error) {
	f, err := os.Open(filePath) //nolint:gosec // trusted local session files
	if err != nil {
		return nil, nil, err
	}
	defer f.Close() //nolint:errcheck // read-only file

	scanner := bufio.NewScanner(f)
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 1024*1024)

	var msgs []ConversationMessage
	var cwd string
	stats := &SessionStats{}
	msgsDone := false

	for scanner.Scan() {
		raw := scanner.Bytes()

		var line jsonlLine
		if err := json.Unmarshal(raw, &line); err != nil {
			continue
		}

		// Collect stats from every line (needs full scan)
		collectStats(&line, raw, stats)

		// Collect conversation messages up to the limit
		if msgsDone {
			continue
		}

		if cwd == "" && line.CWD != "" {
			cwd = line.CWD
		}

		if line.Type != roleUser && line.Type != roleAssistant {
			continue
		}

		blocks, isToolResult := parseMessageBlocks(line.Message, cwd)
		if blocks == nil {
			continue
		}

		rawText := formatBlocks(blocks, 0, 0)
		if rawText == "" {
			continue
		}

		role := line.Type
		if role == roleUser && isToolResult {
			role = roleAssistant
		}

		var ts time.Time
		if line.Timestamp != "" {
			ts, _ = time.Parse(time.RFC3339Nano, line.Timestamp)
		}

		truncated := formatBlocks(blocks, 40, 80)

		msgs = append(msgs, ConversationMessage{
			Role:       role,
			Content:    truncated,
			RawContent: rawText,
			Timestamp:  ts,
		})

		if len(msgs) >= maxMessages {
			msgsDone = true
		}
	}

	return msgs, stats, nil
}

// collectStats extracts stats data from a single parsed JSONL line.
func collectStats(line *jsonlLine, raw []byte, stats *SessionStats) {
	// Turn durations from system messages
	if line.Type == "system" {
		var sl struct {
			Subtype    string `json:"subtype"`
			DurationMs int64  `json:"durationMs"`
		}
		if json.Unmarshal(raw, &sl) == nil && sl.Subtype == "turn_duration" && sl.DurationMs > 0 {
			stats.TurnCount++
			stats.TotalDurationMs += sl.DurationMs
		}
		return
	}

	// Token usage from assistant messages
	if line.Type != roleAssistant {
		return
	}
	var al struct {
		Message *statMsg `json:"message"`
	}
	if json.Unmarshal(raw, &al) != nil || al.Message == nil {
		return
	}
	if stats.Model == "" && al.Message.Model != "" {
		stats.Model = al.Message.Model
	}
	if u := al.Message.Usage; u != nil {
		stats.InputTokens += u.InputTokens
		stats.OutputTokens += u.OutputTokens
		stats.CacheReadTokens += u.CacheReadInputTokens
		stats.CacheCreateTokens += u.CacheCreationInputTokens
	}
}

// messageBlock is a parsed content block from a JSONL message.
type messageBlock struct {
	kind     string // blockTypeText, "tool_use", "tool_result"
	text     string // plain text content
	toolName string // tool_use only
	toolArgs []toolArg
	result   string // tool_result only
}

type toolArg struct {
	key string
	val string // already cwd-shortened, untruncated
}

// parseMessageBlocks parses a raw JSONL message into structured blocks.
// Returns nil if the message has no displayable content.
// The bool reports whether the message consists solely of tool_result blocks.
func parseMessageBlocks(raw json.RawMessage, cwd string) ([]messageBlock, bool) {
	if raw == nil {
		return nil, false
	}

	var body messageBody
	if err := json.Unmarshal(raw, &body); err != nil {
		return nil, false
	}

	// Simple string content
	var text string
	if err := json.Unmarshal(body.Content, &text); err == nil {
		if text == "" {
			return nil, false
		}
		return []messageBlock{{kind: blockTypeText, text: text}}, false
	}

	// Array of content blocks
	var rawBlocks []json.RawMessage
	if json.Unmarshal(body.Content, &rawBlocks) != nil {
		return nil, false
	}

	var blocks []messageBlock
	hasText := false
	hasToolResult := false

	for _, blockRaw := range rawBlocks {
		var base struct {
			Type string `json:"type"`
		}
		if json.Unmarshal(blockRaw, &base) != nil {
			continue
		}
		switch base.Type {
		case blockTypeText:
			var tb struct {
				Text string `json:"text"`
			}
			if json.Unmarshal(blockRaw, &tb) == nil && tb.Text != "" {
				hasText = true
				blocks = append(blocks, messageBlock{kind: blockTypeText, text: tb.Text})
			}
		case "tool_use":
			var tu struct {
				Name  string          `json:"name"`
				Input json.RawMessage `json:"input"`
			}
			if json.Unmarshal(blockRaw, &tu) == nil {
				args := parseToolArgs(tu.Input, cwd)
				blocks = append(blocks, messageBlock{kind: "tool_use", toolName: tu.Name, toolArgs: args})
			}
		case "tool_result":
			hasToolResult = true
			var tr struct {
				Content json.RawMessage `json:"content"`
			}
			if json.Unmarshal(blockRaw, &tr) == nil {
				result := extractToolResult(tr.Content, cwd)
				blocks = append(blocks, messageBlock{kind: "tool_result", result: result})
			}
		}
	}

	if len(blocks) == 0 {
		return nil, false
	}
	return blocks, hasToolResult && !hasText
}

// formatBlocks renders parsed blocks into display text.
// toolArgMax and toolResultMax truncate tool content (0 = unlimited).
func formatBlocks(blocks []messageBlock, toolArgMax, toolResultMax int) string {
	var parts []string
	for _, b := range blocks {
		switch b.kind {
		case blockTypeText:
			parts = append(parts, b.text)
		case "tool_use":
			args := formatArgs(b.toolArgs, toolArgMax)
			parts = append(parts, fmt.Sprintf("-> %s(%s)", b.toolName, args))
		case "tool_result":
			r := b.result
			if runes := []rune(r); toolResultMax > 0 && len(runes) > toolResultMax {
				r = string(runes[:toolResultMax-3]) + "..."
			}
			parts = append(parts, fmt.Sprintf("<- %s", r))
		}
	}
	return strings.Join(parts, "\n")
}

// parseToolArgs extracts key=value pairs from tool input JSON.
// Values are cwd-shortened but not truncated. Keys are sorted for deterministic output.
func parseToolArgs(raw json.RawMessage, cwd string) []toolArg {
	if raw == nil {
		return nil
	}
	var obj map[string]json.RawMessage
	if json.Unmarshal(raw, &obj) != nil {
		return nil
	}
	keys := make([]string, 0, len(obj))
	for k := range obj {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	args := make([]toolArg, 0, len(keys))
	for _, k := range keys {
		v := obj[k]
		val := string(v)
		var s string
		if json.Unmarshal(v, &s) == nil {
			val = shortenCwd(s, cwd)
		}
		args = append(args, toolArg{key: k, val: val})
	}
	return args
}

// formatArgs renders tool arguments as "key=val, key=val".
// maxLen truncates each value (0 = unlimited).
func formatArgs(args []toolArg, maxLen int) string {
	parts := make([]string, 0, len(args))
	for _, a := range args {
		val := a.val
		if runes := []rune(val); maxLen > 0 && len(runes) > maxLen {
			val = string(runes[:maxLen-3]) + "..."
		}
		parts = append(parts, fmt.Sprintf("%s=%s", a.key, val))
	}
	return strings.Join(parts, ", ")
}

// extractToolResult extracts full result text from tool result content.
func extractToolResult(raw json.RawMessage, cwd string) string {
	if raw == nil {
		return "(empty)"
	}
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return shortenCwd(s, cwd)
	}
	var blocks []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if json.Unmarshal(raw, &blocks) == nil {
		for _, b := range blocks {
			if b.Type == blockTypeText && b.Text != "" {
				return shortenCwd(b.Text, cwd)
			}
		}
	}
	return "(result)"
}

// shortenCwd replaces a CWD prefix in a path with "./"
func shortenCwd(s, cwd string) string {
	if cwd != "" && strings.HasPrefix(s, cwd+"/") {
		return "./" + s[len(cwd)+1:]
	}
	return s
}
