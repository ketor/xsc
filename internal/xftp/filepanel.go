package xftp

import (
	"fmt"
	"os"
	"path"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// panelHeaderLines 是文件面板中文件列表上方的行数（屏幕坐标偏移）
// 包含：外部 border 顶边(1) + 标题行(1) + 表头行(1) = 3
// 用于鼠标点击时将屏幕 Y 坐标转换为文件列表索引
const panelHeaderLines = 3

// FileEntry 文件列表条目（含选中状态）
type FileEntry struct {
	Info     FileInfo
	Selected bool
}

// FilePanel 文件面板子组件（本地/远程共用）
// 实现独立的 Update/View，作为 Bubble Tea 子组件
type FilePanel struct {
	side       PanelSide
	fs         FileSystem
	cwd        string
	entries    []FileEntry // 当前显示的条目（可能经过过滤）
	allEntries []FileEntry // 全部条目（未过滤）
	cursor     int
	offset     int // 虚拟滚动偏移
	width      int
	height     int
	loading    bool
	err        error
	filter     string // 当前搜索过滤词
}

// NewFilePanel 创建文件面板
func NewFilePanel(side PanelSide, fs FileSystem, initialDir string) FilePanel {
	return FilePanel{
		side: side,
		fs:   fs,
		cwd:  initialDir,
	}
}

// LoadDir 异步加载当前目录（返回 tea.Cmd）
func (p FilePanel) LoadDir() tea.Cmd {
	side := p.side
	cwd := p.cwd
	fs := p.fs
	return func() tea.Msg {
		entries, err := fs.ReadDir(cwd)
		if err != nil {
			return DirLoadErrMsg{Panel: side, Err: err}
		}
		return DirLoadedMsg{Panel: side, Entries: entries, Path: cwd}
	}
}

// Update 处理面板消息
func (p FilePanel) Update(msg tea.Msg) (FilePanel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		return p.handleKey(msg)
	case DirLoadedMsg:
		if msg.Panel == p.side {
			p.cwd = msg.Path
			p.loading = false
			p.err = nil
			p.setEntries(msg.Entries)
		}
		return p, nil
	case DirLoadErrMsg:
		if msg.Panel == p.side {
			p.loading = false
			p.err = msg.Err
		}
		return p, nil
	}
	return p, nil
}

// handleKey 处理键盘输入
func (p FilePanel) handleKey(msg tea.KeyMsg) (FilePanel, tea.Cmd) {
	keys := DefaultKeyMap()
	switch {
	case key.Matches(msg, keys.Up):
		p.CursorUp()
	case key.Matches(msg, keys.Down):
		p.CursorDown()
	case key.Matches(msg, keys.HalfPageUp):
		p.PageUp()
	case key.Matches(msg, keys.HalfPageDown):
		p.PageDown()
	case key.Matches(msg, keys.PageUp):
		p.FullPageUp()
	case key.Matches(msg, keys.PageDown):
		p.FullPageDown()
	case key.Matches(msg, keys.GoToTop):
		p.GoTop()
	case key.Matches(msg, keys.GoToBottom):
		p.GoBottom()
	case key.Matches(msg, keys.Enter), key.Matches(msg, keys.OpenFold):
		return p.EnterDir()
	case key.Matches(msg, keys.Backspace), key.Matches(msg, keys.CloseFold):
		return p.GoParent()
	case key.Matches(msg, keys.Select):
		p.ToggleSelect()
	}
	return p, nil
}

// setEntries 设置文件列表（排序：目录优先，然后按名称）
func (p *FilePanel) setEntries(infos []FileInfo) {
	// 排序：目录优先，再按名称
	sort.Slice(infos, func(i, j int) bool {
		if infos[i].IsDir != infos[j].IsDir {
			return infos[i].IsDir
		}
		return strings.ToLower(infos[i].Name) < strings.ToLower(infos[j].Name)
	})

	p.allEntries = make([]FileEntry, len(infos))
	for i, info := range infos {
		p.allEntries[i] = FileEntry{Info: info}
	}

	// 应用当前过滤
	if p.filter != "" {
		p.applyFilterInternal(p.filter)
	} else {
		p.entries = make([]FileEntry, len(p.allEntries))
		copy(p.entries, p.allEntries)
	}
	p.cursor = 0
	p.offset = 0
}

