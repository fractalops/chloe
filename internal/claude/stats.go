package claude

// SessionStats holds token usage, cost, and timing info.
type SessionStats struct {
	Model             string
	InputTokens       int64
	OutputTokens      int64
	CacheReadTokens   int64
	CacheCreateTokens int64
	TurnCount         int
	TotalDurationMs   int64
}

// statsLine is a lightweight struct for extracting usage from JSONL lines.
type statsLine struct {
	Type    string   `json:"type"`
	Message *statMsg `json:"message"`
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
