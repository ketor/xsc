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

// Mode TUI 模式
type Mode int

const (
	ModeNormal           Mode = iota // 普通模式
	ModeSearch                       // 搜索模式
	ModeCommand                      // 命令模式
	ModeHelp                         // 帮助模式
	ModeError                        // 错误模式
	ModeConfirm                      // 确认对话框模式
	ModeInput                        // 输入对话框模式（mkdir/rename）
	ModeSelector                     // 会话选择器模式
	ModeTransferResult               // 传输结果通知模式
	ModeOverwriteConfirm             // 覆盖确认模式
	ModeContextMenu                  // 右键上下文菜单模式
)

// ContextMenuItem 上下文菜单项（定义在 shared 包，此处为类型别名保持兼容）
type ContextMenuItem = shared.ContextMenuItem

// ContextMenu 上下文菜单
type ContextMenu struct {
	visible bool
	items   []ContextMenuItem
	cursor  int
}

// yankEntry yank 缓冲区条目
type yankEntry struct {
	Name  string // 文件名
	Path  string // 完整路径
	Size  int64
	IsDir bool
}

// confirmEntry 确认对话框待操作条目
type confirmEntry struct {
	Name  string
	Path  string
	IsDir bool
}

// Model xftp 主 Model
type Model struct {
	localPanel  FilePanel
	remotePanel FilePanel
	activePanel PanelSide

	session   *session.Session
	remoteFS  *RemoteFS
	connected bool

	transfer *TransferManager

	// yank 缓冲区：存储标记的文件信息
	yankFiles []yankEntry
	yankSide  PanelSide // yank 来源面板
	yankDir   string    // yank 时的目录

	mode      Mode
	width     int
	height    int
	keys      KeyMap
	statusMsg string
	err       error

	// 搜索
	searchInput textinput.Model
	searchQuery string // 当前生效的搜索词

	// 确认对话框（delete）
	confirmFiles []confirmEntry
	confirmPanel PanelSide

	// 输入对话框（mkdir/rename）
	opInput      textinput.Model
	inputOp      InputOp
	inputPanel   PanelSide
	inputOldName string // rename 时的原文件名

	// 命令模式
	cmdInput textinput.Model

	// 会话选择器
	selector Selector

	// 传输结果通知
	transferResult *TransferResultMsg

	// 覆盖确认（paste 冲突）
	overwriteConflicts  []string
	pendingPasteDir     Direction
	pendingPasteDestDir string

	// 鼠标双击检测相关字段
	lastClickTime  time.Time // 上次点击时间
	lastClickIndex int       // 上次点击的文件索引
	lastClickPanel PanelSide // 上次点击的面板

	// Shift+Click 范围选择锚点
	selectionAnchor int

	// 右键上下文菜单
	contextMenu ContextMenu
}

// NewModel 创建 xftp Model
// 如果 s 为 nil，进入会话选择器模式
func NewModel(s *session.Session) Model {
	// 搜索输入框
	searchInput := textinput.New()
	searchInput.Placeholder = "搜索文件..."
	searchInput.Prompt = "/"
	searchInput.CharLimit = 50
	searchInput.Width = 30

	// 操作输入框（mkdir/rename）
	opInput := textinput.New()
	opInput.CharLimit = 255
	opInput.Width = 40

	// 命令输入框
	cmdInput := textinput.New()
	cmdInput.Prompt = ":"
	cmdInput.CharLimit = 50
	cmdInput.Width = 30

	// 如果没有指定 session，进入选择器模式
	if s == nil {
		return Model{
			mode:        ModeSelector,
			keys:        DefaultKeyMap(),
			statusMsg:   "请选择会话",
			searchInput: searchInput,
			opInput:     opInput,
			cmdInput:    cmdInput,
			selector:    NewSelector(),
		}
	}

	// 创建本地文件系统
	localFS, err := NewLocalFS()
	var localDir string
	if err != nil {
		localDir = "/"
	} else {
		localDir, _ = localFS.Getwd()
	}

	// 创建本地面板
	var localPanel FilePanel
	if err != nil {
		// LocalFS 创建失败时用一个空面板
		localPanel = NewFilePanel(PanelLeft, nil, localDir)
		localPanel.err = err
	} else {
		localPanel = NewFilePanel(PanelLeft, localFS, localDir)
	}

	// 远程面板先创建空壳，连接成功后再初始化
	remotePanel := NewFilePanel(PanelRight, nil, "/")

	return Model{
		localPanel:  localPanel,
		remotePanel: remotePanel,
		activePanel: PanelLeft,
		session:     s,
		transfer:    NewTransferManager(),
		mode:        ModeNormal,
		keys:        DefaultKeyMap(),
		statusMsg:   "正在连接...",
		searchInput: searchInput,
		opInput:     opInput,
		cmdInput:    cmdInput,
	}
}

// Init 初始化（Bubble Tea 接口）
func (m Model) Init() tea.Cmd {
	if m.mode == ModeSelector {
		return tea.Batch(
			tea.EnterAltScreen,
			m.selector.Init(),
		)
	}
	return tea.Batch(
		tea.EnterAltScreen,
		m.localPanel.LoadDir(),
		m.connectRemote(),
	)
}

