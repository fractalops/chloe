package claude

import "testing"

func TestEstimateCost(t *testing.T) {
	tests := []struct {
		name  string
		stats SessionStats
		want  float64 // approximate, check within tolerance
	}{
		{
			name: "haiku default",
			stats: SessionStats{
				Model:        "claude-haiku-4-5",
				InputTokens:  1_000_000,
				OutputTokens: 100_000,
			},
			want: 1.0 + 0.5, // $1/M input + $5/M * 0.1M output
		},
		{
			name: "zero tokens",
			stats: SessionStats{
				Model: "claude-sonnet-4-5",
			},
			want: 0.0,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := estimateCost(&tt.stats)
			diff := got - tt.want
			if diff < -0.01 || diff > 0.01 {
				t.Errorf("estimateCost() = %f, want ~%f", got, tt.want)
			}
		})
	}
}
