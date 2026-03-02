package claude

import "testing"

func TestIsUUID(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		{"550e8400-e29b-41d4-a716-446655440000", true},
		{"ABCDEF00-1234-5678-9ABC-DEF012345678", true},
		{"", false},
		{"not-a-uuid", false},
		{"550e8400-e29b-41d4-a716-44665544000", false},   // too short
		{"550e8400-e29b-41d4-a716-4466554400000", false}, // too long
		{"550e8400xe29b-41d4-a716-446655440000", false},  // wrong separator
		{"550e8400-e29b-41d4-a716-44665544000g", false},  // invalid hex char
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := IsUUID(tt.input)
			if got != tt.want {
				t.Errorf("IsUUID(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestStatusFromStat(t *testing.T) {
	tests := []struct {
		stat string
		want string
	}{
		{"S", "active"},
		{"S+", "active"},
		{"R", "active"},
		{"T", "suspended"},
		{"T+", "suspended"},
	}
	for _, tt := range tests {
		t.Run(tt.stat, func(t *testing.T) {
			got := statusFromStat(tt.stat)
			if got != tt.want {
				t.Errorf("statusFromStat(%q) = %q, want %q", tt.stat, got, tt.want)
			}
		})
	}
}