// connectRemote 异步建立远程连接
func (m Model) connectRemote() tea.Cmd {
	s := m.session
	return func() tea.Msg {
		remoteFS, err := NewRemoteFS(s)
		if err != nil {
			return ConnectErrMsg{Err: err}
		}
		return ConnectedMsg{RemoteFS: remoteFS}
	}
}

// Update 处理消息（Bubble Tea 接口）
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		if m.mode == ModeSelector {
			m.selector.SetSize(m.width, m.height)
		} else {
			m.updatePanelSizes()
		}
		return m, nil

	case sessionsLoadedMsg:
		// 路由到选择器
		if m.mode == ModeSelector {
			var cmd tea.Cmd
			m.selector, cmd = m.selector.Update(msg)
			return m, cmd
		}
		return m, nil

	case SessionSelectedMsg:
		// 用户选择了 session，切换到文件管理模式
		return m.handleSessionSelected(msg.Session)

	case tea.MouseMsg:
		// 选择器模式：路由鼠标事件到选择器
		if m.mode == ModeSelector {
			var cmd tea.Cmd
			m.selector, cmd = m.selector.Update(msg)
			return m, cmd
		}
		return m.handleMouse(msg)

	case tea.KeyMsg:
		return m.handleKey(msg)

	case ConnectedMsg:
		return m.handleConnected(msg)

	case ConnectErrMsg:
		// 连接失败：显示错误，用户必须确认后返回选择器
		m.err = msg.Err
		m.mode = ModeError
		m.statusMsg = fmt.Sprintf("连接失败: %v", msg.Err)
		return m, nil

	case DisconnectedMsg:
		m.connected = false
		m.statusMsg = "连接已断开"
		return m, nil

	case DirLoadedMsg:
		return m.handleDirLoaded(msg)

	case DirLoadErrMsg:
		return m.handleDirLoadErr(msg)

	case FileOpCompleteMsg:
		m.statusMsg = fmt.Sprintf("操作完成: %s", msg.Op)
		// 刷新两个面板
		return m, tea.Batch(
			m.localPanel.LoadDir(),
			m.remotePanel.LoadDir(),
		)

	case FileOpErrorMsg:
		m.statusMsg = fmt.Sprintf("操作失败: %s - %v", msg.Op, msg.Err)
		return m, tea.Tick(3*time.Second, func(time.Time) tea.Msg {
			return errorDismissMsg{}
		})

	case errorDismissMsg:
		// 3 秒后自动清除错误状态
		// 已知局限：简单的定时器方案可能清除后续无关的 statusMsg，
		// 但对于当前使用场景已经够用，暂不引入更复杂的消息序列号机制
		if m.err != nil {
			m.err = nil
			m.statusMsg = ""
		}
		return m, nil

	case reconnectedMsg:
		// 重连成功
		if m.remoteFS != nil {
			m.remoteFS.Close()
		}
		m.remoteFS = msg.RemoteFS
		m.connected = true
		m.statusMsg = "重连成功"
		remoteCwd, err := msg.RemoteFS.Getwd()
		if err != nil {
			remoteCwd = "/"
		}
		m.remotePanel = NewFilePanel(PanelRight, msg.RemoteFS, remoteCwd)
		m.updatePanelSizes()
		return m, m.remotePanel.LoadDir()

	case TransferProgressMsg:
		m.statusMsg = fmt.Sprintf("传输中 %d%% (%s)",
			int(msg.Progress*100), formatSize(msg.Transferred))
		// 继续监听进度
		return m, m.transfer.ListenProgress()

	case TransferCompleteMsg:
		// 记录完成的文件统计（不检查 Status，因为 TransferCompleteMsg 可能先于
		// ListenProgress 更新状态到 StatusCompleted 到达）
		for _, t := range m.transfer.Tasks() {
			if t.ID == msg.TaskID {
				m.transfer.RecordFileComplete(t.Size)
				break
			}
		}
		m.statusMsg = "传输完成"
		// 刷新两个面板
		cmds := []tea.Cmd{
			m.localPanel.LoadDir(),
			m.remotePanel.LoadDir(),
		}
		// 如果还有等待的任务，继续执行
		if m.transfer.HasPending() && m.remoteFS != nil {
			cmds = append(cmds,
				m.transfer.StartNext(m.remoteFS.SFTPClient()),
				m.transfer.ListenProgress(),
			)
		} else {
			// 所有传输完成，显示结果通知
			files, dirs, bytes, failed := m.transfer.Stats()
			m.transferResult = &TransferResultMsg{
				Files:      files,
				Dirs:       dirs,
				TotalBytes: bytes,
				Failed:     failed,
			}
			m.mode = ModeTransferResult
			m.transfer.ResetStats()
		}
		return m, tea.Batch(cmds...)

	case TransferErrorMsg:
		m.transfer.RecordFailed()
		m.statusMsg = fmt.Sprintf("传输失败: %v", msg.Err)
		// 如果还有等待的任务，继续执行
		if m.transfer.HasPending() && m.remoteFS != nil {
			return m, tea.Batch(
				m.transfer.StartNext(m.remoteFS.SFTPClient()),
				m.transfer.ListenProgress(),
			)
		}
		// 所有传输完成（含失败），显示结果通知
		files, dirs, bytes, failed := m.transfer.Stats()
		m.transferResult = &TransferResultMsg{
			Files:      files,
			Dirs:       dirs,
			TotalBytes: bytes,
			Failed:     failed,
		}
		m.mode = ModeTransferResult
		m.transfer.ResetStats()
		return m, nil
	}

	return m, nil
}

