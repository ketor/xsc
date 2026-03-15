package xftp

import "github.com/charmbracelet/lipgloss"

// Gruvbox 色彩常量
const (
	colorBg      = "#282828"
	colorBgAlt   = "#3c3836"
	colorBgPanel = "#504945"
	colorFg      = "#ebdbb2"
	colorFgDim   = "#a89984"
	colorFgDark  = "#665c54"
	colorYellow  = "#fabd2f"
	colorGreen   = "#b8bb26"
	colorBlue    = "#83a598"
	colorRed     = "#fb4934"
	colorOrange  = "#fe8019"
	colorPurple  = "#d3869b"
	colorAqua    = "#8ec07c"
)

// 面板边框样式
var (
	// 激活面板边框
	ActivePanelStyle = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(lipgloss.Color(colorYellow))

	// 非激活面板边框
	InactivePanelStyle = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(lipgloss.Color(colorFgDark))

	// 面板标题样式
	PanelTitleStyle = lipgloss.NewStyle().
			Background(lipgloss.Color(colorBgPanel)).
			Foreground(lipgloss.Color(colorFg)).
			Bold(true).
			Padding(0, 1)

	// 面板标题（激活）
	PanelTitleActiveStyle = lipgloss.NewStyle().
				Background(lipgloss.Color(colorBgPanel)).
				Foreground(lipgloss.Color(colorYellow)).
				Bold(true).
				Padding(0, 1)
)

// 文件/目录样式
var (
	// 目录样式
	DirStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color(colorBlue)).
			Bold(true)

	// 普通文件样式
	FileStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color(colorFg))

	// 符号链接样式
	SymlinkStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color(colorAqua))

	// 可执行文件样式
	ExecStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color(colorGreen)).
			Bold(true)

	// 隐藏文件样式（.开头）
	HiddenStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color(colorFgDim))
)

// 选中/光标样式
var (
	// 光标所在行
	CursorStyle = lipgloss.NewStyle().
			Background(lipgloss.Color(colorBgAlt)).
			Foreground(lipgloss.Color(colorYellow)).
			Bold(true)

	// 多选标记的文件
	SelectedStyle = lipgloss.NewStyle().
			Background(lipgloss.Color(colorBgAlt)).
			Foreground(lipgloss.Color(colorOrange)).
			Bold(true)

	// yank 标记的文件
	YankedStyle = lipgloss.NewStyle().
			Background(lipgloss.Color(colorBgAlt)).
			Foreground(lipgloss.Color(colorPurple)).
			Bold(true)
)

// 状态栏样式
var (
	StatusBarStyle = lipgloss.NewStyle().
			Background(lipgloss.Color(colorBgAlt)).
			Foreground(lipgloss.Color(colorFgDim)).
			Padding(0, 1)

	StatusBarKeyStyle = lipgloss.NewStyle().
				Background(lipgloss.Color(colorBgAlt)).
				Foreground(lipgloss.Color(colorBlue))

	StatusBarValueStyle = lipgloss.NewStyle().
				Background(lipgloss.Color(colorBgAlt)).
				Foreground(lipgloss.Color(colorYellow))

	// 错误提示
	StatusErrStyle = lipgloss.NewStyle().
			Background(lipgloss.Color(colorRed)).
			Foreground(lipgloss.Color(colorFg)).
			Padding(0, 1)

	// 成功提示
	StatusOkStyle = lipgloss.NewStyle().
			Background(lipgloss.Color(colorGreen)).
			Foreground(lipgloss.Color(colorBg)).
			Padding(0, 1)
)

// 传输面板样式
var (
	// 传输面板边框
	TransferPanelStyle = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(lipgloss.Color(colorFgDark))

	// 进度条（已完成部分）
	ProgressFilledStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color(colorGreen))

	// 进度条（未完成部分）
	ProgressEmptyStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color(colorFgDark))

	// 传输完成
	TransferDoneStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color(colorGreen))

	// 传输失败
	TransferFailStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color(colorRed))

	// 传输速度
	TransferSpeedStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color(colorFgDim))
)

// 搜索和命令行样式
var (
	SearchStyle = lipgloss.NewStyle().
			Background(lipgloss.Color(colorBg)).
			Foreground(lipgloss.Color(colorFg)).
			Padding(0, 1)

	CmdHintStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color(colorFgDark))

	CmdHintActiveStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color(colorYellow))
)

// 帮助视图样式
var (
	HelpSectionStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color(colorYellow)).
				Bold(true)

	HelpKeyStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color(colorBlue)).
			Width(16)

	HelpDescStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color(colorFg))

	HelpContainerStyle = lipgloss.NewStyle().
				Padding(1, 2)
)

// 确认对话框样式
var (
	ConfirmBoxStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color(colorYellow)).
			Padding(1, 2)

	ConfirmTitleStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color(colorYellow)).
				Bold(true)

	ConfirmMsgStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color(colorFg))

	ConfirmYesBtnStyle = lipgloss.NewStyle().
				Background(lipgloss.Color(colorGreen)).
				Foreground(lipgloss.Color(colorBg)).
				Bold(true).
				Padding(0, 1)

	ConfirmNoBtnStyle = lipgloss.NewStyle().
				Background(lipgloss.Color(colorRed)).
				Foreground(lipgloss.Color(colorFg)).
				Bold(true).
				Padding(0, 1)
)

// 文件权限样式
var (
	PermReadStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color(colorYellow))

	PermWriteStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color(colorRed))

	PermExecStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color(colorGreen))

	PermNoneStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color(colorFgDark))
)

// 文件大小样式
var (
	FileSizeStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color(colorFgDim)).
			Width(8).
			Align(lipgloss.Right)

	FileTimeStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color(colorFgDark))
)

// 选择器布局样式
var (
	// 选择器树面板
	selectorTreeStyle = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(lipgloss.Color(colorYellow)).
				Padding(0, 1)

	// 选择器详情面板
	selectorDetailStyle = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(lipgloss.Color(colorFgDark)).
				Padding(1)

	// 详情面板标题
	SelectorDetailTitleStyle = lipgloss.NewStyle().
					Background(lipgloss.Color(colorBgPanel)).
					Foreground(lipgloss.Color(colorFg)).
					Bold(true).
					Padding(0, 1)

	// 详情面板键名
	SelectorDetailKeyStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color(colorBlue))

	// 详情面板值
	SelectorDetailValueStyle = lipgloss.NewStyle().
					Foreground(lipgloss.Color(colorYellow))

	// 外部来源文件样式
	securecrtFileStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color(colorPurple))

	xshellFileStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color(colorAqua))

	mobaxtermFileStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color(colorOrange))
)

// 右键上下文菜单样式
var (
	ContextMenuBarStyle = lipgloss.NewStyle().
				Background(lipgloss.Color(colorBgAlt)).
				Padding(0, 1)

	ContextMenuItemStyle = lipgloss.NewStyle().
				Background(lipgloss.Color(colorBgPanel)).
				Foreground(lipgloss.Color(colorFg))

	ContextMenuActiveStyle = lipgloss.NewStyle().
				Background(lipgloss.Color(colorYellow)).
				Foreground(lipgloss.Color(colorBg)).
				Bold(true)
)
