package xftp

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/ketor/xsc/internal/session"
	"github.com/ketor/xsc/internal/shared"
)

// SessionSelectedMsg 用户选择了一个 session
type SessionSelectedMsg struct {
	Session *session.Session
}

// sessionsLoadedMsg session 树加载完成（内部消息）
type sessionsLoadedMsg struct {
	tree        *session.SessionNode
	sessionsDir string
}

// selectorCommand 是 shared.Command 的类型别名，保持向后兼容
type selectorCommand = shared.Command

// selectorCommands 是选择器所有 : 命令的注册表
var selectorCommands = []selectorCommand{
	{Name: "q", Aliases: []string{"quit"}, Description: "退出程序"},
	{Name: "noh", Aliases: []string{"nohlsearch"}, Description: "清除搜索过滤"},
	{Name: "pw", Aliases: []string{"password"}, Description: "切换密码显示"},
}

// Selector session 选择器
type Selector struct {
	tree      *session.SessionNode   // session 树根节点
	flatNodes []*session.SessionNode // 展平后的可见节点
	cursor    int
	offset    int
	width     int
	height    int

	// 搜索
	searchInput textinput.Model
	searching   bool
	filter      string

	// 命令模式
	commandInput textinput.Model
	commanding   bool

	// 帮助
	showHelp bool

	// 密码显示
	showPassword bool

	// 鼠标
	lastClickTime  time.Time // 上次点击时间（双击检测）
	lastClickIndex int       // 上次点击的节点索引（双击检测）

	// 状态
	loading       bool
	lastKeyG      bool   // 检测 gg 组合
	lineNumBuffer string // 数字键累积（用于 nG 跳行）
	statusMsg     string
	showError     bool
	errorMessage  string
}

// NewSelector 创建 session 选择器
func NewSelector() Selector {
	searchInput := textinput.New()
	searchInput.Placeholder = "搜索会话..."
	searchInput.Prompt = "/"
	searchInput.CharLimit = 50
	searchInput.Width = 30

	commandInput := textinput.New()
	commandInput.Prompt = ":"
	commandInput.CharLimit = 20
	commandInput.Width = 20

	return Selector{
		loading:      true,
		searchInput:  searchInput,
		commandInput: commandInput,
		statusMsg:    "加载会话中...",
	}
}

// Init 初始化选择器（加载 session 树）
func (s Selector) Init() tea.Cmd {
	return s.loadSessions()
}

// loadSessions 异步加载所有 session
func (s Selector) loadSessions() tea.Cmd {
	return func() tea.Msg {
		tree, sessionsDir := shared.LoadSessionTree()
		return sessionsLoadedMsg{tree: tree, sessionsDir: sessionsDir}
	}
}

// Update 处理选择器消息
func (s Selector) Update(msg tea.Msg) (Selector, tea.Cmd) {
	switch msg := msg.(type) {
	case sessionsLoadedMsg:
		s.loading = false
		s.tree = msg.tree
		if s.tree != nil {
			s.tree.SetParent(nil)
			s.expandAll(s.tree)
			s.updateFlatNodes()
			s.statusMsg = fmt.Sprintf("%d 个会话", s.countSessions(s.tree))
		} else {
			s.statusMsg = "未找到会话"
		}
		return s, nil

	case tea.MouseMsg:
		return s.handleMouse(msg)

	case tea.KeyMsg:
		// 错误模式：任意键关闭
		if s.showError {
			s.showError = false
			s.errorMessage = ""
			return s, nil
		}
		// 帮助模式：任意键退出（Ctrl+C 退出程序）
		if s.showHelp {
			keys := DefaultKeyMap()
			if key.Matches(msg, keys.Quit) {
				return s, tea.Quit
			}
			s.showHelp = false
			return s, nil
		}
		if s.commanding {
			return s.handleCommandKey(msg)
		}
		if s.searching {
			return s.handleSearchKey(msg)
		}
		return s.handleNormalKey(msg)
	}
	return s, nil
}

