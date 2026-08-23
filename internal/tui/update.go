package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/ketor/xsc/internal/shared"
)

// Update 处理消息
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.detailView.Width = m.width * 30 / 100
		m.detailView.Height = m.height - 3
		return m, nil

	case agentKeysLoadedMsg:
		m.agentKeyCache = &AgentKeyCache{
			keys: msg.keys,
			err:  msg.err,
		}
		return m, nil

	case tea.MouseMsg:
		return m.handleMouse(msg)

	case tea.KeyMsg:
		if msg.Type == tea.KeyF10 {
			m.menu.OpenMenu(0, topMenus)
			return m, nil
		}
		if m.menu.Open {
			return m.handleTopMenuKey(msg)
		}
		// 右键菜单打开时，处理菜单键盘操作
		if m.contextMenu.visible {
			return m.handleContextMenuKey(msg)
		}

		// 如果显示错误信息，按任意键关闭
		if m.showError {
			m.showError = false
			m.errorMessage = ""
			return m, nil
		}

		// 如果显示帮助，按任意键关闭帮助（除了 q/Ctrl+c 仍然退出）
		if m.showHelp {
			if key.Matches(msg, m.keys.Quit) {
				return m, tea.Quit
			}
			m.showHelp = false
			return m, nil
		}

		if m.searchMode {
			// 直接处理 ESC 键，避免被 textinput 拦截
			if msg.Type == tea.KeyEsc {
				m.searchMode = false
				m.searchQuery = ""
				m.searchInput.SetValue("")
				m.cursor = 0
				return m, nil
			}
			return m.handleSearchInput(msg)
		}

		if m.lineNumMode {
			return m.handleLineNumInput(msg)
		}

		if m.newSessionMode {
			return m.handleNewSessionInput(msg)
		}

		if m.renameMode {
			return m.handleRenameInput(msg)
		}

		if m.deleteConfirmMode {
			return m.handleDeleteConfirmInput(msg)
		}

		// 普通模式下，Esc 清空搜索过滤（如果有过滤条件）
		if msg.Type == tea.KeyEsc && m.searchQuery != "" {
			m.searchQuery = ""
			m.searchInput.SetValue("")
			m.cursor = 0
			return m, nil
		}

		// 处理键盘输入
		// 统一重置 lineNumBuffer 和 lastKeyG（仅数字键和 g 键在各自分支中恢复）
		savedLineNumBuffer := m.lineNumBuffer
		savedLastKeyG := m.lastKeyG
		m.lineNumBuffer = ""
		m.lastKeyG = false

		switch {
		case key.Matches(msg, m.keys.Quit):
			return m, tea.Quit

		case key.Matches(msg, m.keys.Help):
			m.showHelp = !m.showHelp
			return m, nil

		case key.Matches(msg, m.keys.Up):
			m.moveCursor(-1)
			return m, nil

		case key.Matches(msg, m.keys.Down):
			m.moveCursor(1)
			return m, nil

		case key.Matches(msg, m.keys.PageUp):
			m.moveCursor(-(m.height - 3))
			return m, nil

		case key.Matches(msg, m.keys.PageDown):
			m.moveCursor(m.height - 3)
			return m, nil

		case key.Matches(msg, m.keys.HalfPageUp):
			m.moveCursor(-((m.height - 3) / 2))
			return m, nil

		case key.Matches(msg, m.keys.HalfPageDown):
			m.moveCursor((m.height - 3) / 2)
			return m, nil

		// Vim: gg - 跳转到顶部（或者 g 后面跟 g）
		case msg.String() == "g":
			// 检测是否是 'gg' 组合
			if savedLastKeyG {
				m.cursor = 0
				return m, nil
			}
			m.lastKeyG = true // 恢复：等待下一个 g
			return m, nil

		// Vim: G - 跳转到底部，或者数字+G跳转到指定行
		case msg.String() == "G":
			visibleNodes := m.getVisibleNodes()
			if savedLineNumBuffer != "" {
				// 如果有累积的数字，跳转到指定行
				var lineNum int
				fmt.Sscanf(savedLineNumBuffer, "%d", &lineNum)
				if lineNum > 0 && len(visibleNodes) > 0 {
					m.cursor = lineNum - 1
					if m.cursor >= len(visibleNodes) {
						m.cursor = len(visibleNodes) - 1
					}
					if m.cursor < 0 {
						m.cursor = 0
					}
				}
			} else {
				// 没有数字，跳转到底部
				if len(visibleNodes) > 0 {
					m.cursor = len(visibleNodes) - 1
				}
			}
			return m, nil

		// Vim: 0 - 跳转到行首（对于列表，跳到顶部）
		case msg.String() == "0":
			m.cursor = 0
			return m, nil

		// Vim: $ - 跳转到行尾（对于列表，跳到底部）
		case msg.String() == "$":
			visibleNodes := m.getVisibleNodes()
			if len(visibleNodes) > 0 {
				m.cursor = len(visibleNodes) - 1
			}
			return m, nil

		// Vim: ^ - 跳转到第一个非空字符（对于树形列表，跳到第一个文件）
		case msg.String() == "^":
			visibleNodes := m.getVisibleNodes()
			for i, node := range visibleNodes {
				if !node.IsDir {
					m.cursor = i
					break
				}
			}
			return m, nil

		// Vim: n - 有搜索时查找下一个，无搜索时新建会话
		case msg.String() == "n":
			if m.searchQuery != "" {
				m.searchNext(1)
				return m, nil
			}
			return m, m.prepareNewSession()

		// Vim: N - 查找上一个
		case msg.String() == "N":
			if m.searchQuery != "" {
				m.searchNext(-1)
			}
			return m, nil

		// Vim: : - 进入行号跳转模式
		case msg.String() == ":":
			m.lineNumMode = true
			m.lineNumInput.SetValue("")
			m.lineNumInput.Focus()
			return m, textinput.Blink

		// 数字键 - 可能是在输入行号
		case len(msg.String()) == 1 && msg.String()[0] >= '1' && msg.String()[0] <= '9':
			// 恢复并累积数字
			m.lineNumBuffer = savedLineNumBuffer + msg.String()
			return m, nil

		case key.Matches(msg, m.keys.GoToTop):
			m.cursor = 0
			return m, nil

		case key.Matches(msg, m.keys.GoToBottom):
			visibleNodes := m.getVisibleNodes()
			if len(visibleNodes) > 0 {
				m.cursor = len(visibleNodes) - 1
			}
			return m, nil

		case key.Matches(msg, m.keys.Space):
			selected := m.getSelectedNode()
			if selected != nil && selected.IsDir {
				selected.Expanded = !selected.Expanded
			}
			return m, nil

		case key.Matches(msg, m.keys.Enter):
			selected := m.getSelectedNode()
			if selected != nil && !selected.IsDir && selected.Session != nil && selected.Session.Valid {
				// 使用 execCommand 执行外部命令，确保完全退出 TUI 后再连接
				return m, m.execSSHCommand(selected.Session)
			}
			return m, nil

		case key.Matches(msg, m.keys.Search):
			m.searchMode = true
			m.searchInput.Focus()
			return m, textinput.Blink

		case key.Matches(msg, m.keys.Edit):
			selected := m.getSelectedNode()
			if selected == nil {
				m.errorMessage = "No item selected"
				m.showError = true
			} else if selected.IsDir {
				m.errorMessage = "Cannot edit a directory"
				m.showError = true
			} else if selected.IsReadOnly() {
				m.errorMessage = "Cannot edit imported session (read-only)"
				m.showError = true
			} else if selected.Session == nil {
				m.errorMessage = "No session data available"
				m.showError = true
			} else if selected.Session.FilePath == "" {
				m.errorMessage = "Session file path is empty"
				m.showError = true
			} else {
				return m, m.execEditCommand(selected.Session)
			}
			return m, nil

		case key.Matches(msg, m.keys.Delete):
			selected := m.getSelectedNode()
			if selected == nil {
				m.errorMessage = "No item selected"
				m.showError = true
			} else if selected.IsDir {
				m.errorMessage = "Cannot delete a directory"
				m.showError = true
			} else if selected.IsReadOnly() {
				m.errorMessage = "Cannot delete imported session (read-only)"
				m.showError = true
			} else if selected.Session == nil {
				m.errorMessage = "No session data available"
				m.showError = true
			} else {
				return m, m.prepareDeleteConfirm(selected)
			}
			return m, nil

		case key.Matches(msg, m.keys.Rename):
			selected := m.getSelectedNode()
			if selected == nil {
				m.errorMessage = "No item selected"
				m.showError = true
			} else if selected.IsDir {
				m.errorMessage = "Cannot rename a directory"
				m.showError = true
			} else if selected.IsReadOnly() {
				m.errorMessage = "Cannot rename imported session (read-only)"
				m.showError = true
			} else if selected.Session == nil {
				m.errorMessage = "No session data available"
				m.showError = true
			} else if selected.Session.FilePath == "" {
				m.errorMessage = "Session file path is empty"
				m.showError = true
			} else {
				return m, m.prepareRenameSession(selected)
			}
			return m, nil

		// Vim: o - Toggle fold (展开/折叠当前目录)
		case msg.String() == "o":
			selected := m.getSelectedNode()
			if selected != nil && selected.IsDir {
				selected.Expanded = !selected.Expanded
			}
			return m, nil

		// Vim: h/← - 折叠当前目录（如果已展开）或跳到父目录
		case key.Matches(msg, m.keys.CloseFold):
			selected := m.getSelectedNode()
			if selected != nil {
				if selected.IsDir && selected.Expanded {
					selected.Expanded = false
				} else if selected.Parent != nil {
					// 查找父目录在可见列表中的位置
					visibleNodes := m.getVisibleNodes()
					for i, node := range visibleNodes {
						if node == selected.Parent {
							m.cursor = i
							break
						}
					}
				}
			}
			return m, nil

		// Vim: l/→ - 展开当前目录（如果已折叠）
		case key.Matches(msg, m.keys.OpenFold):
			selected := m.getSelectedNode()
			if selected != nil && selected.IsDir && !selected.Expanded {
				selected.Expanded = true
			}
			return m, nil

		// Vim: E - 展开所有目录
		case key.Matches(msg, m.keys.OpenAllFolds):
			if m.tree != nil {
				m.expandAll(m.tree)
			}
			return m, nil

		// Vim: C - 折叠所有目录
		case key.Matches(msg, m.keys.CloseAllFolds):
			if m.tree != nil {
				m.collapseAll(m.tree)
			}
			return m, nil
		}

	case sessionsLoadedMsg:
		m.tree = msg.tree
		m.sessionsDir = msg.sessionsDir
		if m.tree != nil {
			m.tree.SetParent(nil)
			// 默认展开所有目录
			m.expandAll(m.tree)
		}
		if msg.err != nil {
			if msg.tree == nil {
				m.errorMessage = msg.err.Error()
				m.showError = true
			} else {
				m.loadWarning = msg.err.Error()
			}
		} else {
			m.loadWarning = ""
		}
		return m, func() tea.Msg {
			// 触发一次刷新以确保界面正确渲染
			if m.width > 0 && m.height > 0 {
				return tea.WindowSizeMsg{Width: m.width, Height: m.height}
			}
			return nil
		}

	case connectCompleteMsg:
		// SSH 连接完成，自动返回 TUI
		cmds := []tea.Cmd{
			tea.EnterAltScreen,
			func() tea.Msg {
				return tea.WindowSizeMsg{Width: m.width, Height: m.height}
			},
		}
		if msg.err != nil {
			cmds = append(cmds, func() tea.Msg {
				return showErrorMsg{err: msg.err}
			})
		}
		return m, tea.Batch(cmds...)

	case showErrorMsg:
		// 在TUI界面中显示错误信息
		m.errorMessage = fmt.Sprintf("Connection failed: %v", msg.err)
		m.showError = true
		return m, nil

	case editorCompleteMsg:
		// 编辑器关闭，重新加载会话
		return m, tea.Batch(
			tea.EnterAltScreen,
			m.loadSessions(),
			func() tea.Msg {
				return tea.WindowSizeMsg{Width: m.width, Height: m.height}
			},
		)

	case newSessionEditorCompleteMsg:
		// 新建会话编辑器关闭，处理结果并重新加载
		return m, tea.Batch(
			m.handleNewSessionComplete(msg),
			tea.EnterAltScreen,
			func() tea.Msg {
				return tea.WindowSizeMsg{Width: m.width, Height: m.height}
			},
		)

	case prepareNewSessionMsg:
		// 进入新建会话模式
		m.newSessionMode = true
		m.newSessionDir = msg.dir
		m.newSessionInput.SetValue("")
		m.newSessionInput.Focus()
		return m, textinput.Blink

	case prepareRenameSessionMsg:
		// 进入重命名会话模式
		if msg.node != nil && msg.node.Session != nil {
			m.renameMode = true
			m.renameTargetNode = msg.node
			// 预设当前文件名（不含扩展名）
			currentName := msg.node.Name
			m.renameInput.SetValue(currentName)
			m.renameInput.Focus()
			return m, textinput.Blink
		}
		return m, nil

	case prepareDeleteConfirmMsg:
		// 进入删除确认模式
		if msg.node != nil && msg.node.Session != nil {
			m.deleteConfirmMode = true
			m.deleteTargetNode = msg.node
			m.deleteConfirmInput.SetValue("")
			m.deleteConfirmInput.Focus()
			return m, textinput.Blink
		}
		return m, nil
	}

	var cmd tea.Cmd
	m.detailView, cmd = m.detailView.Update(msg)
	return m, cmd
}

