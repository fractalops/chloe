package claude

import (
	"bufio"
	"os"
	"strings"
)

const maxSearchResults = 50

// SearchSessions scans JSONL content on disk for sessions containing the query string.
// Returns at most maxSearchResults matches.
func SearchSessions(sessions []Session, query string) []Session {
	query = strings.ToLower(query)
	var results []Session
	for _, s := range sessions {
		if searchFileContains(s.FilePath, query) {
			results = append(results, s)
			if len(results) >= maxSearchResults {
				break
			}
		}
	}
	return results
}

// searchFileContains checks if a file contains the query string (case-insensitive).
func searchFileContains(filePath, query string) bool {
	f, err := os.Open(filePath) //nolint:gosec // trusted local session files
	if err != nil {
		return false
	}
	defer f.Close() //nolint:errcheck // read-only file

	scanner := bufio.NewScanner(f)
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 1024*1024)

	for scanner.Scan() {
		if strings.Contains(strings.ToLower(scanner.Text()), query) {
			return true
		}
	}
	return false
}