// handleKey 处理键盘输入
func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// 选择器模式：路由到选择器
	if m.mode == ModeSelector {
		var cmd tea.Cmd
		m.selector, cmd = m.selector.Update(msg)
		return m, cmd
	}

	// 右键菜单模式
	if m.mode == ModeContextMenu {
		return m.handleContextMenuKey(msg)
	}

	// 搜索模式：优先处理
	if m.mode == ModeSearch {
		return m.handleSearchKey(msg)
	}

	// 确认对话框模式
	if m.mode == ModeConfirm {
		return m.handleConfirmKey(msg)
	}

	// 输入对话框模式
	if m.mode == ModeInput {
		return m.handleInputKey(msg)
	}

	// 帮助模式：任意键关闭
	if m.mode == ModeHelp {
		if key.Matches(msg, m.keys.Quit) {
			return m, tea.Quit
		}
		m.mode = ModeNormal
		return m, nil
	}

	// 错误模式：任意键关闭
	if m.mode == ModeError {
		// 连接失败时返回会话选择器
		if m.session != nil && !m.connected {
			if m.remoteFS != nil {
				m.remoteFS.Close()
				m.remoteFS = nil
			}
			m.session = nil
			m.mode = ModeSelector
			m.err = nil
			m.statusMsg = "请选择会话"
			m.selector = NewSelector()
			return m, m.selector.Init()
		}
		m.mode = ModeNormal
		m.err = nil
		return m, nil
	}

	// 传输结果通知：任意键关闭
	if m.mode == ModeTransferResult {
		m.mode = ModeNormal
		m.transferResult = nil
		return m, nil
	}

	// 覆盖确认模式
	if m.mode == ModeOverwriteConfirm {
		return m.handleOverwriteConfirmKey(msg)
	}

	// 命令模式
	if m.mode == ModeCommand {
		return m.handleCommandKey(msg)
	}

	// 普通模式
	switch {
	case key.Matches(msg, m.keys.Quit):
		return m, tea.Quit

	case key.Matches(msg, m.keys.Help):
		m.mode = ModeHelp
		return m, nil

	case key.Matches(msg, m.keys.Command):
		m.mode = ModeCommand
		m.cmdInput.SetValue("")
		m.cmdInput.Focus()
		return m, textinput.Blink

	case key.Matches(msg, m.keys.Search):
		m.mode = ModeSearch
		m.searchInput.SetValue("")
		m.searchInput.Focus()
		return m, textinput.Blink

	case key.Matches(msg, m.keys.SwitchPanel):
		return m.switchPanel()

	case key.Matches(msg, m.keys.Yank):
		return m.handleYank()

	case key.Matches(msg, m.keys.Paste):
		return m.handlePaste()

	case key.Matches(msg, m.keys.Delete):
		return m.handleDelete()

	case key.Matches(msg, m.keys.Mkdir):
		return m.handleMkdir()

	case key.Matches(msg, m.keys.Rename):
		return m.handleRename()

	case key.Matches(msg, m.keys.Refresh):
		return m.handleRefresh()

	default:
		// 路由到激活面板
		return m.routeToActivePanel(msg)
	}
}

// handleSearchKey 处理搜索模式下的键盘输入
func (m Model) handleSearchKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEsc:
		// 取消搜索，清除过滤
		m.mode = ModeNormal
		m.searchQuery = ""
		m.searchInput.SetValue("")
		m.searchInput.Blur()
		m.activeFilterPanel().ClearFilter()
		return m, nil

	case tea.KeyEnter:
		// 确认搜索
		m.mode = ModeNormal
		m.searchQuery = m.searchInput.Value()
		m.searchInput.Blur()
		// 过滤已在实时输入时应用
		return m, nil

	default:
		// 更新输入框
		var cmd tea.Cmd
		m.searchInput, cmd = m.searchInput.Update(msg)
		// 实时过滤
		m.searchQuery = m.searchInput.Value()
		m.activeFilterPanel().ApplyFilter(m.searchQuery)
		return m, cmd
	}
}

// handleCommandKey 处理命令模式下的键盘输入
func (m Model) handleCommandKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEsc:
		// 取消命令
		m.mode = ModeNormal
		m.cmdInput.SetValue("")
		m.cmdInput.Blur()
		return m, nil

	case tea.KeyEnter:
		// 执行命令
		cmd := strings.TrimSpace(m.cmdInput.Value())
		m.mode = ModeNormal
		m.cmdInput.Blur()
		return m.executeCommand(cmd)

	default:
		var cmd tea.Cmd
		m.cmdInput, cmd = m.cmdInput.Update(msg)
		return m, cmd
	}
}

// executeCommand 执行命令模式输入的命令
func (m Model) executeCommand(cmd string) (tea.Model, tea.Cmd) {
	switch cmd {
	case "q", "quit":
		// 文件传输模式下返回选择器，而非退出程序
		if m.remoteFS != nil {
			m.remoteFS.Close()
			m.remoteFS = nil
		}
		m.connected = false
		m.session = nil
		m.mode = ModeSelector
		m.selector = NewSelector()
		m.statusMsg = "请选择会话"
		return m, m.selector.Init()

	case "reconnect":
		if m.session == nil {
			m.statusMsg = "无活跃会话"
			return m, nil
		}
		m.statusMsg = "正在重连..."
		s := m.session
		return m, func() tea.Msg {
			remoteFS, err := NewRemoteFS(s)
			if err != nil {
				return ConnectErrMsg{Err: err}
			}
			return reconnectedMsg{RemoteFS: remoteFS}
		}

	default:
		m.statusMsg = fmt.Sprintf("未知命令: %s", cmd)
		return m, nil
	}
}