// ApplyFilter 应用搜索过滤
func (p *FilePanel) ApplyFilter(query string) {
	p.filter = query
	if query == "" {
		p.entries = make([]FileEntry, len(p.allEntries))
		copy(p.entries, p.allEntries)
	} else {
		p.applyFilterInternal(query)
	}
	p.cursor = 0
	p.offset = 0
}

// ClearFilter 清除搜索过滤
func (p *FilePanel) ClearFilter() {
	p.filter = ""
	p.entries = make([]FileEntry, len(p.allEntries))
	copy(p.entries, p.allEntries)
	p.cursor = 0
	p.offset = 0
}

// applyFilterInternal 内部过滤实现（模糊匹配文件名）
func (p *FilePanel) applyFilterInternal(query string) {
	query = strings.ToLower(query)
	var filtered []FileEntry
	for _, e := range p.allEntries {
		if strings.Contains(strings.ToLower(e.Info.Name), query) {
			filtered = append(filtered, e)
		}
	}
	p.entries = filtered
}

// CursorUp 光标上移
func (p *FilePanel) CursorUp() {
	if p.cursor > 0 {
		p.cursor--
		p.ensureVisible()
	}
}

// CursorDown 光标下移
func (p *FilePanel) CursorDown() {
	if p.cursor < len(p.entries)-1 {
		p.cursor++
		p.ensureVisible()
	}
}

// PageUp 半页上滚
func (p *FilePanel) PageUp() {
	visibleHeight := p.viewHeight()
	p.cursor -= visibleHeight / 2
	if p.cursor < 0 {
		p.cursor = 0
	}
	p.ensureVisible()
}

// PageDown 半页下滚
func (p *FilePanel) PageDown() {
	visibleHeight := p.viewHeight()
	p.cursor += visibleHeight / 2
	if p.cursor >= len(p.entries) {
		p.cursor = len(p.entries) - 1
	}
	if p.cursor < 0 {
		p.cursor = 0
	}
	p.ensureVisible()
}

// FullPageUp 全页上滚
func (p *FilePanel) FullPageUp() {
	visibleHeight := p.viewHeight()
	p.cursor -= visibleHeight
	if p.cursor < 0 {
		p.cursor = 0
	}
	p.ensureVisible()
}

// FullPageDown 全页下滚
func (p *FilePanel) FullPageDown() {
	visibleHeight := p.viewHeight()
	p.cursor += visibleHeight
	if p.cursor >= len(p.entries) {
		p.cursor = len(p.entries) - 1
	}
	if p.cursor < 0 {
		p.cursor = 0
	}
	p.ensureVisible()
}

// GoTop 跳到顶部
func (p *FilePanel) GoTop() {
	p.cursor = 0
	p.offset = 0
}

// GoBottom 跳到底部
func (p *FilePanel) GoBottom() {
	if len(p.entries) > 0 {
		p.cursor = len(p.entries) - 1
		p.ensureVisible()
	}
}

// EnterDir 进入目录
func (p FilePanel) EnterDir() (FilePanel, tea.Cmd) {
	if p.cursor < 0 || p.cursor >= len(p.entries) {
		return p, nil
	}
	entry := p.entries[p.cursor]
	if !entry.Info.IsDir {
		return p, nil
	}
	p.cwd = path.Join(p.cwd, entry.Info.Name)
	p.loading = true
	return p, p.LoadDir()
}

// GoParent 返回上级目录
func (p FilePanel) GoParent() (FilePanel, tea.Cmd) {
	parent := path.Dir(p.cwd)
	if parent == p.cwd {
		// 已经是根目录
		return p, nil
	}
	p.cwd = parent
	p.loading = true
	return p, p.LoadDir()
}

// ToggleSelect 切换当前文件的选中状态
func (p *FilePanel) ToggleSelect() {
	if p.cursor < 0 || p.cursor >= len(p.entries) {
		return
	}
	p.entries[p.cursor].Selected = !p.entries[p.cursor].Selected
	// 选中后光标自动下移
	if p.cursor < len(p.entries)-1 {
		p.cursor++
		p.ensureVisible()
	}
}

// SelectedFiles 返回所有选中的文件
func (p *FilePanel) SelectedFiles() []FileEntry {
	var selected []FileEntry
	for _, e := range p.entries {
		if e.Selected {
			selected = append(selected, e)
		}
	}
	return selected
}

// ClearSelection 清除所有选中
func (p *FilePanel) ClearSelection() {
	for i := range p.entries {
		p.entries[i].Selected = false
	}
}