// handleMouse 处理鼠标事件
func (s Selector) handleMouse(msg tea.MouseMsg) (Selector, tea.Cmd) {
	// 模态模式下忽略鼠标
	if s.showHelp || s.showError || s.searching || s.commanding {
		return s, nil
	}

	switch msg.Button {
	case tea.MouseButtonWheelUp:
		s.moveCursor(-3)
		return s, nil

	case tea.MouseButtonWheelDown:
		s.moveCursor(3)
		return s, nil

	case tea.MouseButtonLeft:
		if msg.Action != tea.MouseActionPress {
			return s, nil
		}

		// 仅处理树区域点击
		treeWidth := s.width * 70 / 100
		if msg.X >= treeWidth {
			return s, nil
		}

		// Y 边界检查：排除边框和状态栏区域的点击
		contentHeight := s.height - 2        // 减去状态栏
		treeInnerHeight := contentHeight - 2 // 减去上下边框
		if msg.Y < 1 || msg.Y > treeInnerHeight {
			return s, nil
		}

		// 减去边框偏移（selectorTreeStyle 使用 RoundedBorder）
		clickedIndex := s.offset + (msg.Y - 1)

		// 边界检查
		if clickedIndex < 0 || clickedIndex >= len(s.flatNodes) {
			return s, nil
		}

		// 双击检测
		now := time.Now()
		isDoubleClick := clickedIndex == s.lastClickIndex && now.Sub(s.lastClickTime) < 400*time.Millisecond

		// 更新点击记录
		s.lastClickTime = now
		s.lastClickIndex = clickedIndex

		if isDoubleClick {
			node := s.flatNodes[clickedIndex]
			if node.IsDir {
				// 双击目录：展开/折叠
				node.Expanded = !node.Expanded
				s.updateFlatNodes()
			} else if node.Session != nil && node.Session.Valid {
				// 双击会话：选择
				return s, func() tea.Msg {
					return SessionSelectedMsg{Session: node.Session}
				}
			}
		} else {
			// 单击：移动光标
			s.cursor = clickedIndex
			s.ensureVisible()
		}

		return s, nil
	}

	return s, nil
}

