package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/ketor/xsc/internal/mobaxterm"
	"github.com/ketor/xsc/internal/securecrt"
	"github.com/ketor/xsc/internal/session"
	"github.com/ketor/xsc/internal/shared"
	internalssh "github.com/ketor/xsc/internal/ssh"
	"github.com/ketor/xsc/internal/xshell"
)

// View 渲染界面
func (m Model) View() string {
	if m.showHelp {
		return m.renderHelp()
	}

	if m.showError {
		errorStyle := lipgloss.NewStyle().
			Foreground(lipgloss.Color("#fb4934")).
			Background(lipgloss.Color("#282828")).
			Padding(1).
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("#fb4934"))
		return errorStyle.Render(m.errorMessage + "\n\nPress any key to continue...")
	}

	if m.width == 0 || m.height == 0 {
		return "Loading..."
	}

	// 计算布局
	treeWidth := m.width * 70 / 100
	detailWidth := m.width * 30 / 100
	contentHeight := m.height - 2 // 留出状态栏空间

	// 计算可见节点一次，传递给各 render 函数
	visibleNodes := m.getVisibleNodes()

	// 构建树形视图
	treeView := m.renderTree(treeWidth, contentHeight, visibleNodes)

	// 构建详情视图
	detailView := m.renderDetail(detailWidth, contentHeight)

	// 合并主内容区
	content := lipgloss.JoinHorizontal(lipgloss.Top, treeView, detailView)

	// 构建状态栏
	statusBar := m.renderStatusBar(visibleNodes)

	// 合并所有内容
	if m.searchMode {
		searchBar := m.renderSearchBar()
		return lipgloss.JoinVertical(lipgloss.Left, content, statusBar, searchBar)
	}

	if m.lineNumMode {
		lineNumBar := m.renderLineNumBar()
		return lipgloss.JoinVertical(lipgloss.Left, content, statusBar, lineNumBar)
	}

	if m.newSessionMode {
		newSessionBar := m.renderNewSessionBar()
		return lipgloss.JoinVertical(lipgloss.Left, content, statusBar, newSessionBar)
	}

	if m.renameMode {
		renameBar := m.renderRenameBar()
		return lipgloss.JoinVertical(lipgloss.Left, content, statusBar, renameBar)
	}

	if m.deleteConfirmMode {
		deleteConfirmBar := m.renderDeleteConfirmBar()
		return lipgloss.JoinVertical(lipgloss.Left, content, statusBar, deleteConfirmBar)
	}

	return lipgloss.JoinVertical(lipgloss.Left, content, statusBar)
}

// computeTreeOffset 计算树形视图的滚动偏移量
func computeTreeOffset(cursor, totalNodes, viewHeight int) int {
	if totalNodes == 0 || viewHeight <= 0 {
		return 0
	}

	startIdx := 0
	if cursor >= viewHeight {
		startIdx = cursor - viewHeight + 1
	}
	if totalNodes > viewHeight && cursor > viewHeight/2 {
		startIdx = min(cursor-viewHeight/2, totalNodes-viewHeight)
	}
	return startIdx
}

// renderTree 渲染树形视图
func (m Model) renderTree(width, height int, visibleNodes []*session.SessionNode) string {
	if m.tree == nil {
		return treeStyle.Width(width).Height(height).Render("Loading sessions...")
	}

	totalNodes := len(visibleNodes)

	if totalNodes == 0 {
		return treeStyle.Width(width).Height(height).Render("No sessions found")
	}

	// 计算滚动的起始位置，确保光标在可视区域内
	startIdx := computeTreeOffset(m.cursor, totalNodes, height)
	endIdx := min(startIdx+height, totalNodes)

	// 计算行号宽度（根据总节点数的位数）
	lineNumWidth := len(fmt.Sprintf("%d", totalNodes))
	if lineNumWidth < 3 {
		lineNumWidth = 3
	}

	var lines []string
	for i := startIdx; i < endIdx; i++ {
		nodeLine := m.renderNode(visibleNodes[i], i == m.cursor)
		// 添加行号前缀
		lineNum := lipgloss.NewStyle().
			Foreground(lipgloss.Color("#665c54")).
			Width(lineNumWidth).
			Align(lipgloss.Right).
			Render(fmt.Sprintf("%d", i+1))
		line := lineNum + " " + nodeLine
		lines = append(lines, line)
	}

	// 填充空行保持高度
	for len(lines) < height {
		lines = append(lines, "")
	}

	content := strings.Join(lines, "\n")
	return treeStyle.Width(width).Height(height).Render(content)
}