// CurrentEntry 返回当前光标所在的条目
func (p *FilePanel) CurrentEntry() *FileEntry {
	if p.cursor < 0 || p.cursor >= len(p.entries) {
		return nil
	}
	return &p.entries[p.cursor]
}

// Cwd 返回当前工作目录
func (p *FilePanel) Cwd() string {
	return p.cwd
}

// SetSize 设置面板尺寸
func (p *FilePanel) SetSize(w, h int) {
	p.width = w
	p.height = h
}

// View 渲染文件面板
func (p FilePanel) View() string {
	contentWidth := p.width - 2 // 减去边框
	viewH := p.viewHeight()

	// 面板标题：路径
	var titleStyle lipgloss.Style
	var title string
	if p.side == PanelLeft {
		title = "Local: " + p.shortenPath(p.cwd, contentWidth-10)
	} else {
		title = "Remote: " + p.shortenPath(p.cwd, contentWidth-11)
	}
	titleStyle = PanelTitleStyle
	titleLine := titleStyle.Width(contentWidth).Render(title)

	// 表头
	header := p.renderHeader(contentWidth)

	// 内容区
	var lines []string
	if p.loading {
		lines = append(lines, lipgloss.NewStyle().
			Foreground(lipgloss.Color(colorFgDim)).
			Render("  加载中..."))
	} else if p.err != nil {
		lines = append(lines, lipgloss.NewStyle().
			Foreground(lipgloss.Color(colorRed)).
			Render("  错误: "+p.err.Error()))
	} else if len(p.entries) == 0 {
		lines = append(lines, lipgloss.NewStyle().
			Foreground(lipgloss.Color(colorFgDim)).
			Render("  （空目录）"))
	} else {
		endIdx := p.offset + viewH
		if endIdx > len(p.entries) {
			endIdx = len(p.entries)
		}
		for i := p.offset; i < endIdx; i++ {
			lines = append(lines, p.renderEntry(i, contentWidth))
		}
	}

	// 补齐空行
	for len(lines) < viewH {
		lines = append(lines, strings.Repeat(" ", contentWidth))
	}

	// 滚动指示器
	scrollIndicator := p.renderScrollIndicator(contentWidth)

	content := lipgloss.JoinVertical(lipgloss.Left,
		titleLine,
		header,
		strings.Join(lines, "\n"),
		scrollIndicator,
	)

	return content
}

// columnLayout 根据面板宽度确定显示哪些列
// 返回 (showSize, showPerm, showTime)
func columnLayout(width int) (bool, bool, bool) {
	switch {
	case width < 30:
		// 极窄：只显示文件名
		return false, false, false
	case width < 40:
		// 窄：文件名 + 大小
		return true, false, false
	case width < 55:
		// 中等：文件名 + 大小 + 权限
		return true, true, false
	default:
		// 宽：全部列
		return true, true, true
	}
}

// renderHeader 渲染表头
func (p FilePanel) renderHeader(width int) string {
	showSize, showPerm, showTime := columnLayout(width)

	headerStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(colorFgDim)).
		Bold(true)

	// 计算名称列宽度
	nameW := width - 2 // 基础：留 2 字符给前缀
	if showSize {
		nameW -= 9 // " %8s"
	}
	if showPerm {
		nameW -= 11 // " %-10s"
	}
	if showTime {
		nameW -= 7 // " %6s"
	}
	if nameW < 8 {
		nameW = 8
	}

	var line string
	name := truncate("Name", nameW)
	line = fmt.Sprintf(" %-*s", nameW, name)
	if showSize {
		line += fmt.Sprintf(" %8s", "Size")
	}
	if showPerm {
		line += fmt.Sprintf(" %-10s", "Perm")
	}
	if showTime {
		line += fmt.Sprintf(" %s", "Modified")
	}

	return headerStyle.Width(width).Render(line)
}