// handleNormalKey 处理普通模式的键盘输入
func (s Selector) handleNormalKey(msg tea.KeyMsg) (Selector, tea.Cmd) {
	keys := DefaultKeyMap()

	// 统一重置 lineNumBuffer 和 lastKeyG（仅数字键和 g 键在各自分支中恢复）
	savedLineNumBuffer := s.lineNumBuffer
	savedLastKeyG := s.lastKeyG
	s.lineNumBuffer = ""
	s.lastKeyG = false

	switch {
	case key.Matches(msg, keys.Quit):
		return s, tea.Quit

	case key.Matches(msg, keys.Up):
		s.moveCursor(-1)

	case key.Matches(msg, keys.Down):
		s.moveCursor(1)

	case key.Matches(msg, keys.HalfPageUp):
		h := s.viewHeight()
		s.moveCursor(-(h / 2))

	case key.Matches(msg, keys.HalfPageDown):
		h := s.viewHeight()
		s.moveCursor(h / 2)

	case key.Matches(msg, keys.PageUp):
		s.moveCursor(-s.viewHeight())

	case key.Matches(msg, keys.PageDown):
		s.moveCursor(s.viewHeight())

	// gg 跳顶
	case msg.String() == "g":
		if savedLastKeyG {
			s.cursor = 0
			s.ensureVisible()
		} else {
			s.lastKeyG = true
		}
		return s, nil

	// G 跳底（或 nG 跳行）
	case msg.String() == "G":
		if savedLineNumBuffer != "" {
			var lineNum int
			fmt.Sscanf(savedLineNumBuffer, "%d", &lineNum)
			if lineNum > 0 && len(s.flatNodes) > 0 {
				s.cursor = lineNum - 1
				if s.cursor >= len(s.flatNodes) {
					s.cursor = len(s.flatNodes) - 1
				}
				if s.cursor < 0 {
					s.cursor = 0
				}
				s.ensureVisible()
			}
		} else if len(s.flatNodes) > 0 {
			s.cursor = len(s.flatNodes) - 1
			s.ensureVisible()
		}

	// 0 — 跳到第一行
	case msg.String() == "0":
		s.cursor = 0
		s.ensureVisible()

	// $ — 跳到最后一行
	case msg.String() == "$":
		if len(s.flatNodes) > 0 {
			s.cursor = len(s.flatNodes) - 1
			s.ensureVisible()
		}

	// ^ — 跳到第一个文件节点（非目录）
	case msg.String() == "^":
		for i, node := range s.flatNodes {
			if !node.IsDir {
				s.cursor = i
				s.ensureVisible()
				break
			}
		}

	// 数字键累积（用于 nG 跳行）
	case len(msg.String()) == 1 && msg.String()[0] >= '1' && msg.String()[0] <= '9':
		s.lineNumBuffer = savedLineNumBuffer + msg.String()
		return s, nil

	// n — 搜索下一个匹配
	case msg.String() == "n":
		if s.filter != "" {
			s.searchNext(1)
		}

	// N — 搜索上一个匹配
	case msg.String() == "N":
		if s.filter != "" {
			s.searchNext(-1)
		}

	case key.Matches(msg, keys.Enter):
		return s.selectCurrent()

	case key.Matches(msg, keys.OpenFold):
		// l — 展开目录
		node := s.currentNode()
		if node != nil && node.IsDir && !node.Expanded {
			node.Expanded = true
			s.updateFlatNodes()
		}

	case key.Matches(msg, keys.CloseFold):
		// h — 折叠目录或跳到父目录
		node := s.currentNode()
		if node != nil {
			if node.IsDir && node.Expanded {
				node.Expanded = false
				s.updateFlatNodes()
			} else if node.Parent != nil {
				for i, n := range s.flatNodes {
					if n == node.Parent {
						s.cursor = i
						s.ensureVisible()
						break
					}
				}
			}
		}

	case key.Matches(msg, keys.ToggleFold):
		// o — 切换展开/折叠（等同 Space）
		node := s.currentNode()
		if node != nil && node.IsDir {
			node.Expanded = !node.Expanded
			s.updateFlatNodes()
		}

	case key.Matches(msg, keys.Select):
		// Space — 切换展开/折叠
		node := s.currentNode()
		if node != nil && node.IsDir {
			node.Expanded = !node.Expanded
			s.updateFlatNodes()
		}

	case key.Matches(msg, keys.OpenAllFolds):
		// E — 展开所有目录
		if s.tree != nil {
			s.expandAll(s.tree)
			s.updateFlatNodes()
		}

	case key.Matches(msg, keys.CloseAllFolds):
		// C — 折叠所有目录
		if s.tree != nil {
			s.collapseAll(s.tree)
			s.updateFlatNodes()
		}

	case key.Matches(msg, keys.Help):
		// ? — 帮助
		s.showHelp = true
		return s, nil

	case key.Matches(msg, keys.Command):
		// : — 进入命令模式
		s.commanding = true
		s.commandInput.SetValue("")
		s.commandInput.Focus()
		return s, textinput.Blink

	case key.Matches(msg, keys.Search):
		// / — 进入搜索模式
		s.searching = true
		s.searchInput.Focus()
		return s, textinput.Blink
	}

	return s, nil
}

// handleSearchKey 处理搜索模式的键盘输入
func (s Selector) handleSearchKey(msg tea.KeyMsg) (Selector, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEsc:
		// Esc — 退出搜索并清除过滤
		s.searching = false
		s.filter = ""
		s.searchInput.SetValue("")
		s.updateFlatNodes()
		s.cursor = 0
		s.offset = 0
		return s, nil

	case tea.KeyCtrlC:
		// Ctrl+C — 退出搜索但保留当前过滤结果
		s.searching = false
		return s, nil

	case tea.KeyCtrlU:
		// Ctrl+U — 清空当前输入
		s.searchInput.SetValue("")
		s.filter = ""
		s.updateFlatNodes()
		s.cursor = 0
		s.offset = 0
		return s, nil

	case tea.KeyEnter:
		s.searching = false
		s.filter = s.searchInput.Value()
		s.updateFlatNodes()
		s.cursor = 0
		s.offset = 0
		return s, nil

	default:
		var cmd tea.Cmd
		s.searchInput, cmd = s.searchInput.Update(msg)
		// 实时过滤
		s.filter = s.searchInput.Value()
		s.updateFlatNodes()
		s.cursor = 0
		s.offset = 0
		return s, cmd
	}
}