// handleSearchInput 处理搜索输入
func (m Model) handleSearchInput(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEsc:
		// ESC: 取消搜索，清空过滤条件
		m.searchMode = false
		m.searchQuery = ""
		m.searchInput.SetValue("")
		// 重置光标到顶部，避免光标位置超出新的可见节点范围
		m.cursor = 0
		return m, nil

	case tea.KeyCtrlC:
		// Ctrl+c: 取消搜索但保留过滤结果（仅退出输入模式）
		m.searchMode = false
		m.searchQuery = m.searchInput.Value()
		return m, nil

	case tea.KeyEnter:
		// Enter: 确认搜索
		m.searchMode = false
		m.searchQuery = m.searchInput.Value()
		return m, nil

	case tea.KeyCtrlU:
		// Ctrl+u: 清空当前输入（Vim 风格）
		m.searchInput.SetValue("")
		m.searchQuery = ""
		return m, nil

	default:
		var cmd tea.Cmd
		m.searchInput, cmd = m.searchInput.Update(msg)
		m.searchQuery = m.searchInput.Value()
		return m, cmd
	}
}

// handleLineNumInput 处理行号输入
func (m Model) handleLineNumInput(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEsc, tea.KeyCtrlC:
		m.lineNumMode = false
		m.lineNumBuffer = ""
		m.lineNumInput.SetValue("")
		return m, nil

	case tea.KeyTab:
		// Tab 自动补全：匹配第一个命令
		input := m.lineNumInput.Value()
		completions := getCommandCompletions(input)
		if len(completions) > 0 {
			m.lineNumInput.SetValue(completions[0].Name)
			m.lineNumInput.CursorEnd()
		}
		return m, nil

	case tea.KeyEnter:
		m.lineNumMode = false
		cmdStr := m.lineNumInput.Value()
		if cmdStr == "" {
			cmdStr = m.lineNumBuffer
		}

		// 通过命令注册表匹配命令
		switch matchCommand(cmdStr) {
		case "q":
			return m, tea.Quit
		case "noh":
			m.searchQuery = ""
			m.searchInput.SetValue("")
			m.lineNumBuffer = ""
			m.lineNumInput.SetValue("")
			return m, nil
		case "pw":
			m.showPassword = !m.showPassword
			m.lineNumBuffer = ""
			m.lineNumInput.SetValue("")
			return m, nil
		}

		// 未匹配命令，尝试解析行号并跳转
		if cmdStr != "" {
			var lineNum int
			fmt.Sscanf(cmdStr, "%d", &lineNum)
			if lineNum > 0 {
				m.cursor = lineNum - 1
				visibleNodes := m.getVisibleNodes()
				if m.cursor >= len(visibleNodes) {
					m.cursor = len(visibleNodes) - 1
				}
				if m.cursor < 0 {
					m.cursor = 0
				}
			}
		}
		m.lineNumBuffer = ""
		m.lineNumInput.SetValue("")
		return m, nil

	default:
		var cmd tea.Cmd
		m.lineNumInput, cmd = m.lineNumInput.Update(msg)
		return m, cmd
	}
}