// activeFilterPanel 返回当前激活面板的指针（用于修改）
func (m *Model) activeFilterPanel() *FilePanel {
	if m.activePanel == PanelLeft {
		return &m.localPanel
	}
	return &m.remotePanel
}

// switchPanel 切换激活面板
func (m Model) switchPanel() (tea.Model, tea.Cmd) {
	if m.activePanel == PanelLeft {
		m.activePanel = PanelRight
	} else {
		m.activePanel = PanelLeft
	}
	return m, nil
}

// routeToActivePanel 将键盘事件路由到激活面板
// handleRefresh 刷新当前激活面板（清除搜索过滤，重新加载目录）
func (m Model) handleRefresh() (tea.Model, tea.Cmd) {
	// 清除搜索状态
	m.searchQuery = ""
	m.searchInput.SetValue("")

	var cmd tea.Cmd
	if m.activePanel == PanelLeft {
		m.localPanel, cmd = m.localPanel.Refresh()
	} else {
		if !m.connected {
			m.statusMsg = "远程面板未连接"
			return m, nil
		}
		m.remotePanel, cmd = m.remotePanel.Refresh()
	}
	m.statusMsg = "正在刷新..."
	return m, cmd
}

func (m Model) routeToActivePanel(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	if m.activePanel == PanelLeft {
		m.localPanel, cmd = m.localPanel.Update(msg)
	} else {
		if !m.connected {
			m.statusMsg = "远程面板未连接"
			return m, nil
		}
		m.remotePanel, cmd = m.remotePanel.Update(msg)
	}
	// 进入目录后自动清除搜索状态（目录切换时 EnterDir/GoParent 会清除 panel.filter）
	panel := m.localPanel
	if m.activePanel == PanelRight {
		panel = m.remotePanel
	}
	if m.searchQuery != "" && panel.filter == "" {
		m.searchQuery = ""
		m.searchInput.SetValue("")
	}
	return m, cmd
}

