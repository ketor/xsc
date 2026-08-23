package shared

import (
	"strings"

	"github.com/mattn/go-runewidth"
)

// TruncateDisplayWidth 将纯文本限制为单行和指定终端显示宽度。
func TruncateDisplayWidth(value string, width int) string {
	value = strings.NewReplacer("\r", " ", "\n", " ").Replace(value)
	if width <= 0 {
		return ""
	}
	if runewidth.StringWidth(value) <= width {
		return value
	}
	if width <= 3 {
		return truncateRunes(value, width)
	}
	return truncateRunes(value, width-3) + "..."
}

// TruncateDisplayWidthTail 保留文本尾部，适合路径显示。
func TruncateDisplayWidthTail(value string, width int) string {
	value = strings.NewReplacer("\r", " ", "\n", " ").Replace(value)
	if width <= 0 {
		return ""
	}
	if runewidth.StringWidth(value) <= width {
		return value
	}
	if width <= 3 {
		return truncateRunes(value, width)
	}
	targetWidth := width - 3
	runes := []rune(value)
	used := 0
	start := len(runes)
	for start > 0 {
		runeWidth := runewidth.RuneWidth(runes[start-1])
		if used+runeWidth > targetWidth {
			break
		}
		start--
		used += runeWidth
	}
	return "..." + string(runes[start:])
}

func truncateRunes(value string, width int) string {
	var result strings.Builder
	used := 0
	for _, r := range value {
		runeWidth := runewidth.RuneWidth(r)
		if used+runeWidth > width {
			break
		}
		result.WriteRune(r)
		used += runeWidth
	}
	return result.String()
}