// renderNode 渲染单个节点
func (m Model) renderNode(node *session.SessionNode, selected bool) string {
	indent := m.getIndent(node)

	var icon string
	var name string
	isSecureCRT := node.IsSecureCRT()
	isXShell := node.IsXShell()
	isMobaXterm := node.IsMobaXterm()

	if node.IsDir {
		if node.Expanded {
			icon = "▾ "
		} else {
			icon = "▸ "
		}
		// SecureCRT / XShell / MobaXterm 目录使用特殊样式
		if isSecureCRT {
			name = securecrtFolderStyle.Render("[CRT] " + node.Name + "/")
		} else if isXShell {
			name = xshellFolderStyle.Render("[XSH] " + node.Name + "/")
		} else if isMobaXterm {
			name = mobaxtermFolderStyle.Render("[MXT] " + node.Name + "/")
		} else {
			name = folderStyle.Render(node.Name + "/")
		}
	} else {
		// SecureCRT / XShell / MobaXterm 会话使用锁定图标和特殊颜色
		if isSecureCRT || isXShell || isMobaXterm {
			icon = "🔒 "
		} else {
			icon = "  "
		}
		if node.Session != nil && !node.Session.Valid {
			name = invalidStyle.Render(node.Name + " [invalid]")
		} else if isSecureCRT {
			name = securecrtFileStyle.Render(node.Name)
		} else if isXShell {
			name = xshellFileStyle.Render(node.Name)
		} else if isMobaXterm {
			name = mobaxtermFileStyle.Render(node.Name)
		} else {
			name = fileStyle.Render(node.Name)
		}
	}

	line := indent + icon + name

	if selected {
		return selectedStyle.Render(line)
	}
	return line
}

// getIndent 获取节点的缩进
func (m Model) getIndent(node *session.SessionNode) string {
	return shared.GetIndent(node)
}