// handleMouse 处理鼠标事件
func (m Model) handleMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	// 右键菜单模式：处理关闭
	if m.mode == ModeContextMenu {
		if msg.Button == tea.MouseButtonLeft && msg.Action == tea.MouseActionPress {
			m.mode = ModeNormal
			m.contextMenu.visible = false
			return m, nil
		}
		if msg.Button == tea.MouseButtonRight && msg.Action == tea.MouseActionPress {
			m.mode = ModeNormal
			m.contextMenu.visible = false
			// 不 return，继续处理以打开新菜单
		} else {
			return m, nil
		}
	}

	// 确认栏按钮点击检测
	if m.mode == ModeConfirm || m.mode == ModeOverwriteConfirm {
		if msg.Button == tea.MouseButtonLeft && msg.Action == tea.MouseActionPress {
			// 确认栏在底部，检查 Y 坐标是否在确认栏区域
			barY := m.height - 1
			if msg.Y >= barY-1 && msg.Y <= barY {
				// 根据确认消息文本宽度计算按钮位置
				var msgText string
				if m.mode == ModeConfirm {
					if len(m.confirmFiles) == 1 {
						msgText = fmt.Sprintf("确认删除 \"%s\"？", m.confirmFiles[0].Name)
					} else {
						msgText = fmt.Sprintf("确认删除 %d 个文件？", len(m.confirmFiles))
					}
				} else {
					msgText = fmt.Sprintf("目标已存在 %d 个同名文件/目录，是否覆盖？", len(m.overwriteConflicts))
				}
				// 按钮位置：padding(1) + msg + "  " + [Yes(y)] + "  " + [No(n)]
				yesBtn := ConfirmYesBtnStyle.Render("Yes(y)")
				noBtn := ConfirmNoBtnStyle.Render("No(n)")
				yesStart := 1 + lipgloss.Width(msgText) + 2 // padding + msg + gap
				yesEnd := yesStart + lipgloss.Width(yesBtn)
				noStart := yesEnd + 2
				noEnd := noStart + lipgloss.Width(noBtn)

				if msg.X >= yesStart && msg.X < yesEnd {
					// 点击 Yes 按钮
					if m.mode == ModeConfirm {
						return m.executeDelete()
					}
					// overwrite confirm
					dir := m.pendingPasteDir
					destDir := m.pendingPasteDestDir
					m.mode = ModeNormal
					m.overwriteConflicts = nil
					m.pendingPasteDestDir = ""
					return m.executePaste(dir, destDir)
				}
				if msg.X >= noStart && msg.X < noEnd {
					// 点击 No 按钮
					if m.mode == ModeConfirm {
						m.mode = ModeNormal
						m.confirmFiles = nil
						m.statusMsg = "已取消"
					} else {
						m.mode = ModeNormal
						m.overwriteConflicts = nil
						m.pendingPasteDestDir = ""
						m.statusMsg = "已取消"
					}
					return m, nil
				}
				// 点击按钮之外的区域：不做任何操作（安全）
				return m, nil
			}
		}
		return m, nil
	}

	// 可点击关闭的模态（左键点击关闭）
	if msg.Button == tea.MouseButtonLeft && msg.Action == tea.MouseActionPress {
		switch m.mode {
		case ModeHelp:
			m.mode = ModeNormal
			return m, nil
		case ModeError:
			// 连接失败时返回选择器
			if m.session != nil && !m.connected {
				if m.remoteFS != nil {
					m.remoteFS.Close()
					m.remoteFS = nil
				}
				m.session = nil
				m.mode = ModeSelector
				m.err = nil
				m.statusMsg = "请选择会话"
				m.selector = NewSelector()
				return m, m.selector.Init()
			}
			m.mode = ModeNormal
			m.err = nil
			return m, nil
		case ModeTransferResult:
			m.mode = ModeNormal
			m.transferResult = nil
			return m, nil
		}
	}

	// 非普通模式忽略鼠标
	if m.mode != ModeNormal {
		return m, nil
	}

	// 判断点击的面板：X < width/2 → 左面板，否则右面板
	panelWidth := m.width / 2
	var clickedPanel PanelSide
	var panel *FilePanel
	if msg.X < panelWidth {
		clickedPanel = PanelLeft
		panel = &m.localPanel
	} else {
		clickedPanel = PanelRight
		panel = &m.remotePanel
	}

	switch msg.Button {
	case tea.MouseButtonWheelUp:
		// 自动切换到滚轮所在面板
		m.activePanel = clickedPanel
		panel.CursorUp()
		panel.CursorUp()
		panel.CursorUp()
		return m, nil

	case tea.MouseButtonWheelDown:
		m.activePanel = clickedPanel
		panel.CursorDown()
		panel.CursorDown()
		panel.CursorDown()
		return m, nil

	case tea.MouseButtonLeft:
		if msg.Action != tea.MouseActionPress {
			return m, nil
		}

		// 切换到点击的面板
		m.activePanel = clickedPanel

		// 右面板未连接时不处理点击
		if clickedPanel == PanelRight && !m.connected {
			return m, nil
		}

		// 计算面板内 Y 偏移（屏幕坐标 → 文件列表索引）
		fileY := msg.Y - panelHeaderLines

		if fileY < 0 || fileY >= panel.viewHeight() {
			return m, nil
		}

		clickedIndex := panel.offset + fileY

		if clickedIndex >= len(panel.entries) {
			return m, nil
		}

		// 双击检测：同一面板、同一文件索引、400ms 内
		isDoubleClick := clickedPanel == m.lastClickPanel &&
			clickedIndex == m.lastClickIndex &&
			time.Since(m.lastClickTime) < 400*time.Millisecond

		m.lastClickTime = time.Now()
		m.lastClickIndex = clickedIndex
		m.lastClickPanel = clickedPanel

		if isDoubleClick {
			entry := panel.entries[clickedIndex]
			if entry.Info.IsDir {
				panel.cursor = clickedIndex
				panel.ensureVisible()
				var newPanel FilePanel
				var cmd tea.Cmd
				newPanel, cmd = panel.EnterDir()
				if clickedPanel == PanelLeft {
					m.localPanel = newPanel
				} else {
					m.remotePanel = newPanel
				}
				// 清除搜索状态
				if m.searchQuery != "" {
					m.searchQuery = ""
					m.searchInput.SetValue("")
				}
				return m, cmd
			}
			return m, nil
		}

		// Shift+Click：范围选择
		if msg.Shift {
			start := m.selectionAnchor
			end := clickedIndex
			if start > end {
				start, end = end, start
			}
			for i := start; i <= end; i++ {
				if i >= 0 && i < len(panel.entries) {
					panel.entries[i].Selected = true
				}
			}
			panel.cursor = clickedIndex
			panel.ensureVisible()
			return m, nil
		}

		// 普通单击：设置光标和选择锚点
		m.selectionAnchor = clickedIndex
		panel.cursor = clickedIndex
		panel.ensureVisible()
		return m, nil

	case tea.MouseButtonRight:
		if msg.Action != tea.MouseActionPress {
			return m, nil
		}
		if m.mode != ModeNormal {
			return m, nil
		}
		m.activePanel = clickedPanel
		if clickedPanel == PanelRight && !m.connected {
			return m, nil
		}
		fileY := msg.Y - panelHeaderLines
		if fileY < 0 || fileY >= panel.viewHeight() {
			return m, nil
		}
		clickedIndex := panel.offset + fileY
		if clickedIndex >= len(panel.entries) {
			return m, nil
		}
		panel.cursor = clickedIndex
		panel.ensureVisible()

		var items []ContextMenuItem
		items = append(items, ContextMenuItem{Label: "复制", Key: "y", Action: "yank"})
		if len(m.yankFiles) > 0 {
			items = append(items, ContextMenuItem{Label: "粘贴", Key: "p", Action: "paste"})
		}
		items = append(items, ContextMenuItem{Label: "删除", Key: "D", Action: "delete"})
		items = append(items, ContextMenuItem{Label: "新建目录", Key: "m", Action: "mkdir"})
		items = append(items, ContextMenuItem{Label: "重命名", Key: "r", Action: "rename"})

		m.contextMenu = ContextMenu{visible: true, items: items, cursor: 0}
		m.mode = ModeContextMenu
		return m, nil
	}

	return m, nil
}

