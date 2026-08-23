package shared

import (
	"strings"
	"testing"

	"github.com/mattn/go-runewidth"
)

func TestTruncateDisplayWidth(t *testing.T) {
	result := TruncateDisplayWidth("加载 XShell 会话失败：目录不存在", 16)
	if runewidth.StringWidth(result) > 16 {
		t.Fatalf("display width = %d: %q", runewidth.StringWidth(result), result)
	}
	if !strings.HasSuffix(result, "...") {
		t.Fatalf("truncated text should end with ellipsis: %q", result)
	}
}

func TestTruncateDisplayWidthForcesSingleLine(t *testing.T) {
	result := TruncateDisplayWidth("line one\nline two", 80)
	if strings.ContainsAny(result, "\r\n") {
		t.Fatalf("result contains newline: %q", result)
	}
}

func TestTruncateDisplayWidthTail(t *testing.T) {
	result := TruncateDisplayWidthTail("/根目录/"+strings.Repeat("很长的目录/", 10)+"file.log", 20)
	if runewidth.StringWidth(result) > 20 {
		t.Fatalf("display width = %d: %q", runewidth.StringWidth(result), result)
	}
	if !strings.HasPrefix(result, "...") || !strings.HasSuffix(result, "file.log") {
		t.Fatalf("tail truncation lost path suffix: %q", result)
	}
}