// renderEntry 渲染单个文件条目
func (p FilePanel) renderEntry(idx int, width int) string {
	entry := p.entries[idx]
	isCursor := idx == p.cursor
	isSelected := entry.Selected

	showSize, showPerm, showTime := columnLayout(width)

	// 计算名称列宽度（与 renderHeader 保持一致）
	nameW := width - 2
	if showSize {
		nameW -= 9
	}
	if showPerm {
		nameW -= 11
	}
	if showTime {
		nameW -= 7
	}
	if nameW < 8 {
		nameW = 8
	}

	// 图标前缀
	var prefix string
	if entry.Info.IsDir {
		prefix = "▸ "
	} else {
		prefix = "  "
	}

	// 选中标记
	var selectMark string
	if isSelected {
		selectMark = "● "
	} else {
		selectMark = "  "
	}

	name := truncate(prefix+entry.Info.Name, nameW-2)
	line := fmt.Sprintf("%s%-*s", selectMark, nameW-2, name)

	if showSize {
		line += fmt.Sprintf(" %8s", formatSize(entry.Info.Size))
	}
	if showPerm {
		line += fmt.Sprintf(" %s", formatPerm(entry.Info.Mode))
	}
	if showTime {
		line += fmt.Sprintf(" %s", formatTime(entry.Info.ModTime))
	}

	// 应用样式
	var style lipgloss.Style
	switch {
	case isCursor:
		style = CursorStyle.Width(width)
	case isSelected:
		style = SelectedStyle.Width(width)
	case entry.Info.IsDir:
		style = DirStyle.Width(width)
	case entry.Info.LinkTarget != "":
		style = SymlinkStyle.Width(width)
	case entry.Info.Mode&0111 != 0:
		style = ExecStyle.Width(width)
	case strings.HasPrefix(entry.Info.Name, "."):
		style = HiddenStyle.Width(width)
	default:
		style = FileStyle.Width(width)
	}

	return style.Render(line)
}

// renderScrollIndicator 渲染滚动指示器
func (p FilePanel) renderScrollIndicator(width int) string {
	total := len(p.entries)
	if total == 0 {
		return lipgloss.NewStyle().
			Foreground(lipgloss.Color(colorFgDark)).
			Width(width).
			Render("")
	}

	pos := fmt.Sprintf(" %d/%d", p.cursor+1, total)
	return lipgloss.NewStyle().
		Foreground(lipgloss.Color(colorFgDark)).
		Width(width).
		Render(pos)
}

// viewHeight 计算可用于文件列表的高度
// 总高度减去：标题(1) + 表头(1) + 滚动指示器(1)
func (p FilePanel) viewHeight() int {
	h := p.height - 3
	if h < 1 {
		h = 1
	}
	return h
}

// ensureVisible 确保光标在可视区域内
func (p *FilePanel) ensureVisible() {
	viewH := p.viewHeight()
	if p.cursor < p.offset {
		p.offset = p.cursor
	}
	if p.cursor >= p.offset+viewH {
		p.offset = p.cursor - viewH + 1
	}
}

// shortenPath 缩短路径显示
func (p FilePanel) shortenPath(path string, maxLen int) string {
	if len(path) <= maxLen {
		return path
	}
	// 保留文件名和尾部
	if maxLen > 5 {
		return "..." + path[len(path)-maxLen+3:]
	}
	return path[:maxLen]
}

// ============================================================
// 格式化辅助函数
// ============================================================

// formatSize 格式化文件大小
func formatSize(size int64) string {
	if size < 0 {
		return "-"
	}
	const (
		kb = 1024
		mb = 1024 * kb
		gb = 1024 * mb
	)
	switch {
	case size >= gb:
		return fmt.Sprintf("%.1fG", float64(size)/float64(gb))
	case size >= mb:
		return fmt.Sprintf("%.1fM", float64(size)/float64(mb))
	case size >= kb:
		return fmt.Sprintf("%.1fK", float64(size)/float64(kb))
	default:
		return fmt.Sprintf("%dB", size)
	}
}

// formatPerm 格式化文件权限（rwxrwxrwx 格式）
func formatPerm(mode os.FileMode) string {
	var buf [10]byte
	// 文件类型
	switch {
	case mode.IsDir():
		buf[0] = 'd'
	case mode&os.ModeSymlink != 0:
		buf[0] = 'l'
	default:
		buf[0] = '-'
	}
	// rwx 三组
	const rwx = "rwx"
	for i := 0; i < 3; i++ {
		for j := 0; j < 3; j++ {
			if mode&(1<<uint(8-i*3-j)) != 0 {
				buf[1+i*3+j] = rwx[j]
			} else {
				buf[1+i*3+j] = '-'
			}
		}
	}
	return string(buf[:])
}

// formatTime 格式化修改时间（简短格式）
func formatTime(t time.Time) string {
	now := time.Now()
	if t.Year() == now.Year() {
		return t.Format("Jan 02")
	}
	return t.Format("Jan 06")
}

// truncate 截断字符串
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	if maxLen > 3 {
		return s[:maxLen-3] + "..."
	}
	return s[:maxLen]
}