// renderDetail 渲染详情视图
func (m Model) renderDetail(width, height int) string {
	selected := m.getSelectedNode()
	if selected == nil {
		return detailBoxStyle.
			Width(width - 4).
			Height(height - 2).
			Render("No session selected")
	}

	if selected.IsDir {
		content := fmt.Sprintf("Folder: %s\n\nContains %d items",
			selected.Name, len(selected.Children))
		return detailBoxStyle.
			Width(width - 4).
			Height(height - 2).
			Render(content)
	}

	if selected.Session == nil {
		return detailBoxStyle.
			Width(width - 4).
			Height(height - 2).
			Render("No session data")
	}

	s := selected.Session
	var content strings.Builder

	// 标题 - 显示节点文件名（不含后缀）
	content.WriteString(detailTitleStyle.Render(selected.Name))
	content.WriteString("\n\n")

	// 配置详情
	content.WriteString(detailKeyStyle.Render("Host: "))
	content.WriteString(detailValueStyle.Render(s.Host) + "\n\n")

	content.WriteString(detailKeyStyle.Render("Port: "))
	content.WriteString(detailValueStyle.Render(fmt.Sprintf("%d", s.Port)) + "\n\n")

	content.WriteString(detailKeyStyle.Render("User: "))
	content.WriteString(detailValueStyle.Render(s.User) + "\n\n")

	// 显示认证方式列表
	content.WriteString(detailKeyStyle.Render("Auth Methods:\n"))
	content.WriteString("\n")
	var authLines []string

	if len(s.AuthMethods) > 0 {
		// 显示多种认证方式（SecureCRT 风格）
		for i, am := range s.AuthMethods {
			authIcon := m.getAuthIcon(am.Type)
			authTypeStr := m.formatAuthType(am.Type)

			// 添加详细信息
			var detail string
			switch am.Type {
			case "password":
				if am.EncryptedPassword != "" {
					// 有加密密码，根据 showPassword 决定是否解密显示
					if m.showPassword {
						// 根据密码来源选择解密器
						var decrypted string
						var err error
						switch s.PasswordSource {
						case "xshell":
							decrypted, err = xshell.DecryptPassword(am.EncryptedPassword, s.MasterPassword)
						case "mobaxterm":
							decrypted, err = mobaxterm.DecryptPassword(am.EncryptedPassword, s.MasterPassword)
						default:
							decrypted, err = securecrt.DecryptPassword(am.EncryptedPassword, s.MasterPassword)
						}
						if err == nil {
							detail = fmt.Sprintf(" (%s)", decrypted)
						} else {
							detail = fmt.Sprintf(" (decrypt failed: %v)", err)
						}
					} else {
						detail = " (encrypted)"
					}
				} else if am.Password != "" {
					// 已有明文密码
					if m.showPassword {
						detail = fmt.Sprintf(" (%s)", am.Password)
					} else {
						detail = " (********)"
					}
				}
			case "key", "publickey":
				if am.KeyPath != "" {
					detail = fmt.Sprintf(" (%s)", am.KeyPath)
				} else {
					detail = " (global)"
				}
			}

			// 构建行内容 - 简单格式，避免emoji宽度问题
			// 格式: 序号. + 空格 + 图标 + 空格 + 类型 + 详情
			line := fmt.Sprintf("%d. %s %s", i+1, authIcon, authTypeStr)
			if detail != "" {
				line += detail
			}
			authLines = append(authLines, line)
		}
	} else {
		// 显示单一认证方式（原生 XSSH 风格）
		authTypeStr := m.formatAuthType(string(s.AuthType))
		authIcon := m.getAuthIcon(string(s.AuthType))
		var detail string

		// 根据认证类型显示详细信息
		switch s.AuthType {
		case session.AuthTypePassword:
			if s.Password != "" {
				if m.showPassword {
					detail = fmt.Sprintf(" (%s)", s.Password)
				} else {
					detail = " (********)"
				}
			} else if s.EncryptedPassword != "" {
				if m.showPassword {
					// 仅在显示密码时才解密
					if err := s.ResolvePassword(); err == nil {
						detail = fmt.Sprintf(" (%s)", s.Password)
					} else {
						detail = fmt.Sprintf(" (decrypt failed: %v)", err)
					}
				} else {
					detail = " (********)"
				}
			}
		case session.AuthTypeKey:
			if s.KeyPath != "" {
				detail = fmt.Sprintf(" (%s)", s.KeyPath)
			} else {
				detail = " (global)"
			}
		}

		// 构建行内容 - 简单格式，避免emoji宽度问题
		// 格式: 序号. + 空格 + 图标 + 空格 + 类型 + 详情
		line := fmt.Sprintf("1. %s %s", authIcon, authTypeStr)
		if detail != "" {
			line += detail
		}
		authLines = append(authLines, line)
	}

	// 统一渲染所有行 - 使用 lipgloss.JoinVertical 确保对齐
	if len(authLines) > 0 {
		authContent := lipgloss.JoinVertical(lipgloss.Left, authLines...)
		content.WriteString(authContent)
		content.WriteString("\n\n")
	}

	// 显示 SSH Agent keys（如果是 Agent 认证）
	if s.AuthType == session.AuthTypeAgent {
		content.WriteString(detailKeyStyle.Render("SSH Agent Keys:\n"))
		content.WriteString("\n")
		// 使用缓存的 SSH Agent keys（在 Init/Update 中加载）
		var keys []internalssh.AgentKeyInfo
		var err error
		if m.agentKeyCache != nil {
			keys = m.agentKeyCache.keys
			err = m.agentKeyCache.err
		}
		if err != nil {
			content.WriteString(invalidStyle.Render("  "+err.Error()) + "\n\n")
		} else if len(keys) == 0 {
			content.WriteString(detailValueStyle.Render("  (no keys loaded)") + "\n\n")
		} else {
			for _, k := range keys {
				comment := k.Comment
				if comment == "" {
					comment = "(no comment)"
				}
				content.WriteString(detailValueStyle.Render(
					fmt.Sprintf("  %s %s", k.Type, comment)) + "\n")
			}
			content.WriteString("\n")
		}
	}

	if s.Description != "" {
		content.WriteString(detailKeyStyle.Render("Description:\n"))
		content.WriteString("\n")
		content.WriteString(s.Description + "\n\n")
	}

	if !s.Valid {
		content.WriteString(invalidStyle.Render("Error: " + s.Error.Error()))
	}

	// 应用边框样式
	return detailBoxStyle.
		Width(width - 4).   // 减去边框和padding的宽度
		Height(height - 2). // 减去边框的高度
		Render(content.String())
}