// handleNewSessionInput 处理新建会话的文件名输入
func (m Model) handleNewSessionInput(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEsc:
		// 取消新建会话
		m.newSessionMode = false
		m.newSessionInput.SetValue("")
		m.newSessionDir = ""
		return m, nil

	case tea.KeyEnter:
		// 确认文件名，开始创建会话
		filename := m.newSessionInput.Value()
		if filename == "" {
			m.errorMessage = "Filename cannot be empty"
			m.showError = true
			m.newSessionMode = false
			m.newSessionInput.SetValue("")
			return m, nil
		}

		// 确保文件名有.yaml后缀
		if !strings.HasSuffix(filename, ".yaml") && !strings.HasSuffix(filename, ".yml") {
			filename = filename + ".yaml"
		}

		m.newSessionMode = false
		m.newSessionInput.SetValue("")
		return m, m.createNewSession(m.newSessionDir, filename)

	default:
		var cmd tea.Cmd
		m.newSessionInput, cmd = m.newSessionInput.Update(msg)
		return m, cmd
	}
}

// handleRenameInput 处理重命名会话的文件名输入
func (m Model) handleRenameInput(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEsc:
		// 取消重命名
		m.renameMode = false
		m.renameInput.SetValue("")
		m.renameTargetNode = nil
		return m, nil

	case tea.KeyEnter:
		// 确认新文件名
		newName := m.renameInput.Value()
		if newName == "" {
			m.errorMessage = "Filename cannot be empty"
			m.showError = true
			m.renameMode = false
			m.renameInput.SetValue("")
			m.renameTargetNode = nil
			return m, nil
		}

		// 确保文件名有.yaml后缀
		if !strings.HasSuffix(newName, ".yaml") && !strings.HasSuffix(newName, ".yml") {
			newName = newName + ".yaml"
		}

		node := m.renameTargetNode
		m.renameMode = false
		m.renameInput.SetValue("")
		m.renameTargetNode = nil

		if node != nil && node.Session != nil {
			return m, m.renameSession(node, newName)
		}
		return m, nil

	default:
		var cmd tea.Cmd
		m.renameInput, cmd = m.renameInput.Update(msg)
		return m, cmd
	}
}