// handleCommandKey 处理命令模式的键盘输入
func (s Selector) handleCommandKey(msg tea.KeyMsg) (Selector, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEsc, tea.KeyCtrlC:
		s.commanding = false
		s.commandInput.SetValue("")
		return s, nil

	case tea.KeyTab:
		// Tab 自动补全
		input := s.commandInput.Value()
		completions := getSelectorCommandCompletions(input)
		if len(completions) > 0 {
			s.commandInput.SetValue(completions[0].Name)
			s.commandInput.CursorEnd()
		}
		return s, nil

	case tea.KeyEnter:
		s.commanding = false
		cmdStr := s.commandInput.Value()
		s.commandInput.SetValue("")

		switch matchSelectorCommand(cmdStr) {
		case "q":
			return s, tea.Quit
		case "noh":
			s.filter = ""
			s.searchInput.SetValue("")
			s.updateFlatNodes()
			s.cursor = 0
			s.offset = 0
			return s, nil
		case "pw":
			s.showPassword = !s.showPassword
			if s.showPassword {
				s.statusMsg = "密码已显示"
			} else {
				s.statusMsg = "密码已隐藏"
			}
			return s, nil
		}

		// 尝试解析行号并跳转
		if cmdStr != "" {
			var lineNum int
			fmt.Sscanf(cmdStr, "%d", &lineNum)
			if lineNum > 0 && len(s.flatNodes) > 0 {
				s.cursor = lineNum - 1
				if s.cursor >= len(s.flatNodes) {
					s.cursor = len(s.flatNodes) - 1
				}
				if s.cursor < 0 {
					s.cursor = 0
				}
				s.ensureVisible()
			}
		}
		return s, nil

	default:
		var cmd tea.Cmd
		s.commandInput, cmd = s.commandInput.Update(msg)
		return s, cmd
	}
}

// selectCurrent 选择当前光标指向的 session
func (s Selector) selectCurrent() (Selector, tea.Cmd) {
	node := s.currentNode()
	if node == nil {
		return s, nil
	}

	if node.IsDir {
		// 目录：切换展开/折叠
		node.Expanded = !node.Expanded
		s.updateFlatNodes()
		return s, nil
	}

	if node.Session != nil && node.Session.Valid {
		return s, func() tea.Msg {
			return SessionSelectedMsg{Session: node.Session}
		}
	}

	s.statusMsg = "无效的会话"
	return s, nil
}

// View 渲染选择器（70/30 左右分栏布局）
func (s Selector) View(width, height int) string {
	if s.loading {
		content := lipgloss.NewStyle().
			Foreground(lipgloss.Color(colorFgDim)).
			Render("\n  加载中...")
		return lipgloss.NewStyle().
			Width(width).Height(height).
			Render(content)
	}

	// 计算布局尺寸
	treeWidth := width * 70 / 100
	detailWidth := width - treeWidth
	contentHeight := height - 2 // 留出状态栏空间

	// 渲染树面板
	treeView := s.renderTreePanel(treeWidth, contentHeight)

	// 渲染详情面板
	detailView := s.renderDetailPanel(detailWidth, contentHeight)

	// 水平拼接
	content := lipgloss.JoinHorizontal(lipgloss.Top, treeView, detailView)

	// 状态栏
	statusBar := s.renderSelectorStatusBar(width)

	// 帮助模式：覆盖整个内容区域
	if s.showHelp {
		helpView := s.renderHelp(width, height)
		return helpView
	}

	// 搜索模式显示搜索栏
	if s.searching {
		searchBar := SearchStyle.Width(width).Render(s.searchInput.View() + "  (Esc:取消 Enter:确认 C-u:清空 C-c:保留过滤)")
		return lipgloss.JoinVertical(lipgloss.Left, content, statusBar, searchBar)
	}

	// 命令模式显示命令栏
	if s.commanding {
		cmdBar := s.renderCommandBar(width)
		return lipgloss.JoinVertical(lipgloss.Left, content, statusBar, cmdBar)
	}

	// 错误模式
	if s.showError {
		errorStyle := lipgloss.NewStyle().
			Foreground(lipgloss.Color(colorRed)).
			Background(lipgloss.Color(colorBg)).
			Padding(1).
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color(colorRed))
		return errorStyle.Render(s.errorMessage + "\n\n按任意键返回...")
	}

	return lipgloss.JoinVertical(lipgloss.Left, content, statusBar)
}

