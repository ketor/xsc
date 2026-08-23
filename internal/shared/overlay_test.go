package shared

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
)

func TestOverlayBlockPreservesDimensionsAndBackground(t *testing.T) {
	base := strings.Join([]string{"menu--------", "abcdefghijkl", "mnopqrstuvwx", "status------"}, "\n")
	result := OverlayBlock(base, []string{"XYZ", "123"}, 2, 1, 3, 12)
	if lipgloss.Height(result) != lipgloss.Height(base) {
		t.Fatalf("height changed from %d to %d", lipgloss.Height(base), lipgloss.Height(result))
	}
	lines := strings.Split(result, "\n")
	if lines[1] != "abXYZfghijkl" {
		t.Fatalf("first overlay line = %q", lines[1])
	}
	if lines[2] != "mn123rstuvwx" {
		t.Fatalf("second overlay line = %q", lines[2])
	}
	if lines[3] != "status------" {
		t.Fatalf("uncovered status line changed: %q", lines[3])
	}
	for _, line := range lines {
		if lipgloss.Width(line) != 12 {
			t.Fatalf("line width = %d, want 12: %q", lipgloss.Width(line), line)
		}
	}
}
