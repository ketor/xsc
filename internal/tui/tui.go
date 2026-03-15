package tui

import (
	"fmt"
	"os"
	"time"

	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/ketor/xsc/internal/session"
	"github.com/ketor/xsc/internal/shared"
	internalssh "github.com/ketor/xsc/internal/ssh"
)

// 样式定义
var (
	// 树形结构样式
	treeStyle = lipgloss.NewStyle().
			Padding(0, 1)

	selectedStyle = lipgloss.NewStyle().
			Background(lipgloss.Color("#3c3836")).
			Foreground(lipgloss.Color("#fabd2f")).
			Bold(true)

	folderStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#83a598")).
			Bold(true)

	fileStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#b8bb26"))

	invalidStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#fb4934"))

	// SecureCRT 样式（使用紫色系区分）
	securecrtFolderStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#b16286")).
				Bold(true)

	securecrtFileStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#d3869b"))

	// XShell 样式（使用青色系区分）
	xshellFolderStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#458588")).
				Bold(true)

	xshellFileStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#8ec07c"))

	// MobaXterm 样式（使用橙色系区分）
	mobaxtermFolderStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#d65d0e")).
				Bold(true)

	mobaxtermFileStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#fe8019"))

	lineNumberStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#665c54")).
			Width(4).
			Align(lipgloss.Right)

	// 详情面板样式
	detailTitleStyle = lipgloss.NewStyle().
				Background(lipgloss.Color("#504945")).
				Foreground(lipgloss.Color("#ebdbb2")).
				Bold(true).
				Padding(0, 1)

	detailContentStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#ebdbb2")).
				Padding(1)

	detailBoxStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("#665c54")).
			Padding(1)

	detailKeyStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#83a598"))

	detailValueStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#fabd2f"))

	// 状态栏样式
	statusBarStyle = lipgloss.NewStyle().
			Background(lipgloss.Color("#3c3836")).
			Foreground(lipgloss.Color("#a89984")).
			Padding(0, 1)

	// 搜索框样式
	searchStyle = lipgloss.NewStyle().
			Background(lipgloss.Color("#282828")).
			Foreground(lipgloss.Color("#ebdbb2")).
			Padding(0, 1)

	// 命令补全提示样式
	cmdHintStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#665c54"))

	cmdHintActiveStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#fabd2f"))

	// 帮助视图样式
	helpSectionStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#fabd2f")).
				Bold(true)

	helpKeyStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#83a598")).
			Width(16)

	helpDescStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#ebdbb2"))

	helpContainerStyle = lipgloss.NewStyle().
				Padding(1, 2)
)

// Command 是 shared.Command 的类型别名，保持向后兼容
type Command = shared.Command

// commands 是所有 : 命令的注册表（单一数据源）
var commands = []Command{
	{Name: "q", Aliases: []string{"quit"}, Description: "退出程序"},
	{Name: "noh", Aliases: []string{"nohlsearch"}, Description: "清除搜索高亮/过滤"},
	{Name: "pw", Aliases: []string{"password"}, Description: "切换密码明文显示"},
}

// AgentKeyCache SSH Agent keys 缓存
type AgentKeyCache struct {
	keys      []internalssh.AgentKeyInfo
	err       error
	timestamp int64
}

// agentKeysLoadedMsg SSH Agent keys 加载完成消息
type agentKeysLoadedMsg struct {
	keys []internalssh.AgentKeyInfo
	err  error
}

// loadAgentKeys 异步加载 SSH Agent keys
func loadAgentKeys() tea.Msg {
	keys, err := internalssh.ListAgentKeys()
	return agentKeysLoadedMsg{keys: keys, err: err}
}

// 消息类型
type connectCompleteMsg struct {
	err error
}

// showErrorMsg 显示错误消息
type showErrorMsg struct {
	err error
}

// sessionsLoadedMsg 会话加载完成消息
type sessionsLoadedMsg struct {
	tree        *session.SessionNode
	sessionsDir string
}

// editorCompleteMsg 编辑器完成消息
type editorCompleteMsg struct {
	err error
}

// prepareNewSessionMsg 触发进入新建会话模式的消息
type prepareNewSessionMsg struct {
	dir string
}

