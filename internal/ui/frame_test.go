// internal/ui/frame_test.go
package ui

import "testing"

func TestContentHeight(t *testing.T) {
	tests := []struct {
		name string
		h    int
		want int
	}{
		{"typical 24-row terminal", 24, 18},
		{"tiny terminal clamps to 1", 3, 1},
		{"zero falls back to default height", 0, contentHeight(defaultTermHeight)},
		{"negative falls back to default height", -5, contentHeight(defaultTermHeight)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := contentHeight(tt.h); got != tt.want {
				t.Errorf("contentHeight(%d) = %d, want %d", tt.h, got, tt.want)
			}
		})
	}
}
