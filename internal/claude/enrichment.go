package claude

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"time"
)

// SessionMeta holds pre-computed session analytics from ~/.claude/usage-data/session-meta/.
type SessionMeta struct {
	SessionID       string         `json:"session_id"`
	ProjectPath     string         `json:"project_path"`
	DurationMinutes int            `json:"duration_minutes"`
	UserMsgCount    int            `json:"user_message_count"`
	AssistantMsgCnt int            `json:"assistant_message_count"`
	ToolCounts      map[string]int `json:"tool_counts"`
	Languages       map[string]int `json:"languages"`
	GitCommits      int            `json:"git_commits"`
	InputTokens     int64          `json:"input_tokens"`
	OutputTokens    int64          `json:"output_tokens"`
	LinesAdded      int            `json:"lines_added"`
	LinesRemoved    int            `json:"lines_removed"`
	FilesModified   int            `json:"files_modified"`
	UsesMCP         bool           `json:"uses_mcp"`
	UsesWebSearch   bool           `json:"uses_web_search"`
	ToolErrors      int            `json:"tool_errors"`
	ToolErrorCats   map[string]int `json:"tool_error_categories"`
	Summary         string         `json:"summary"`
}

// TotalTokens returns the sum of input and output tokens.
func (m *SessionMeta) TotalTokens() int64 {
	return m.InputTokens + m.OutputTokens
}

// SessionFacets holds session quality metrics from ~/.claude/usage-data/facets/.
type SessionFacets struct {
	SessionID         string         `json:"session_id"`
	UnderlyingGoal    string         `json:"underlying_goal"`
	GoalCategories    map[string]int `json:"goal_categories"`
	Outcome           string         `json:"outcome"`
	UserSatisfaction  map[string]int `json:"user_satisfaction_counts"`
	ClaudeHelpfulness string         `json:"claude_helpfulness"`
	SessionType       string         `json:"session_type"`
	FrictionCounts    map[string]int `json:"friction_counts"`
	FrictionDetail    string         `json:"friction_detail"`
	PrimarySuccess    string         `json:"primary_success"`
	BriefSummary      string         `json:"brief_summary"`
}

// Task holds a single task entry from ~/.claude/tasks/{session}/.
type Task struct {
	ID        string   `json:"id"`
	Subject   string   `json:"subject"`
	Status    string   `json:"status"`
	Blocks    []string `json:"blocks"`
	BlockedBy []string `json:"blockedBy"`
}

// DailyActivity holds one day's usage stats from stats-cache.json.
type DailyActivity struct {
	Date          string `json:"date"`
	MessageCount  int    `json:"messageCount"`
	SessionCount  int    `json:"sessionCount"`
	ToolCallCount int    `json:"toolCallCount"`
}

// StatsCache wraps the global stats-cache.json.
type StatsCache struct {
	Version          int             `json:"version"`
	LastComputedDate string          `json:"lastComputedDate"`
	DailyActivity    []DailyActivity `json:"dailyActivity"`
}

// TodayStats returns the stats for today's date, or nil if not present.
func (sc *StatsCache) TodayStats() *DailyActivity {
	if sc == nil {
		return nil
	}
	today := time.Now().Format("2006-01-02")
	for i := len(sc.DailyActivity) - 1; i >= 0; i-- {
		if sc.DailyActivity[i].Date == today {
			return &sc.DailyActivity[i]
		}
	}
	return nil
}

// claudeDir returns ~/.claude.
func claudeDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".claude"), nil
}

// LoadSessionMeta reads the session-meta file for the given session ID.
func LoadSessionMeta(id string) *SessionMeta {
	dir, err := claudeDir()
	if err != nil {
		return nil
	}
	path := filepath.Join(dir, "usage-data", "session-meta", id+".json")
	data, err := os.ReadFile(path) //nolint:gosec // trusted local files
	if err != nil {
		return nil
	}
	var meta SessionMeta
	if json.Unmarshal(data, &meta) != nil {
		return nil
	}
	return &meta
}

// LoadSessionFacets reads the facets file for the given session ID.
func LoadSessionFacets(id string) *SessionFacets {
	dir, err := claudeDir()
	if err != nil {
		return nil
	}
	path := filepath.Join(dir, "usage-data", "facets", id+".json")
	data, err := os.ReadFile(path) //nolint:gosec // trusted local files
	if err != nil {
		return nil
	}
	var facets SessionFacets
	if json.Unmarshal(data, &facets) != nil {
		return nil
	}
	return &facets
}

// LoadSessionTasks reads all task files for a session, sorted by ID.
func LoadSessionTasks(id string) []Task {
	dir, err := claudeDir()
	if err != nil {
		return nil
	}
	taskDir := filepath.Join(dir, "tasks", id)
	entries, err := os.ReadDir(taskDir)
	if err != nil {
		return nil
	}
	var tasks []Task
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".json" {
			continue
		}
		data, err := os.ReadFile(filepath.Join(taskDir, e.Name())) //nolint:gosec // trusted local files
		if err != nil {
			continue
		}
		var t Task
		if json.Unmarshal(data, &t) == nil && t.ID != "" {
			tasks = append(tasks, t)
		}
	}
	sort.Slice(tasks, func(i, j int) bool {
		return tasks[i].ID < tasks[j].ID
	})
	return tasks
}

// LoadStatsCache reads the global stats-cache.json.
func LoadStatsCache() *StatsCache {
	dir, err := claudeDir()
	if err != nil {
		return nil
	}
	path := filepath.Join(dir, "stats-cache.json")
	data, err := os.ReadFile(path) //nolint:gosec // trusted local file
	if err != nil {
		return nil
	}
	var sc StatsCache
	if json.Unmarshal(data, &sc) != nil {
		return nil
	}
	return &sc
}

// ApplyEnrichment loads session-meta, facets, and tasks for each session.
func ApplyEnrichment(sessions []Session) {
	for i := range sessions {
		sessions[i].Meta = LoadSessionMeta(sessions[i].ID)
		sessions[i].Facets = LoadSessionFacets(sessions[i].ID)
		sessions[i].Tasks = LoadSessionTasks(sessions[i].ID)
	}
}