// renderTreePanel 渲染左侧树面板（带边框）
func (s Selector) renderTreePanel(width, height int) string {
	innerWidth := width - 2 // 减去边框
	innerHeight := height - 2

	if len(s.flatNodes) == 0 {
		var msg string
		if s.filter != "" {
			msg = fmt.Sprintf("\n  未找到匹配 \"%s\" 的会话", s.filter)
		} else {
			msg = "\n  未找到会话\n\n  请先使用 xssh 导入或创建 SSH 会话"
		}
		content := lipgloss.NewStyle().
			Foreground(lipgloss.Color(colorFgDim)).
			Render(msg)
		return selectorTreeStyle.
			Width(innerWidth).Height(innerHeight).
			Render(content)
	}

	// 计算行号宽度
	totalNodes := len(s.flatNodes)
	lineNumWidth := len(fmt.Sprintf("%d", totalNodes))
	if lineNumWidth < 3 {
		lineNumWidth = 3
	}

	// 树形列表
	viewH := innerHeight
	if viewH < 1 {
		viewH = 1
	}
	endIdx := s.offset + viewH
	if endIdx > totalNodes {
		endIdx = totalNodes
	}

	var lines []string
	for i := s.offset; i < endIdx; i++ {
		// 行号前缀
		lineNumStyle := lipgloss.NewStyle().
			Foreground(lipgloss.Color(colorFgDark)).
			Width(lineNumWidth).
			Align(lipgloss.Right)
		if i == s.cursor {
			lineNumStyle = lineNumStyle.Foreground(lipgloss.Color(colorYellow))
		}
		lineNum := lineNumStyle.Render(fmt.Sprintf("%d", i+1))
		nodeLine := s.renderNode(s.flatNodes[i], i == s.cursor, innerWidth-lineNumWidth-3)
		lines = append(lines, lineNum+" "+nodeLine)
	}
	for len(lines) < viewH {
		lines = append(lines, "")
	}
	treeContent := strings.Join(lines, "\n")

	return selectorTreeStyle.
		Width(innerWidth).Height(innerHeight).
		Render(treeContent)
}

// renderDetailPanel 渲染右侧详情面板
func (s Selector) renderDetailPanel(width, height int) string {
	innerWidth := width - 2
	innerHeight := height - 2

	node := s.currentNode()
	if node == nil {
		return selectorDetailStyle.
			Width(innerWidth).Height(innerHeight).
			Render(lipgloss.NewStyle().
				Foreground(lipgloss.Color(colorFgDim)).
				Render("未选择会话"))
	}

	var content strings.Builder

	if node.IsDir {
		// 目录：显示目录名和子项数
		content.WriteString(SelectorDetailTitleStyle.Render(node.Name + "/"))
		content.WriteString("\n\n")
		childCount := len(node.Children)
		content.WriteString(SelectorDetailKeyStyle.Render("子项: "))
		content.WriteString(SelectorDetailValueStyle.Render(fmt.Sprintf("%d", childCount)))
		content.WriteString("\n")
	} else if node.Session != nil {
		sess := node.Session
		// 标题
		content.WriteString(SelectorDetailTitleStyle.Render(node.Name))
		content.WriteString("\n\n")

		// Host
		content.WriteString(SelectorDetailKeyStyle.Render("Host: "))
		content.WriteString(SelectorDetailValueStyle.Render(sess.Host))
		content.WriteString("\n\n")

		// Port
		content.WriteString(SelectorDetailKeyStyle.Render("Port: "))
		content.WriteString(SelectorDetailValueStyle.Render(fmt.Sprintf("%d", sess.Port)))
		content.WriteString("\n\n")

		// User
		content.WriteString(SelectorDetailKeyStyle.Render("User: "))
		content.WriteString(SelectorDetailValueStyle.Render(sess.User))
		content.WriteString("\n\n")

		// AuthType
		content.WriteString(SelectorDetailKeyStyle.Render("Auth: "))
		authStr := string(sess.AuthType)
		if len(sess.AuthMethods) > 0 {
			var methods []string
			for _, am := range sess.AuthMethods {
				methods = append(methods, am.Type)
			}
			authStr = strings.Join(methods, ", ")
		}
		content.WriteString(SelectorDetailValueStyle.Render(authStr))
		content.WriteString("\n\n")

		// Password（密码显示/隐藏）
		if sess.AuthType == session.AuthTypePassword || s.hasPasswordAuth(sess) {
			content.WriteString(SelectorDetailKeyStyle.Render("Pass: "))
			if s.showPassword {
				// 延迟解密加密密码
				if sess.Password == "" && sess.EncryptedPassword != "" {
					sess.ResolvePassword()
				}
				if sess.Password != "" {
					content.WriteString(SelectorDetailValueStyle.Render(sess.Password))
				} else {
					content.WriteString(SelectorDetailValueStyle.Render("(empty)"))
				}
			} else {
				content.WriteString(SelectorDetailValueStyle.Render("********"))
			}
			content.WriteString("\n\n")
		}

		// Description
		if sess.Description != "" {
			content.WriteString(SelectorDetailKeyStyle.Render("Desc: "))
			content.WriteString(lipgloss.NewStyle().
				Foreground(lipgloss.Color(colorFg)).
				Render(sess.Description))
			content.WriteString("\n")
		}

		// Invalid 标记
		if !sess.Valid {
			content.WriteString("\n")
			content.WriteString(lipgloss.NewStyle().
				Foreground(lipgloss.Color(colorRed)).
				Render("Error: " + sess.Error.Error()))
		}
	} else {
		content.WriteString(lipgloss.NewStyle().
			Foreground(lipgloss.Color(colorFgDim)).
			Render("无会话数据"))
	}

	return selectorDetailStyle.
		Width(innerWidth).Height(innerHeight).
		Render(content.String())
}