// handleDeleteConfirmInput 处理删除确认的输入
func (m Model) handleDeleteConfirmInput(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEsc:
		// 取消删除
		m.deleteConfirmMode = false
		m.deleteConfirmInput.SetValue("")
		m.deleteTargetNode = nil
		return m, nil

	case tea.KeyEnter:
		// 检查确认输入
		confirmation := m.deleteConfirmInput.Value()
		if confirmation != "YES" {
			m.deleteConfirmMode = false
			m.deleteConfirmInput.SetValue("")
			m.deleteTargetNode = nil
			return m, nil
		}

		// 确认删除
		node := m.deleteTargetNode
		m.deleteConfirmMode = false
		m.deleteConfirmInput.SetValue("")
		m.deleteTargetNode = nil

		if node != nil {
			return m, m.deleteSession(node)
		}
		return m, nil

	default:
		var cmd tea.Cmd
		m.deleteConfirmInput, cmd = m.deleteConfirmInput.Update(msg)
		return m, cmd
	}
}

func (m Model) handleTopMenuKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEsc, tea.KeyF10:
		m.menu.Close()
		return m, nil
	case tea.KeyLeft:
		m.menu.MoveMenu(-1, topMenus)
		return m, nil
	case tea.KeyRight:
		m.menu.MoveMenu(1, topMenus)
		return m, nil
	case tea.KeyUp:
		m.menu.MoveItem(-1, topMenus)
		return m, nil
	case tea.KeyDown:
		m.menu.MoveItem(1, topMenus)
		return m, nil
	case tea.KeyEnter:
		item, ok := m.menu.Selected(topMenus)
		m.menu.Close()
		if !ok {
			return m, nil
		}
		return m.executeTopMenuAction(item.Action)
	}
	return m, nil
}

