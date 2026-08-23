package shared

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// RenderMenuPopup 将菜单渲染为带边框的固定宽度弹窗。
func RenderMenuPopup(menu Menu, cursor int, border, itemStyle, selectedStyle, disabledStyle lipgloss.Style) ([]string, int) {
	innerWidth := 12
	for _, item := range menu.Items {
		width := lipgloss.Width(item.Label) + lipgloss.Width(item.Shortcut) + 4
		innerWidth = max(innerWidth, width)
	}

	lines := make([]string, 0, len(menu.Items)+2)
	lines = append(lines, border.Render("┌"+strings.Repeat("─", innerWidth)+"┐"))
	for index, item := range menu.Items {
		label := " " + item.Label
		shortcut := item.Shortcut
		gap := max(1, innerWidth-lipgloss.Width(label)-lipgloss.Width(shortcut)-1)
		content := label + strings.Repeat(" ", gap) + shortcut + " "
		style := itemStyle
		if item.Disabled {
			style = disabledStyle
		} else if index == cursor {
			style = selectedStyle
		}
		lines = append(lines, border.Render("│")+style.Render(content)+border.Render("│"))
	}
	lines = append(lines, border.Render("└"+strings.Repeat("─", innerWidth)+"┘"))
	return lines, innerWidth + 2
}