// renderSelectorStatusBar 渲染选择器状态栏
func (s Selector) renderSelectorStatusBar(width int) string {
	var left string

	// 位置信息
	if len(s.flatNodes) > 0 {
		left = fmt.Sprintf(" %d/%d", s.cursor+1, len(s.flatNodes))
	}

	// 过滤状态
	if s.filter != "" {
		left += fmt.Sprintf(" | 过滤: '%s'", s.filter)
	} else {
		left += fmt.Sprintf(" | %s", s.statusMsg)
	}

	// 右侧帮助
	right := " Enter:选择 /:搜索 q:退出 "

	gap := width - lipgloss.Width(left) - lipgloss.Width(right)
	if gap < 0 {
		gap = 0
	}

	bar := left + strings.Repeat(" ", gap) + right
	return StatusBarStyle.Width(width).Render(bar)
}

// renderNode 渲染单个树节点
func (s Selector) renderNode(node *session.SessionNode, selected bool, width int) string {
	indent := s.getIndent(node)

	isSecureCRT := node.IsSecureCRT()
	isXShell := node.IsXShell()
	isMobaXterm := node.IsMobaXterm()
	isExternal := isSecureCRT || isXShell || isMobaXterm

	var icon string
	var name string

	if node.IsDir {
		if node.Expanded {
			icon = "▾ "
		} else {
			icon = "▸ "
		}

		if selected {
			// 选中时使用纯文本，CursorStyle 统一着色
			if isSecureCRT {
				name = "[CRT] " + node.Name + "/"
			} else if isXShell {
				name = "[XSH] " + node.Name + "/"
			} else if isMobaXterm {
				name = "[MXT] " + node.Name + "/"
			} else {
				name = node.Name + "/"
			}
		} else {
			// 非选中：外部来源目录使用特殊样式
			if isSecureCRT {
				name = lipgloss.NewStyle().Foreground(lipgloss.Color("#b16286")).Bold(true).
					Render("[CRT] " + node.Name + "/")
			} else if isXShell {
				name = lipgloss.NewStyle().Foreground(lipgloss.Color("#458588")).Bold(true).
					Render("[XSH] " + node.Name + "/")
			} else if isMobaXterm {
				name = lipgloss.NewStyle().Foreground(lipgloss.Color("#d65d0e")).Bold(true).
					Render("[MXT] " + node.Name + "/")
			} else {
				name = DirStyle.Render(node.Name + "/")
			}
		}
	} else {
		// 外部来源文件使用🔒图标
		if isExternal {
			icon = "🔒 "
		} else {
			icon = "  "
		}

		nodeName := node.Name
		// 显示连接信息
		if node.Session != nil && node.Session.Valid {
			info := fmt.Sprintf(" (%s@%s:%d)", node.Session.User, node.Session.Host, node.Session.Port)
			maxName := width - len(indent) - 4 - len(info) - 2
			if maxName > 0 && len(nodeName) > maxName {
				nodeName = nodeName[:maxName-3] + "..."
			}

			if selected {
				// 选中时使用纯文本
				name = nodeName + info
			} else {
				dimInfo := lipgloss.NewStyle().Foreground(lipgloss.Color(colorFgDim)).Render(info)
				// 外部来源文件使用不同颜色
				if isSecureCRT {
					name = securecrtFileStyle.Render(nodeName) + dimInfo
				} else if isXShell {
					name = xshellFileStyle.Render(nodeName) + dimInfo
				} else if isMobaXterm {
					name = mobaxtermFileStyle.Render(nodeName) + dimInfo
				} else {
					name = FileStyle.Render(nodeName) + dimInfo
				}
			}
		} else if node.Session != nil && !node.Session.Valid {
			if selected {
				name = nodeName + " [invalid]"
			} else {
				name = lipgloss.NewStyle().Foreground(lipgloss.Color(colorRed)).Render(nodeName + " [invalid]")
			}
		} else {
			if selected {
				name = nodeName
			} else {
				name = FileStyle.Render(nodeName)
			}
		}
	}

	line := indent + icon + name

	if selected {
		return CursorStyle.Width(width).Render(line)
	}

	return line
}