func (m Model) executeTopMenuAction(action string) (tea.Model, tea.Cmd) {
	switch action {
	case "quit":
		return m, tea.Quit
	case "reload":
		return m, m.loadSessions()
	case "password":
		m.showPassword = !m.showPassword
		return m, nil
	case "expand-all":
		if m.tree != nil {
			m.expandAll(m.tree)
		}
		return m, nil
	case "collapse-all":
		if m.tree != nil {
			m.collapseAll(m.tree)
		}
		return m, nil
	case "new":
		return m, m.prepareNewSession()
	case "connect":
		return m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	case "edit":
		return m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'e'}})
	case "rename":
		return m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'c'}})
	case "delete":
		return m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'D'}})
	case "search":
		return m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	case "help":
		return m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'?'}})
	}
	return m, nil
}

func (m Model) handleTopMenuMouse(msg tea.MouseMsg) (Model, tea.Cmd, bool) {
	if msg.Y == 0 {
		index := shared.MenuIndexAtX(topMenus, msg.X)
		if index >= 0 {
			if msg.Button == tea.MouseButtonLeft && msg.Action == tea.MouseActionPress {
				if m.menu.Open && m.menu.Active == index {
					m.menu.Close()
				} else {
					m.menu.OpenMenu(index, topMenus)
				}
				m.contextMenu.visible = false
				return m, nil, true
			}
			if m.menu.Open {
				m.menu.OpenMenu(index, topMenus)
				return m, nil, true
			}
		}
	}
	if !m.menu.Open {
		return m, nil, false
	}

	items := topMenus[m.menu.Active].Items
	startX := shared.MenuStartX(topMenus, m.menu.Active)
	width := 0
	for _, item := range items {
		width = max(width, len([]rune(item.Label))+len([]rune(item.Shortcut))+5)
	}
	if msg.Y >= 1 && msg.Y <= len(items) && msg.X >= startX && msg.X < startX+width {
		m.menu.Cursor = msg.Y - 1
		if msg.Button == tea.MouseButtonLeft && msg.Action == tea.MouseActionPress {
			item, ok := m.menu.Selected(topMenus)
			m.menu.Close()
			if !ok {
				return m, nil, true
			}
			next, cmd := m.executeTopMenuAction(item.Action)
			return next.(Model), cmd, true
		}
		return m, nil, true
	}
	if msg.Button == tea.MouseButtonLeft && msg.Action == tea.MouseActionPress {
		m.menu.Close()
		return m, nil, true
	}
	return m, nil, true
}