// getAuthIcon 返回认证类型的图标
func (m Model) getAuthIcon(authType string) string {
	switch authType {
	case "password":
		return "🔑"
	case "key", "publickey":
		return "🔐"
	case "agent":
		return "🤖"
	case "keyboard-interactive":
		return "⌨️"
	case "gssapi":
		return "🎫"
	default:
		return "🔓"
	}
}

// formatAuthType 格式化认证类型显示名称
func (m Model) formatAuthType(authType string) string {
	switch authType {
	case "password":
		return "Password"
	case "key", "publickey":
		return "Public Key"
	case "agent":
		return "SSH Agent"
	case "keyboard-interactive":
		return "Keyboard Interactive"
	case "gssapi":
		return "GSSAPI"
	default:
		return authType
	}
}

// renderStatusBar 渲染状态栏
func (m Model) renderStatusBar(visibleNodes []*session.SessionNode) string {
	var status strings.Builder

	if m.searchMode {
		status.WriteString("Search mode | ")
	}

	selected := m.getSelectedNode()
	if selected != nil && !selected.IsDir {
		status.WriteString(fmt.Sprintf("Session: %s | ", selected.Name))
	}

	// 显示搜索状态
	if m.searchQuery != "" {
		status.WriteString(fmt.Sprintf("Filter: '%s' (%d) | ", m.searchQuery, len(visibleNodes)))
		status.WriteString("Esc:clear Enter:confirm | ")
	} else {
		status.WriteString(fmt.Sprintf("Total: %d | ", len(visibleNodes)))
	}
	if m.showPassword {
		status.WriteString("[PW] ")
	}
	status.WriteString("Press ? for help, :q or Ctrl+c to quit")

	return statusBarStyle.Width(m.width).Render(status.String())
}

// renderSearchBar 渲染搜索栏
func (m Model) renderSearchBar() string {
	// 添加退出提示到搜索栏
	searchWithHint := m.searchInput.View() + "  (Esc:clear Enter:confirm)"
	return searchStyle.Width(m.width).Render(searchWithHint)
}

// renderLineNumBar 渲染行号跳转栏（带命令补全提示）
func (m Model) renderLineNumBar() string {
	input := m.lineNumInput.Value()
	completions := getCommandCompletions(input)

	var hints []string
	for i, cmd := range completions {
		hint := fmt.Sprintf(":%s - %s", cmd.Name, cmd.Description)
		if i == 0 {
			hints = append(hints, cmdHintActiveStyle.Render(hint))
		} else {
			hints = append(hints, cmdHintStyle.Render(hint))
		}
	}

	bar := m.lineNumInput.View()
	if len(hints) > 0 {
		bar += "  " + strings.Join(hints, "  ")
	}
	bar += "  " + cmdHintStyle.Render("(Tab:补全 Enter:执行 Esc:取消)")

	return searchStyle.Width(m.width).Render(bar)
}