// handleContextMenuKey 处理右键菜单模式的键盘输入
func (m Model) handleContextMenuKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case msg.Type == tea.KeyEsc:
		m.mode = ModeNormal
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
	m.mode = ModeNormal
	m.contextMenu.visible = false
	return m, nil
}

// executeContextMenuAction 执行右键菜单选中的操作
func (m Model) executeContextMenuAction() (tea.Model, tea.Cmd) {
	if m.contextMenu.cursor < 0 || m.contextMenu.cursor >= len(m.contextMenu.items) {
		m.mode = ModeNormal
		m.contextMenu.visible = false
		return m, nil
	}
	action := m.contextMenu.items[m.contextMenu.cursor].Action
	m.mode = ModeNormal
	m.contextMenu.visible = false
	switch action {
	case "yank":
		return m.handleYank()
	case "paste":
		return m.handlePaste()
	case "delete":
		return m.handleDelete()
	case "mkdir":
		return m.handleMkdir()
	case "rename":
		return m.handleRename()
	}
	return m, nil
}

// renderContextMenuBar 渲染右键上下文菜单栏
func (m Model) renderContextMenuBar() string {
	var parts []string
	for i, item := range m.contextMenu.items {
		label := fmt.Sprintf("%s(%s)", item.Label, item.Key)
		if i == m.contextMenu.cursor {
			parts = append(parts, ContextMenuActiveStyle.Render(label))
		} else {
			parts = append(parts, ContextMenuItemStyle.Render(label))
		}
	}
	bar := strings.Join(parts, "  ")
	return ContextMenuBarStyle.Width(m.width).Render(bar)
}

// handleConnected 处理连接成功
func (m Model) handleConnected(msg ConnectedMsg) (tea.Model, tea.Cmd) {
	m.remoteFS = msg.RemoteFS
	m.connected = true
	m.statusMsg = "已连接"

	// 获取远程初始目录
	remoteCwd, err := msg.RemoteFS.Getwd()
	if err != nil {
		remoteCwd = "/"
	}

	// 初始化远程面板
	m.remotePanel = NewFilePanel(PanelRight, msg.RemoteFS, remoteCwd)
	m.updatePanelSizes()

	return m, m.remotePanel.LoadDir()
}

// handleDirLoaded 处理目录加载完成
func (m Model) handleDirLoaded(msg DirLoadedMsg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	if msg.Panel == PanelLeft {
		m.localPanel, cmd = m.localPanel.Update(msg)
	} else {
		m.remotePanel, cmd = m.remotePanel.Update(msg)
	}
	return m, cmd
}

// handleDirLoadErr 处理目录加载失败
func (m Model) handleDirLoadErr(msg DirLoadErrMsg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	if msg.Panel == PanelLeft {
		m.localPanel, cmd = m.localPanel.Update(msg)
	} else {
		m.remotePanel, cmd = m.remotePanel.Update(msg)
	}
	m.statusMsg = fmt.Sprintf("目录加载失败: %v", msg.Err)
	return m, cmd
}

// updatePanelSizes 根据窗口大小更新面板尺寸
func (m *Model) updatePanelSizes() {
	if m.width == 0 || m.height == 0 {
		return
	}
	// 面板各占一半宽度
	panelWidth := m.width / 2
	// 高度：减去状态栏(1)，如果有活跃传输再减去传输条(1)
	reserved := 1
	if m.transfer != nil && m.transfer.ActiveTask() != nil {
		reserved = 2
	}
	panelHeight := m.height - reserved - 2 // 减去面板边框高度（RoundedBorder 上+下）
	if panelHeight < 3 {
		panelHeight = 3
	}
	m.localPanel.SetSize(panelWidth, panelHeight)
	m.remotePanel.SetSize(m.width-panelWidth, panelHeight)
}

// View 渲染界面（Bubble Tea 接口）
func (m Model) View() string {
	if m.width == 0 || m.height == 0 {
		return "Loading..."
	}

	// 选择器模式
	if m.mode == ModeSelector {
		return m.selector.View(m.width, m.height)
	}

	// 帮助模式
	if m.mode == ModeHelp {
		return m.renderHelp()
	}

	// 错误弹窗
	if m.mode == ModeError {
		return m.renderError()
	}

	// 传输结果通知
	if m.mode == ModeTransferResult {
		return m.renderTransferResult()
	}

	// 渲染双面板
	leftView := m.renderPanel(PanelLeft)
	rightView := m.renderPanel(PanelRight)

	// 水平拼接两个面板
	panels := lipgloss.JoinHorizontal(lipgloss.Top, leftView, rightView)

	// 状态栏
	statusBar := m.renderStatusBar()

	// 命令模式显示命令栏
	if m.mode == ModeCommand {
		cmdBar := m.renderCmdBar()
		return lipgloss.JoinVertical(lipgloss.Left, panels, statusBar, cmdBar)
	}

	// 搜索模式显示搜索栏
	if m.mode == ModeSearch {
		searchBar := m.renderSearchBar()
		return lipgloss.JoinVertical(lipgloss.Left, panels, statusBar, searchBar)
	}

	// 确认对话框
	if m.mode == ModeConfirm {
		confirmBar := m.renderConfirmBar()
		return lipgloss.JoinVertical(lipgloss.Left, panels, statusBar, confirmBar)
	}

	// 覆盖确认对话框
	if m.mode == ModeOverwriteConfirm {
		overwriteBar := m.renderOverwriteConfirmBar()
		return lipgloss.JoinVertical(lipgloss.Left, panels, statusBar, overwriteBar)
	}

	// 输入对话框
	if m.mode == ModeInput {
		inputBar := m.renderInputBar()
		return lipgloss.JoinVertical(lipgloss.Left, panels, statusBar, inputBar)
	}

	// 右键上下文菜单
	if m.mode == ModeContextMenu && m.contextMenu.visible {
		menuBar := m.renderContextMenuBar()
		return lipgloss.JoinVertical(lipgloss.Left, panels, statusBar, menuBar)
	}

	// 传输进度条（如果有活跃传输）
	transferBar := m.renderTransferBar()
	if transferBar != "" {
		return lipgloss.JoinVertical(lipgloss.Left, panels, transferBar, statusBar)
	}

	// 垂直拼接
	return lipgloss.JoinVertical(lipgloss.Left, panels, statusBar)
}

