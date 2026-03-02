package tui

import (
	"testing"

	"github.com/fractalops/chloe/internal/claude"
)

func TestFilterSessions(t *testing.T) {
	sessions := []claude.Session{
		{ID: "1", Status: "active"},
		{ID: "2", Status: "inactive"},
		{ID: "3", Status: "suspended"},
	}

	t.Run("groupAll", func(t *testing.T) {
		got := filterSessions(sessions, groupAll)
		if len(got) != 3 {
			t.Errorf("groupAll: got %d sessions, want 3", len(got))
		}
	})

	t.Run("groupActiveOnly", func(t *testing.T) {
		got := filterSessions(sessions, groupActiveOnly)
		if len(got) != 2 {
			t.Errorf("groupActiveOnly: got %d sessions, want 2", len(got))
		}
		for _, s := range got {
			if s.Status == "inactive" {
				t.Error("groupActiveOnly should exclude inactive sessions")
			}
		}
	})
}

func TestGroupModeNext(t *testing.T) {
	if groupAll.next() != groupActiveOnly {
		t.Errorf("groupAll.next() = %v, want groupActiveOnly", groupAll.next())
	}
	if groupActiveOnly.next() != groupAll {
		t.Errorf("groupActiveOnly.next() = %v, want groupAll", groupActiveOnly.next())
	}
}