// renderNewSessionBar 渲染新建会话文件名输入栏
func (m Model) renderNewSessionBar() string {
	hint := cmdHintStyle.Render("(Enter:确认 Esc:取消)")
	bar := m.newSessionInput.View() + "  " + hint
	return searchStyle.Width(m.width).Render(bar)
}

// renderRenameBar 渲染重命名会话文件名输入栏
func (m Model) renderRenameBar() string {
	hint := cmdHintStyle.Render("(Enter:确认 Esc:取消)")
	bar := m.renameInput.View() + "  " + hint
	return searchStyle.Width(m.width).Render(bar)
}

// renderDeleteConfirmBar 渲染删除确认栏
func (m Model) renderDeleteConfirmBar() string {
	warningStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#fb4934")).
		Bold(true)

	warning := warningStyle.Render("⚠️  Warning: This action cannot be undone!")
	bar := warning + "  " + m.deleteConfirmInput.View()
	return searchStyle.Width(m.width).Render(bar)
}

// renderHelp 渲染自定义帮助视图
func (m Model) renderHelp() string {
	var b strings.Builder

	renderSection := func(title string, items [][2]string) {
		b.WriteString(helpSectionStyle.Render(title))
		b.WriteString("\n")
		for _, item := range items {
			b.WriteString("  ")
			b.WriteString(helpKeyStyle.Render(item[0]))
			b.WriteString(helpDescStyle.Render(item[1]))
			b.WriteString("\n")
		}
		b.WriteString("\n")
	}

	renderSection("移动", [][2]string{
		{"↑/k, ↓/j", "上下移动"},
		{"PgUp/C-b", "向上翻页"},
		{"PgDn/C-f", "向下翻页"},
		{"C-u, C-d", "向上/下半页"},
		{"gg", "跳转到顶部"},
		{"G", "跳转到底部"},
		{"<n>G, :<n>", "跳转到第 n 行"},
		{"0", "跳转到第一行"},
		{"$", "跳转到最后一行"},
		{"^", "跳转到第一个会话"},
	})

	renderSection("折叠", [][2]string{
		{"Space/o", "展开/折叠目录"},
		{"h/←", "折叠目录或跳到父目录"},
		{"l/→", "展开目录"},
		{"E", "展开所有目录"},
		{"C", "折叠所有目录"},
	})

	renderSection("搜索", [][2]string{
		{"/", "进入搜索模式"},
		{"Enter", "确认搜索"},
		{"Esc", "取消搜索并清除过滤"},
		{"Ctrl+c", "退出搜索并保留过滤"},
		{"n/N", "下一个/上一个匹配"},
	})

	renderSection("会话操作", [][2]string{
		{"Enter", "连接到选中会话"},
		{"e", "编辑会话配置"},
		{"n", "新建会话"},
		{"D", "删除会话 (输入 YES 确认)"},
		{"c", "重命名会话"},
	})

	// 从命令注册表自动生成命令部分
	cmdItems := make([][2]string, len(commands))
	for i, cmd := range commands {
		aliases := strings.Join(cmd.Aliases, "/")
		cmdItems[i] = [2]string{
			fmt.Sprintf(":%s/:%s", cmd.Name, aliases),
			cmd.Description,
		}
	}
	renderSection("命令 (: 模式)", cmdItems)

	renderSection("其他", [][2]string{
		{"?", "显示/关闭帮助"},
		{"Ctrl+c/:q", "退出程序"},
	})

	return helpContainerStyle.Render(b.String())
}