// renderSearchBar 渲染搜索栏
func (m Model) renderSearchBar() string {
	searchWithHint := m.searchInput.View() + "  (Esc:取消 Enter:确认)"
	return SearchStyle.Width(m.width).Render(searchWithHint)
}

// renderConfirmBar 渲染确认对话框
func (m Model) renderConfirmBar() string {
	var msg string
	if len(m.confirmFiles) == 1 {
		msg = fmt.Sprintf("确认删除 \"%s\"？", m.confirmFiles[0].Name)
	} else {
		msg = fmt.Sprintf("确认删除 %d 个文件？", len(m.confirmFiles))
	}
	yesBtn := ConfirmYesBtnStyle.Render("Yes(y)")
	noBtn := ConfirmNoBtnStyle.Render("No(n)")
	bar := msg + "  " + yesBtn + "  " + noBtn
	return ConfirmMsgStyle.Width(m.width).Padding(0, 1).Render(bar)
}

// renderOverwriteConfirmBar 渲染覆盖确认对话框
func (m Model) renderOverwriteConfirmBar() string {
	msg := fmt.Sprintf("目标已存在 %d 个同名文件/目录，是否覆盖？", len(m.overwriteConflicts))
	yesBtn := ConfirmYesBtnStyle.Render("Yes(y)")
	noBtn := ConfirmNoBtnStyle.Render("No(n)")
	bar := msg + "  " + yesBtn + "  " + noBtn
	return ConfirmMsgStyle.Width(m.width).Padding(0, 1).Render(bar)
}

// renderInputBar 渲染输入对话框
func (m Model) renderInputBar() string {
	inputWithHint := m.opInput.View() + "  (Esc:取消 Enter:确认)"
	return SearchStyle.Width(m.width).Render(inputWithHint)
}

// renderCmdBar 渲染命令栏
func (m Model) renderCmdBar() string {
	cmdWithHint := m.cmdInput.View() + "  " + CmdHintStyle.Render("(q:退出 reconnect:重连)")
	return SearchStyle.Width(m.width).Render(cmdWithHint)
}

// renderPanel 渲染单个面板（含边框）
func (m Model) renderPanel(side PanelSide) string {
	var panel FilePanel
	if side == PanelLeft {
		panel = m.localPanel
	} else {
		panel = m.remotePanel
	}

	content := panel.View()

	// 根据激活状态选择边框样式
	var style lipgloss.Style
	if side == m.activePanel {
		style = ActivePanelStyle
	} else {
		style = InactivePanelStyle
	}

	return style.Width(panel.width - 2).Height(panel.height).Render(content)
}

// renderStatusBar 渲染状态栏
func (m Model) renderStatusBar() string {
	// 左侧：连接信息 + 活跃面板路径和文件计数
	var left string
	if m.session != nil {
		left = fmt.Sprintf(" %s@%s:%d",
			m.session.User,
			m.session.Host,
			m.session.Port,
		)
	}

	// 活跃面板信息
	var panel *FilePanel
	if m.activePanel == PanelLeft {
		panel = &m.localPanel
	} else {
		panel = &m.remotePanel
	}
	fileCount := len(panel.entries)
	selectedCount := len(panel.SelectedFiles())
	if selectedCount > 0 {
		left += fmt.Sprintf(" | %s [%d/%d]", panel.cwd, selectedCount, fileCount)
	} else {
		left += fmt.Sprintf(" | %s [%d]", panel.cwd, fileCount)
	}

	// 右侧：状态信息 + 帮助提示
	right := fmt.Sprintf(" %s | ?:Help :q:Quit ", m.statusMsg)

	// 可用内容宽度（减去 StatusBarStyle 的左右 Padding 各 1 字符）
	contentWidth := m.width - 2
	// 如果左右合计超出宽度，截断左侧路径部分
	gap := contentWidth - lipgloss.Width(left) - lipgloss.Width(right)
	if gap < 0 {
		maxLeft := contentWidth - lipgloss.Width(right)
		if maxLeft < 0 {
			maxLeft = 0
		}
		if len(left) > maxLeft {
			if maxLeft > 3 {
				left = left[:maxLeft-3] + "..."
			} else {
				left = left[:maxLeft]
			}
		}
		gap = 0
	}

	bar := left + strings.Repeat(" ", gap) + right

	return StatusBarStyle.Width(m.width).Render(bar)
}

