package shared

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

// OverlayBlock 将 popup 矩形覆盖到 base 的固定行网格中，保留弹窗左右两侧的底层内容。
func OverlayBlock(base string, popup []string, x, y, popupWidth, screenWidth int) string {
	if len(popup) == 0 || popupWidth <= 0 || screenWidth <= 0 {
		return base
	}
	baseLines := strings.Split(base, "\n")
	for row, popupLine := range popup {
		lineIndex := y + row
		if lineIndex < 0 || lineIndex >= len(baseLines) {
			continue
		}
		left := ansi.Cut(baseLines[lineIndex], 0, x)
		right := ansi.Cut(baseLines[lineIndex], x+popupWidth, screenWidth)
		baseLines[lineIndex] = FitANSI(left+popupLine+right, screenWidth, lipgloss.NewStyle())
	}
	return strings.Join(baseLines, "\n")
}

// FitANSI 将 ANSI 样式字符串裁剪或填充到固定显示宽度。
func FitANSI(value string, width int, fill lipgloss.Style) string {
	if width <= 0 {
		return ""
	}
	value = ansi.Truncate(value, width, "")
	padding := width - lipgloss.Width(value)
	if padding > 0 {
		value += fill.Render(strings.Repeat(" ", padding))
	}
	return value
}
