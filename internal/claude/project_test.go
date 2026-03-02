package claude

import "testing"

func TestDecodeProjectPath(t *testing.T) {
	tests := []struct {
		encoded string
		want    string
	}{
		{"", ""},
		{"-Users-mfundo-me-tmp", "/Users/mfundo/me/tmp"},
		{"-Users-mfundo-my--project", "/Users/mfundo/my-project"},
		{"-Users-mfundo-a--b--c", "/Users/mfundo/a-b-c"},
		{"-Users-mfundo", "/Users/mfundo"},
		{"noLeadingDash", "noLeadingDash"},
	}
	for _, tt := range tests {
		t.Run(tt.encoded, func(t *testing.T) {
			got := DecodeProjectPath(tt.encoded)
			if got != tt.want {
				t.Errorf("DecodeProjectPath(%q) = %q, want %q", tt.encoded, got, tt.want)
			}
		})
	}
}

func TestShortenPath(t *testing.T) {
	// ShortenPath depends on the current user's home directory, so we just
	// verify it doesn't panic and returns the input for non-home paths.
	got := ShortenPath("/some/other/path")
	if got != "/some/other/path" {
		t.Errorf("ShortenPath(%q) = %q, want unchanged", "/some/other/path", got)
	}
}