// prepareRenameSessionMsg 触发进入重命名会话模式的消息
type prepareRenameSessionMsg struct {
	node *session.SessionNode
}

// prepareDeleteConfirmMsg 触发进入删除确认模式的消息
type prepareDeleteConfirmMsg struct {
	node *session.SessionNode
}

// newSessionEditorCompleteMsg 新建会话编辑器完成消息
type newSessionEditorCompleteMsg struct {
	err        error
	tempPath   string
	targetPath string
}

// ContextMenuItem 右键菜单项
type ContextMenuItem struct {
	Label  string
	Key    string
	Action string
}

// ContextMenu 右键上下文菜单状态
type ContextMenu struct {
	visible bool
	items   []ContextMenuItem
	cursor  int
	node    *session.SessionNode
}

// Model 是 TUI 的模型
type Model struct {
	keys          KeyMap
	help          help.Model
	tree          *session.SessionNode
	cursor        int
	width         int
	height        int
	sessionsDir   string
	searchInput   textinput.Model
	searchMode    bool
	searchQuery   string
	lineNumInput  textinput.Model
	lineNumMode   bool
	lineNumBuffer string
	detailView    viewport.Model
	showHelp      bool
	showError     bool
	errorMessage  string
	agentKeyCache *AgentKeyCache
	lastKeyG      bool // 用于检测 'gg' 快捷键
	showPassword  bool // 是否显示密码明文，默认隐藏

	// 新建会话相关字段
	newSessionMode  bool            // 是否处于新建会话的文件名输入模式
	newSessionInput textinput.Model // 文件名输入框
	newSessionDir   string          // 新会话要保存的目录

	// 重命名会话相关字段
	renameMode       bool                 // 是否处于重命名会话的文件名输入模式
	renameInput      textinput.Model      // 新文件名输入框
	renameTargetNode *session.SessionNode // 要重命名的目标节点

	// 删除会话确认相关字段
	deleteConfirmMode  bool                 // 是否处于删除确认模式
	deleteConfirmInput textinput.Model      // 确认输入框
	deleteTargetNode   *session.SessionNode // 要删除的目标节点

	// 鼠标双击检测相关字段
	lastClickTime  time.Time // 上次点击时间
	lastClickIndex int       // 上次点击的节点索引

	// 右键上下文菜单
	contextMenu ContextMenu
}

// 初始化 Model
func initialModel() Model {
	keys := DefaultKeyMap()

	// 初始化搜索输入框
	searchInput := textinput.New()
	searchInput.Placeholder = "Search..."
	searchInput.Prompt = "/"
	searchInput.CharLimit = 50
	searchInput.Width = 30

	// 初始化行号输入框
	lineNumInput := textinput.New()
	lineNumInput.Placeholder = ""
	lineNumInput.Prompt = ":"
	lineNumInput.CharLimit = 20
	lineNumInput.Width = 20

	// 初始化新建会话文件名输入框
	newSessionInput := textinput.New()
	newSessionInput.Placeholder = "session-name"
	newSessionInput.Prompt = "Name: "
	newSessionInput.CharLimit = 50
	newSessionInput.Width = 30

	// 初始化重命名会话文件名输入框
	renameInput := textinput.New()
	renameInput.Placeholder = "new-name"
	renameInput.Prompt = "Rename to: "
	renameInput.CharLimit = 50
	renameInput.Width = 30

	// 初始化删除确认输入框
	deleteConfirmInput := textinput.New()
	deleteConfirmInput.Placeholder = ""
	deleteConfirmInput.Prompt = "Type YES to confirm: "
	deleteConfirmInput.CharLimit = 10
	deleteConfirmInput.Width = 30

	return Model{
		keys:               keys,
		help:               help.New(),
		searchInput:        searchInput,
		lineNumInput:       lineNumInput,
		newSessionInput:    newSessionInput,
		renameInput:        renameInput,
		deleteConfirmInput: deleteConfirmInput,
	}
}

// Init 初始化 Bubble Tea 程序
func (m Model) Init() tea.Cmd {
	return tea.Batch(
		m.loadSessions(),
		tea.EnterAltScreen,
		loadAgentKeys,
	)
}

// Run 启动 TUI
func Run() {
	p := tea.NewProgram(initialModel(), tea.WithAltScreen(), tea.WithMouseCellMotion())
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