// renderHelp 渲染帮助视图
func (m Model) renderHelp() string {
	sections := []struct {
		title string
		keys  [][2]string
	}{
		{
			title: "导航",
			keys: [][2]string{
				{"j/k", "上下移动"},
				{"h/l", "折叠/展开目录"},
				{"Ctrl+u/d", "半页滚动"},
				{"gg/G", "跳顶/跳底"},
				{"Tab", "切换面板"},
				{"Enter", "进入目录"},
				{"Backspace", "返回上级"},
			},
		},
		{
			title: "文件操作",
			keys: [][2]string{
				{"Space", "多选/取消"},
				{"y", "标记传输"},
				{"p", "粘贴/传输"},
				{"D", "删除"},
				{"r", "重命名"},
				{"m", "创建目录"},
				{"R", "刷新面板"},
			},
		},
		{
			title: "命令模式（: 进入）",
			keys: [][2]string{
				{":q / :quit", "退出程序"},
				{":reconnect", "重新连接远程"},
			},
		},
		{
			title: "其他",
			keys: [][2]string{
				{"?", "帮助"},
				{"/", "搜索（实时过滤）"},
				{":", "进入命令模式"},
				{"Ctrl+c", "退出"},
			},
		},
	}

	var lines []string
	lines = append(lines, HelpSectionStyle.Render("xftp 帮助"))
	lines = append(lines, "")

	for _, sec := range sections {
		lines = append(lines, HelpSectionStyle.Render(sec.title))
		for _, k := range sec.keys {
			line := HelpKeyStyle.Render(k[0]) + HelpDescStyle.Render(k[1])
			lines = append(lines, line)
		}
		lines = append(lines, "")
	}

	lines = append(lines, lipgloss.NewStyle().
		Foreground(lipgloss.Color(colorFgDim)).
		Render("按任意键返回..."))

	return HelpContainerStyle.Render(strings.Join(lines, "\n"))
}

// renderError 渲染错误弹窗
func (m Model) renderError() string {
	errMsg := "未知错误"
	if m.err != nil {
		errMsg = m.err.Error()
	}

	hint := "按任意键返回..."
	if m.session != nil && !m.connected {
		hint = "按任意键返回会话列表..."
	}

	style := lipgloss.NewStyle().
		Foreground(lipgloss.Color(colorRed)).
		Background(lipgloss.Color(colorBg)).
		Padding(1).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color(colorRed))

	return style.Render(errMsg + "\n\n" + hint)
}

// renderTransferResult 渲染传输结果通知
func (m Model) renderTransferResult() string {
	r := m.transferResult
	if r == nil {
		return ""
	}

	var lines []string
	lines = append(lines, "传输完成！")
	lines = append(lines, "")
	if r.Dirs > 0 {
		lines = append(lines, fmt.Sprintf("  目录: %d 个", r.Dirs))
	}
	lines = append(lines, fmt.Sprintf("  文件: %d 个", r.Files))
	lines = append(lines, fmt.Sprintf("  总计: %s", formatSize(r.TotalBytes)))
	if r.Failed > 0 {
		lines = append(lines, fmt.Sprintf("  失败: %d 个", r.Failed))
	}
	lines = append(lines, "")
	lines = append(lines, "按任意键继续...")

	style := lipgloss.NewStyle().
		Foreground(lipgloss.Color(colorGreen)).
		Background(lipgloss.Color(colorBg)).
		Padding(1).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color(colorGreen))

	if r.Failed > 0 {
		style = style.
			Foreground(lipgloss.Color(colorOrange)).
			BorderForeground(lipgloss.Color(colorOrange))
	}

	return style.Render(strings.Join(lines, "\n"))
}

// handleSessionSelected 处理用户选择 session 后的状态转换
func (m Model) handleSessionSelected(s *session.Session) (tea.Model, tea.Cmd) {
	m.session = s
	m.mode = ModeNormal
	m.statusMsg = "正在连接..."

	// 初始化本地文件系统和面板
	localFS, err := NewLocalFS()
	var localDir string
	if err != nil {
		localDir = "/"
	} else {
		localDir, _ = localFS.Getwd()
	}

	if err != nil {
		m.localPanel = NewFilePanel(PanelLeft, nil, localDir)
		m.localPanel.err = err
	} else {
		m.localPanel = NewFilePanel(PanelLeft, localFS, localDir)
	}

	m.remotePanel = NewFilePanel(PanelRight, nil, "/")
	m.activePanel = PanelLeft
	m.transfer = NewTransferManager()
	m.updatePanelSizes()

	return m, tea.Batch(
		m.localPanel.LoadDir(),
		m.connectRemote(),
	)
}

// Run 启动 xftp TUI 程序
func Run(s *session.Session) error {
	m := NewModel(s)
	p := tea.NewProgram(m, tea.WithAltScreen(), tea.WithMouseCellMotion())

	finalModel, err := p.Run()
	if err != nil {
		return fmt.Errorf("TUI 运行失败: %w", err)
	}

	// 清理远程连接
	if fm, ok := finalModel.(Model); ok && fm.remoteFS != nil {
		fm.remoteFS.Close()
	}

	return nil
}