// ============================================================
// 内部辅助方法
// ============================================================

// updateFlatNodes 根据展开状态和过滤条件重新生成可见节点列表
func (s *Selector) updateFlatNodes() {
	if s.tree == nil {
		s.flatNodes = nil
		return
	}

	if s.filter == "" {
		s.flatNodes = s.tree.FlattenVisible()
	} else {
		// 带过滤的展平：只显示匹配的叶子节点及其祖先路径
		s.flatNodes = s.filterNodes(s.tree)
	}
}

// filterNodes 递归过滤节点
func (s *Selector) filterNodes(node *session.SessionNode) []*session.SessionNode {
	var result []*session.SessionNode
	filterLower := strings.ToLower(s.filter)

	for _, child := range node.Children {
		if child.IsDir {
			// 目录：递归检查是否有匹配的子节点
			childMatches := s.filterNodes(child)
			if len(childMatches) > 0 {
				result = append(result, child)
				child.Expanded = true
				result = append(result, childMatches...)
			}
		} else {
			// 叶子节点：匹配名称、host、user
			if s.matchesFilter(child, filterLower) {
				result = append(result, child)
			}
		}
	}
	return result
}

// matchesFilter 检查节点是否匹配过滤条件
func (s *Selector) matchesFilter(node *session.SessionNode, filterLower string) bool {
	if strings.Contains(strings.ToLower(node.Name), filterLower) {
		return true
	}
	if node.Session != nil {
		if strings.Contains(strings.ToLower(node.Session.Host), filterLower) {
			return true
		}
		if strings.Contains(strings.ToLower(node.Session.User), filterLower) {
			return true
		}
	}
	return false
}

// currentNode 返回当前光标指向的节点
func (s *Selector) currentNode() *session.SessionNode {
	if s.cursor < 0 || s.cursor >= len(s.flatNodes) {
		return nil
	}
	return s.flatNodes[s.cursor]
}

// moveCursor 移动光标
func (s *Selector) moveCursor(delta int) {
	s.cursor += delta
	if s.cursor < 0 {
		s.cursor = 0
	}
	if s.cursor >= len(s.flatNodes) {
		s.cursor = len(s.flatNodes) - 1
	}
	if s.cursor < 0 {
		s.cursor = 0
	}
	s.ensureVisible()
}

// ensureVisible 确保光标在可视区域内
func (s *Selector) ensureVisible() {
	viewH := s.viewHeight()
	if s.cursor < s.offset {
		s.offset = s.cursor
	}
	if s.cursor >= s.offset+viewH {
		s.offset = s.cursor - viewH + 1
	}
}

// viewHeight 可用于树列表的高度
// 总高度减去：状态栏(2) + 边框(2)
func (s *Selector) viewHeight() int {
	h := s.height - 4
	if h < 1 {
		h = 1
	}
	return h
}

// getIndent 获取节点缩进
func (s Selector) getIndent(node *session.SessionNode) string {
	return shared.GetIndent(node)
}

// expandAll 展开所有目录
func (s *Selector) expandAll(node *session.SessionNode) {
	shared.ExpandAll(node)
}