// handleMouse 处理鼠标事件
func (m Model) handleMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	if next, cmd, handled := m.handleTopMenuMouse(msg); handled {
		return next, cmd
	}
	// 右键菜单打开时支持鼠标直接点击菜单项。
	if m.contextMenu.visible {
		if msg.Button == tea.MouseButtonLeft && msg.Action == tea.MouseActionPress {
			if msg.Y == m.height-1 {
				x := 0
				if m.contextMenu.node != nil {
					x = len([]rune(m.contextMenu.node.Name)) + 4
				}
				for index, item := range m.contextMenu.items {
					width := len([]rune(item.Label)) + len([]rune(item.Key)) + 5
					if msg.X >= x && msg.X < x+width {
						m.contextMenu.cursor = index
						return m.executeContextMenuAction()
					}
					x += width + 1
				}
			}
			m.contextMenu.visible = false
			return m, nil
		}
		if msg.Button == tea.MouseButtonRight && msg.Action == tea.MouseActionPress {
			m.contextMenu.visible = false
		} else {
			return m, nil
		}
	}

	// 帮助/错误模态下，左键点击关闭
	if m.showHelp && msg.Button == tea.MouseButtonLeft && msg.Action == tea.MouseActionPress {
		m.showHelp = false
		return m, nil
	}
	if m.showError && msg.Button == tea.MouseButtonLeft && msg.Action == tea.MouseActionPress {
		m.showError = false
		m.errorMessage = ""
		return m, nil
	}

	// 其他输入模态下忽略鼠标（有 textinput 焦点）
	if m.searchMode || m.lineNumMode || m.newSessionMode || m.renameMode || m.deleteConfirmMode {
		return m, nil
	}

	switch msg.Button {
	case tea.MouseButtonWheelUp:
		if msg.X >= m.width*70/100 {
			m.detailView.LineUp(3)
		} else {
			m.moveCursor(-3)
		}
		return m, nil

	case tea.MouseButtonWheelDown:
		if msg.X >= m.width*70/100 {
			m.detailView.LineDown(3)
		} else {
			m.moveCursor(3)
		}
		return m, nil
	case tea.MouseButtonLeft:
		if msg.Action != tea.MouseActionPress {
			return m, nil
		}

		// 仅处理顶部菜单下方的树区域点击。
		treeWidth := m.width * 70 / 100
		contentHeight := m.height - 2
		contentY := msg.Y - 1
		if msg.X < 0 || msg.X >= treeWidth || contentY < 0 || contentY >= contentHeight {
			return m, nil
		}

		visibleNodes := m.getVisibleNodes()
		if len(visibleNodes) == 0 {
			return m, nil
		}
		startIdx := computeTreeOffset(m.cursor, len(visibleNodes), contentHeight)
		clickedIndex := startIdx + contentY
		if clickedIndex >= len(visibleNodes) {
			return m, nil
		}

		// 双击检测：同一节点索引 + 400ms 内
		isDoubleClick := clickedIndex == m.lastClickIndex &&
			time.Since(m.lastClickTime) < 400*time.Millisecond

		m.lastClickTime = time.Now()
		m.lastClickIndex = clickedIndex

		if isDoubleClick {
			m.cursor = clickedIndex
			node := visibleNodes[clickedIndex]
			if node.IsDir {
				node.Expanded = !node.Expanded
			} else if node.Session != nil && node.Session.Valid {
				return m, m.execSSHCommand(node.Session)
			}
			return m, nil
		}

		// 单击：设置光标
		m.cursor = clickedIndex
		return m, nil

	case tea.MouseButtonRight:
		if msg.Action != tea.MouseActionPress {
			return m, nil
		}
		m.contextMenu.visible = false

		treeWidth := m.width * 70 / 100
		contentHeight := m.height - 2
		contentY := msg.Y - 1
		if msg.X < 0 || msg.X >= treeWidth || contentY < 0 || contentY >= contentHeight {
			return m, nil
		}

		visibleNodes := m.getVisibleNodes()
		if len(visibleNodes) == 0 {
			return m, nil
		}
		startIdx := computeTreeOffset(m.cursor, len(visibleNodes), contentHeight)
		clickedIndex := startIdx + contentY
		if clickedIndex >= len(visibleNodes) {
			return m, nil
		}

		node := visibleNodes[clickedIndex]
		m.cursor = clickedIndex

		if node.IsDir {
			label := "展开"
			if node.Expanded {
				label = "折叠"
			}
			items := []ContextMenuItem{
				{Label: label, Key: "Click", Action: "toggle-folder"},
				{Label: "新建会话", Key: "n", Action: "new-in-folder"},
			}
			m.contextMenu = ContextMenu{visible: true, items: items, cursor: 0, node: node}
			return m, nil
		}

		var items []ContextMenuItem
		if node.Session != nil && node.Session.Valid {
			items = append(items, ContextMenuItem{Label: "连接", Key: "Enter", Action: "connect"})
		}
		if node.Session != nil {
			if !node.IsReadOnly() {
				items = append(items, ContextMenuItem{Label: "编辑", Key: "e", Action: "edit"})
				items = append(items, ContextMenuItem{Label: "重命名", Key: "c", Action: "rename"})
				items = append(items, ContextMenuItem{Label: "删除", Key: "D", Action: "delete"})
			}
		}
		if len(items) == 0 {
			return m, nil
		}

		m.contextMenu = ContextMenu{visible: true, items: items, cursor: 0, node: node}
		return m, nil
	}

	return m, nil
}

