package xftp

import "github.com/charmbracelet/bubbles/key"

// KeyMap 定义 xftp 的快捷键映射
type KeyMap struct {
	// 导航
	Up           key.Binding
	Down         key.Binding
	HalfPageUp   key.Binding
	HalfPageDown key.Binding
	PageUp       key.Binding
	PageDown     key.Binding
	GoToTop      key.Binding
	GoToBottom   key.Binding

	// 面板
	SwitchPanel key.Binding

	// 目录导航
	Enter         key.Binding // 进入目录
	Backspace     key.Binding // 返回上级
	OpenFold      key.Binding // 展开（l）
	CloseFold     key.Binding // 折叠（h）
	ToggleFold    key.Binding // 切换展开/折叠（o）
	OpenAllFolds  key.Binding // 展开所有（E）
	CloseAllFolds key.Binding // 折叠所有（C）

	// 文件操作
	Refresh key.Binding // 刷新当前面板
	Yank    key.Binding // 标记（yank）
	Paste  key.Binding // 粘贴/传输
	Delete key.Binding // 删除
	Rename key.Binding // 重命名
	Mkdir  key.Binding // 创建目录
	Select key.Binding // 多选/取消选择

	// 全局
	Help    key.Binding
	Command key.Binding // : 命令模式
	Search  key.Binding
	Quit    key.Binding
}

// DefaultKeyMap 返回默认快捷键配置
func DefaultKeyMap() KeyMap {
	return KeyMap{
		Up: key.NewBinding(
			key.WithKeys("up", "k"),
			key.WithHelp("↑/k", "上移"),
		),
		Down: key.NewBinding(
			key.WithKeys("down", "j"),
			key.WithHelp("↓/j", "下移"),
		),
		HalfPageUp: key.NewBinding(
			key.WithKeys("ctrl+u"),
			key.WithHelp("C-u", "半页上滚"),
		),
		HalfPageDown: key.NewBinding(
			key.WithKeys("ctrl+d"),
			key.WithHelp("C-d", "半页下滚"),
		),
		PageUp: key.NewBinding(
			key.WithKeys("pgup", "ctrl+b"),
			key.WithHelp("PgUp/C-b", "向上翻页"),
		),
		PageDown: key.NewBinding(
			key.WithKeys("pgdown", "ctrl+f"),
			key.WithHelp("PgDn/C-f", "向下翻页"),
		),
		GoToTop: key.NewBinding(
			key.WithKeys("home", "g"),
			key.WithHelp("Home/g", "跳顶"),
		),
		GoToBottom: key.NewBinding(
			key.WithKeys("end", "G"),
			key.WithHelp("End/G", "跳底"),
		),
		SwitchPanel: key.NewBinding(
			key.WithKeys("tab"),
			key.WithHelp("Tab", "切换面板"),
		),
		Enter: key.NewBinding(
			key.WithKeys("enter"),
			key.WithHelp("Enter", "进入目录"),
		),
		Backspace: key.NewBinding(
			key.WithKeys("backspace"),
			key.WithHelp("BS", "返回上级"),
		),
		OpenFold: key.NewBinding(
			key.WithKeys("l", "right"),
			key.WithHelp("l/→", "展开"),
		),
		CloseFold: key.NewBinding(
			key.WithKeys("h", "left"),
			key.WithHelp("h/←", "折叠"),
		),
		ToggleFold: key.NewBinding(
			key.WithKeys("o"),
			key.WithHelp("o", "切换展开/折叠"),
		),
		OpenAllFolds: key.NewBinding(
			key.WithKeys("E"),
			key.WithHelp("E", "展开所有"),
		),
		CloseAllFolds: key.NewBinding(
			key.WithKeys("C"),
			key.WithHelp("C", "折叠所有"),
		),
		Refresh: key.NewBinding(
			key.WithKeys("R"),
			key.WithHelp("R", "刷新面板"),
		),
		Yank: key.NewBinding(
			key.WithKeys("y"),
			key.WithHelp("y", "标记传输"),
		),
		Paste: key.NewBinding(
			key.WithKeys("p"),
			key.WithHelp("p", "粘贴/传输"),
		),
		Delete: key.NewBinding(
			key.WithKeys("D"),
			key.WithHelp("D", "删除"),
		),
		Rename: key.NewBinding(
			key.WithKeys("r"),
			key.WithHelp("r", "重命名"),
		),
		Mkdir: key.NewBinding(
			key.WithKeys("m"),
			key.WithHelp("m", "新建目录"),
		),
		Select: key.NewBinding(
			key.WithKeys(" "),
			key.WithHelp("Space", "多选"),
		),
		Help: key.NewBinding(
			key.WithKeys("?"),
			key.WithHelp("?", "帮助"),
		),
		Command: key.NewBinding(
			key.WithKeys(":"),
			key.WithHelp(":", "命令模式"),
		),
		Search: key.NewBinding(
			key.WithKeys("/"),
			key.WithHelp("/", "搜索"),
		),
		Quit: key.NewBinding(
			key.WithKeys("ctrl+c"),
			key.WithHelp("C-c/:q", "退出"),
		),
	}
}