// collapseAll 折叠所有目录
func (s *Selector) collapseAll(node *session.SessionNode) {
	shared.CollapseAll(node)
}

// searchNext 查找下一个/上一个匹配项（在过滤后的节点中循环搜索）
func (s *Selector) searchNext(direction int) {
	if s.filter == "" || len(s.flatNodes) == 0 {
		return
	}

	query := strings.ToLower(s.filter)
	startIdx := s.cursor

	for i := 1; i <= len(s.flatNodes); i++ {
		idx := startIdx + (i * direction)

		// 循环搜索
		if idx >= len(s.flatNodes) {
			idx = idx % len(s.flatNodes)
		} else if idx < 0 {
			idx = idx + len(s.flatNodes)
		}

		node := s.flatNodes[idx]
		if node.IsDir {
			continue
		}
		if s.matchesFilter(node, query) {
			s.cursor = idx
			s.ensureVisible()
			return
		}
	}
}

// renderCommandBar 渲染命令输入栏（带命令补全提示）
func (s Selector) renderCommandBar(width int) string {
	input := s.commandInput.Value()
	completions := getSelectorCommandCompletions(input)

	var hints []string
	for i, cmd := range completions {
		hint := fmt.Sprintf(":%s - %s", cmd.Name, cmd.Description)
		if i == 0 {
			hints = append(hints, CmdHintActiveStyle.Render(hint))
		} else {
			hints = append(hints, CmdHintStyle.Render(hint))
		}
	}

	bar := s.commandInput.View()
	if len(hints) > 0 {
		bar += "  " + strings.Join(hints, "  ")
	}
	bar += "  " + CmdHintStyle.Render("(Tab:补全 Enter:执行 Esc:取消)")

	return SearchStyle.Width(width).Render(bar)
}

// renderHelp 渲染帮助视图
func (s Selector) renderHelp(width, height int) string {
	var b strings.Builder

	renderSection := func(title string, items [][2]string) {
		b.WriteString(HelpSectionStyle.Render(title))
		b.WriteString("\n")
		for _, item := range items {
			b.WriteString("  ")
			b.WriteString(HelpKeyStyle.Render(item[0]))
			b.WriteString(HelpDescStyle.Render(item[1]))
			b.WriteString("\n")
		}
		b.WriteString("\n")
	}

	renderSection("导航", [][2]string{
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

	renderSection("树操作", [][2]string{
		{"Space/o", "展开/折叠目录"},
		{"h/←", "折叠目录或跳到父目录"},
		{"l/→", "展开目录"},
		{"E", "展开所有目录"},
		{"C", "折叠所有目录"},
		{"Enter", "选择会话"},
	})

	renderSection("搜索", [][2]string{
		{"/", "进入搜索模式"},
		{"Enter", "确认搜索"},
		{"Esc", "取消搜索并清除过滤"},
		{"Ctrl+c", "退出搜索并保留过滤"},
		{"Ctrl+u", "清空搜索输入"},
		{"n/N", "下一个/上一个匹配"},
	})

	// 从命令注册表自动生成
	cmdItems := make([][2]string, len(selectorCommands))
	for i, cmd := range selectorCommands {
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

	helpContent := HelpContainerStyle.Render(b.String())
	return lipgloss.NewStyle().Width(width).Height(height).Render(helpContent)
}

// matchSelectorCommand 根据输入返回匹配的命令规范名
func matchSelectorCommand(input string) string {
	return shared.MatchCommand(input, selectorCommands)
}

// getSelectorCommandCompletions 根据前缀返回匹配的命令列表
func getSelectorCommandCompletions(prefix string) []selectorCommand {
	return shared.GetCommandCompletions(prefix, selectorCommands)
}

// hasPasswordAuth 检查 session 的 AuthMethods 中是否包含密码认证
func (s Selector) hasPasswordAuth(sess *session.Session) bool {
	for _, am := range sess.AuthMethods {
		if am.Type == "password" {
			return true
		}
	}
	return false
}

// countSessions 统计叶子节点数量
func (s *Selector) countSessions(node *session.SessionNode) int {
	return shared.CountSessions(node)
}

// SetSize 设置尺寸（View 方法用参数传递，这里保留供外部调用）
func (s *Selector) SetSize(w, h int) {
	s.width = w
	s.height = h
}
