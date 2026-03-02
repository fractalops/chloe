package claude

import (
	"os"
	"path/filepath"
	"strings"
)

// ClaudeProjectsDir returns the path to Claude's projects directory.
func ClaudeProjectsDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".claude", "projects"), nil
}

// DecodeProjectPath converts a Claude project directory name back to a filesystem path.
// Claude Code encodes "/" as "-" and literal "-" as "--".
// e.g., "-Users-mfundo-me-tmp" → "/Users/mfundo/me/tmp"
// e.g., "-Users-mfundo-my--project" → "/Users/mfundo/my-project"
func DecodeProjectPath(encoded string) string {
	if encoded == "" {
		return ""
	}
	// Replace leading dash with /
	if encoded[0] == '-' {
		encoded = "/" + encoded[1:]
	}
	// Use a placeholder so "--" (literal hyphen) doesn't get consumed by single-dash replacement
	const placeholder = "\x00"
	encoded = strings.ReplaceAll(encoded, "--", placeholder)
	encoded = strings.ReplaceAll(encoded, "-", "/")
	encoded = strings.ReplaceAll(encoded, placeholder, "-")
	return encoded
}

// ShortenPath replaces the home directory prefix with ~.
func ShortenPath(path string) string {
	home, err := os.UserHomeDir()
	if err != nil {
		return path
	}
	if strings.HasPrefix(path, home) {
		return "~" + path[len(home):]
	}
	return path
}

// DiscoverProjects returns all project directory entries under ~/.claude/projects/.
func DiscoverProjects(baseDir string) ([]ProjectInfo, error) {
	entries, err := os.ReadDir(baseDir)
	if err != nil {
		return nil, err
	}
	var projects []ProjectInfo
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		name := e.Name()
		projects = append(projects, ProjectInfo{
			Key:  name,
			Path: DecodeProjectPath(name),
			Dir:  filepath.Join(baseDir, name),
		})
	}
	return projects, nil
}

// ProjectInfo holds decoded project metadata.
type ProjectInfo struct {
	Key  string // Raw directory name
	Path string // Decoded filesystem path
	Dir  string // Full path to project directory
}

// FindSessionFiles returns all .jsonl files in a project directory.
func FindSessionFiles(projectDir string) ([]string, error) {
	pattern := filepath.Join(projectDir, "*.jsonl")
	return filepath.Glob(pattern)
}