// handleContextMenuKey 处理右键菜单的键盘输入
func (m Model) handleContextMenuKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case msg.Type == tea.KeyEsc:
		m.contextMenu.visible = false
		return m, nil
	case msg.String() == "j" || msg.Type == tea.KeyDown:
		if m.contextMenu.cursor < len(m.contextMenu.items)-1 {
			m.contextMenu.cursor++
		}
		return m, nil
	case msg.String() == "k" || msg.Type == tea.KeyUp:
		if m.contextMenu.cursor > 0 {
			m.contextMenu.cursor--
		}
		return m, nil
	case msg.Type == tea.KeyEnter:
		return m.executeContextMenuAction()
	}
	m.contextMenu.visible = false
	return m, nil
}

// executeContextMenuAction 执行右键菜单选中的操作
func (m Model) executeContextMenuAction() (tea.Model, tea.Cmd) {
	if m.contextMenu.cursor < 0 || m.contextMenu.cursor >= len(m.contextMenu.items) {
		m.contextMenu.visible = false
		return m, nil
	}
	item := m.contextMenu.items[m.contextMenu.cursor]
	node := m.contextMenu.node
	m.contextMenu.visible = false
	if node == nil {
		return m, nil
	}
	if item.Action == "toggle-folder" && node.IsDir {
		node.Expanded = !node.Expanded
		return m, nil
	}
	if item.Action == "new-in-folder" && node.IsDir {
		return m, m.prepareNewSession()
	}
	if node.Session == nil {
		return m, nil
	}

	switch item.Action {
	case "connect":
		if node.Session.Valid {
			return m, m.execSSHCommand(node.Session)
		}
	case "edit":
		if node.Session.FilePath != "" {
			return m, m.execEditCommand(node.Session)
		}
	case "delete":
		return m, m.prepareDeleteConfirm(node)
	case "rename":
		if node.Session.FilePath != "" {
			return m, m.prepareRenameSession(node)
		}
	}
	return m, nil
}
