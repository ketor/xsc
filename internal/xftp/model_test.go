package xftp

import (
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/ketor/xsc/internal/session"
)

// TestNewModelWithNilSession 测试无 session 时进入选择器模式
func TestNewModelWithNilSession(t *testing.T) {
	m := NewModel(nil)
	if m.mode != ModeSelector {
		t.Errorf("无 session 时模式应为 ModeSelector，实际: %d", m.mode)
	}
	if m.statusMsg != "请选择会话" {
		t.Errorf("状态消息应为 '请选择会话'，实际: %s", m.statusMsg)
	}
}

// TestNewModelWithSession 测试有 session 时的初始化
func TestNewModelWithSession(t *testing.T) {
	s := &session.Session{
		Host:     "192.168.1.1",
		Port:     22,
		User:     "root",
		AuthType: session.AuthTypePassword,
		Password: "pass",
		Valid:    true,
	}

	m := NewModel(s)
	if m.mode != ModeNormal {
		t.Errorf("有 session 时模式应为 ModeNormal，实际: %d", m.mode)
	}
	if m.session != s {
		t.Error("session 应被设置")
	}
	if m.transfer == nil {
		t.Error("transfer manager 应被初始化")
	}
	if m.activePanel != PanelLeft {
		t.Errorf("默认活跃面板应为 PanelLeft，实际: %d", m.activePanel)
	}
}

// TestModeConstants 测试 Mode 常量
func TestModeConstants(t *testing.T) {
	if ModeNormal != 0 {
		t.Errorf("ModeNormal 应为 0，实际: %d", ModeNormal)
	}
	if ModeSelector != 7 {
		t.Errorf("ModeSelector 应为 7，实际: %d", ModeSelector)
	}
}

// TestModelSwitchPanel 测试面板切换
func TestModelSwitchPanel(t *testing.T) {
	m := NewModel(&session.Session{
		Host:     "192.168.1.1",
		Port:     22,
		User:     "root",
		AuthType: session.AuthTypePassword,
		Password: "pass",
		Valid:    true,
	})

	if m.activePanel != PanelLeft {
		t.Error("初始应为左面板")
	}

	result, _ := m.switchPanel()
	m2 := result.(Model)
	if m2.activePanel != PanelRight {
		t.Error("切换后应为右面板")
	}

	result, _ = m2.switchPanel()
	m3 := result.(Model)
	if m3.activePanel != PanelLeft {
		t.Error("再次切换后应为左面板")
	}
}

// TestModelActiveFilterPanel 测试获取激活面板
func TestModelActiveFilterPanel(t *testing.T) {
	m := NewModel(&session.Session{
		Host:     "192.168.1.1",
		Port:     22,
		User:     "root",
		AuthType: session.AuthTypePassword,
		Password: "pass",
		Valid:    true,
	})

	m.activePanel = PanelLeft
	panel := m.activeFilterPanel()
	if panel == nil {
		t.Fatal("activeFilterPanel 不应返回 nil")
	}

	m.activePanel = PanelRight
	panel = m.activeFilterPanel()
	if panel == nil {
		t.Fatal("右面板 activeFilterPanel 不应返回 nil")
	}
}

// TestModelViewLoadingState 测试 View 在未设置尺寸时的行为
func TestModelViewLoadingState(t *testing.T) {
	m := NewModel(nil)
	m.width = 0
	m.height = 0
	view := m.View()
	if view != "Loading..." {
		t.Errorf("尺寸为 0 时应显示 'Loading...'，实际: %s", view)
	}
}

// TestModelUpdatePanelSizesZero 测试尺寸为 0 时不 panic
func TestModelUpdatePanelSizesZero(t *testing.T) {
	m := NewModel(&session.Session{
		Host:     "192.168.1.1",
		Port:     22,
		User:     "root",
		AuthType: session.AuthTypePassword,
		Password: "pass",
		Valid:    true,
	})
	m.width = 0
	m.height = 0
	// 不应 panic
	m.updatePanelSizes()
}

// TestModelUpdatePanelSizes 测试正常尺寸更新
func TestModelUpdatePanelSizes(t *testing.T) {
	m := NewModel(&session.Session{
		Host:     "192.168.1.1",
		Port:     22,
		User:     "root",
		AuthType: session.AuthTypePassword,
		Password: "pass",
		Valid:    true,
	})
	m.width = 100
	m.height = 40
	m.updatePanelSizes()

	// 每个面板应约为总宽度的一半
	if m.localPanel.width != 50 {
		t.Errorf("本地面板宽度应为 50，实际: %d", m.localPanel.width)
	}
	if m.remotePanel.width != 50 {
		t.Errorf("远程面板宽度应为 50，实际: %d", m.remotePanel.width)
	}
}

// TestColumnLayout 测试列布局函数
func TestColumnLayout(t *testing.T) {
	tests := []struct {
		width    int
		showSize bool
		showPerm bool
		showTime bool
	}{
		{20, false, false, false},
		{35, true, false, false},
		{45, true, true, false},
		{60, true, true, true},
	}

	for _, tt := range tests {
		size, perm, time := columnLayout(tt.width)
		if size != tt.showSize || perm != tt.showPerm || time != tt.showTime {
			t.Errorf("columnLayout(%d) = (%v, %v, %v), want (%v, %v, %v)",
				tt.width, size, perm, time, tt.showSize, tt.showPerm, tt.showTime)
		}
	}
}

// TestNewTransferManager 测试创建传输管理器
func TestNewTransferManager(t *testing.T) {
	tm := NewTransferManager()
	if tm == nil {
		t.Fatal("NewTransferManager 不应返回 nil")
	}
	if tm.progressCh == nil {
		t.Error("progressCh 不应为 nil")
	}
}

// TestLocalFSImplementsFileSystem 测试 LocalFS 实现 FileSystem 接口
func TestLocalFSImplementsFileSystem(t *testing.T) {
	fs, err := NewLocalFS()
	if err != nil {
		t.Fatalf("NewLocalFS 失败: %v", err)
	}

	cwd, err := fs.Getwd()
	if err != nil {
		t.Fatalf("Getwd 失败: %v", err)
	}
	if cwd == "" {
		t.Error("cwd 不应为空")
	}
}

// TestLocalFSReadDir 测试本地文件系统读取目录
func TestLocalFSReadDir(t *testing.T) {
	tmpDir := t.TempDir()
	fs := &LocalFS{cwd: tmpDir}

	entries, err := fs.ReadDir(tmpDir)
	if err != nil {
		t.Fatalf("ReadDir 失败: %v", err)
	}
	// 空目录
	if len(entries) != 0 {
		t.Errorf("空目录应返回 0 个条目，实际: %d", len(entries))
	}
}

// TestLocalFSMkdirAndRemove 测试本地创建和删除目录
func TestLocalFSMkdirAndRemove(t *testing.T) {
	tmpDir := t.TempDir()
	fs := &LocalFS{cwd: tmpDir}

	dirPath := tmpDir + "/testdir"
	if err := fs.Mkdir(dirPath); err != nil {
		t.Fatalf("Mkdir 失败: %v", err)
	}

	if err := fs.Remove(dirPath); err != nil {
		t.Fatalf("Remove 失败: %v", err)
	}
}

// TestLocalFSStat 测试本地 Stat
func TestLocalFSStat(t *testing.T) {
	tmpDir := t.TempDir()
	fs := &LocalFS{cwd: tmpDir}

	info, err := fs.Stat(tmpDir)
	if err != nil {
		t.Fatalf("Stat 失败: %v", err)
	}
	if !info.IsDir {
		t.Error("临时目录应为目录")
	}
}

// TestLocalFSStatFile 测试本地 Stat 文件
func TestLocalFSStatFile(t *testing.T) {
	tmpDir := t.TempDir()
	fs := &LocalFS{cwd: tmpDir}

	filePath := tmpDir + "/test.txt"
	if err := os.WriteFile(filePath, []byte("hello"), 0644); err != nil {
		t.Fatalf("创建文件失败: %v", err)
	}

	info, err := fs.Stat(filePath)
	if err != nil {
		t.Fatalf("Stat 失败: %v", err)
	}
	if info.IsDir {
		t.Error("文件不应为目录")
	}
	if info.Size != 5 {
		t.Errorf("文件大小应为 5，实际 %d", info.Size)
	}
}

// TestLocalFSStatNonexistent 测试 Stat 不存在的路径
func TestLocalFSStatNonexistent(t *testing.T) {
	fs := &LocalFS{cwd: "/tmp"}
	_, err := fs.Stat("/nonexistent/path")
	if err == nil {
		t.Error("Stat 不存在的路径应返回错误")
	}
}

// TestLocalFSRename 测试本地重命名
func TestLocalFSRename(t *testing.T) {
	tmpDir := t.TempDir()
	fs := &LocalFS{cwd: tmpDir}

	oldPath := tmpDir + "/old.txt"
	newPath := tmpDir + "/new.txt"
	if err := os.WriteFile(oldPath, []byte("content"), 0644); err != nil {
		t.Fatalf("创建文件失败: %v", err)
	}

	if err := fs.Rename(oldPath, newPath); err != nil {
		t.Fatalf("Rename 失败: %v", err)
	}

	if _, err := os.Stat(newPath); os.IsNotExist(err) {
		t.Error("重命名后新文件应存在")
	}
}

// TestLocalFSChmod 测试本地修改权限
func TestLocalFSChmod(t *testing.T) {
	tmpDir := t.TempDir()
	fs := &LocalFS{cwd: tmpDir}

	filePath := tmpDir + "/chmod_test.txt"
	if err := os.WriteFile(filePath, []byte("data"), 0644); err != nil {
		t.Fatalf("创建文件失败: %v", err)
	}

	if err := fs.Chmod(filePath, 0755); err != nil {
		t.Fatalf("Chmod 失败: %v", err)
	}

	info, _ := os.Stat(filePath)
	if info.Mode().Perm() != 0755 {
		t.Errorf("权限应为 0755，实际: %o", info.Mode().Perm())
	}
}

// TestLocalFSReadDirWithFiles 测试读取含文件的目录
func TestLocalFSReadDirWithFiles(t *testing.T) {
	tmpDir := t.TempDir()
	fs := &LocalFS{cwd: tmpDir}

	// 创建文件和子目录
	os.WriteFile(tmpDir+"/file1.txt", []byte("a"), 0644)
	os.Mkdir(tmpDir+"/subdir", 0755)

	entries, err := fs.ReadDir(tmpDir)
	if err != nil {
		t.Fatalf("ReadDir 失败: %v", err)
	}
	if len(entries) != 2 {
		t.Errorf("期望 2 个条目，实际 %d", len(entries))
	}

	// 验证文件信息
	for _, e := range entries {
		if e.Name == "file1.txt" {
			if e.IsDir {
				t.Error("file1.txt 不应为目录")
			}
			if e.Size != 1 {
				t.Errorf("file1.txt 大小应为 1，实际 %d", e.Size)
			}
		}
		if e.Name == "subdir" {
			if !e.IsDir {
				t.Error("subdir 应为目录")
			}
		}
	}
}

// TestLocalFSReadDirNonexistent 测试读取不存在的目录
func TestLocalFSReadDirNonexistent(t *testing.T) {
	fs := &LocalFS{cwd: "/tmp"}
	_, err := fs.ReadDir("/nonexistent/dir")
	if err == nil {
		t.Error("读取不存在的目录应返回错误")
	}
}

// TestFormatTime 测试时间格式化
func TestFormatTime(t *testing.T) {
	now := time.Now()

	// 今年的日期
	thisYear := formatTime(now)
	if thisYear == "" {
		t.Error("formatTime 不应返回空字符串")
	}

	// 去年的日期
	lastYear := now.AddDate(-2, 0, 0)
	formatted := formatTime(lastYear)
	if formatted == "" {
		t.Error("formatTime 不应返回空字符串")
	}
}

// TestFilePanelCwd 测试 Cwd 方法
func TestFilePanelCwd(t *testing.T) {
	p := NewFilePanel(PanelLeft, nil, "/home/user")
	if p.Cwd() != "/home/user" {
		t.Errorf("Cwd() = %s, want /home/user", p.Cwd())
	}
}

// TestFilePanelSetEntries 测试 setEntries 排序
func TestFilePanelSetEntries(t *testing.T) {
	p := NewFilePanel(PanelLeft, nil, "/")
	p.height = 30

	infos := []FileInfo{
		{Name: "zebra.txt", IsDir: false},
		{Name: "alpha", IsDir: true},
		{Name: "beta.txt", IsDir: false},
		{Name: "gamma", IsDir: true},
	}

	p.setEntries(infos)

	// 目录应排在前面
	if !p.entries[0].Info.IsDir {
		t.Error("第一个条目应为目录")
	}
	if !p.entries[1].Info.IsDir {
		t.Error("第二个条目应为目录")
	}
	if p.entries[2].Info.IsDir {
		t.Error("第三个条目应为文件")
	}

	// 目录按名称排序
	if p.entries[0].Info.Name != "alpha" {
		t.Errorf("第一个目录应为 'alpha'，实际 %s", p.entries[0].Info.Name)
	}
}

// TestFilePanelSetEntriesWithFilter 测试 setEntries 时应用已有过滤
func TestFilePanelSetEntriesWithFilter(t *testing.T) {
	p := NewFilePanel(PanelLeft, nil, "/")
	p.height = 30
	p.filter = "alpha"

	infos := []FileInfo{
		{Name: "alpha.txt", IsDir: false},
		{Name: "beta.txt", IsDir: false},
	}

	p.setEntries(infos)

	// 过滤后应只有一个条目
	if len(p.entries) != 1 {
		t.Errorf("过滤后应有 1 个条目，实际 %d", len(p.entries))
	}
	if p.entries[0].Info.Name != "alpha.txt" {
		t.Errorf("过滤后应为 'alpha.txt'，实际 %s", p.entries[0].Info.Name)
	}
}

// TestFilePanelToggleSelectBoundary 测试多选边界情况
func TestFilePanelToggleSelectBoundary(t *testing.T) {
	p := NewFilePanel(PanelLeft, nil, "/")
	// 空面板不应 panic
	p.ToggleSelect()

	// 光标越界不应 panic
	p.cursor = -1
	p.ToggleSelect()
}

// TestPanelSideConstants 测试面板方向常量
func TestPanelSideConstants(t *testing.T) {
	if PanelLeft != 0 {
		t.Errorf("PanelLeft 应为 0，实际 %d", PanelLeft)
	}
	if PanelRight != 1 {
		t.Errorf("PanelRight 应为 1，实际 %d", PanelRight)
	}
}

// TestFilePanelSetSize 测试设置面板尺寸
func TestFilePanelSetSize(t *testing.T) {
	p := NewFilePanel(PanelLeft, nil, "/")
	p.SetSize(80, 40)
	if p.width != 80 {
		t.Errorf("width = %d, want 80", p.width)
	}
	if p.height != 40 {
		t.Errorf("height = %d, want 40", p.height)
	}
}

// TestFormatPermSymlink 测试符号链接权限格式化
func TestFormatPermSymlink(t *testing.T) {
	result := formatPerm(os.ModeSymlink | os.FileMode(0777))
	if result[0] != 'l' {
		t.Errorf("符号链接第一个字符应为 'l'，实际: %c", result[0])
	}
}

// TestFormatPermReadOnly 测试只读权限
func TestFormatPermReadOnly(t *testing.T) {
	result := formatPerm(os.FileMode(0444))
	if result != "-r--r--r--" {
		t.Errorf("formatPerm(0444) = %s, want -r--r--r--", result)
	}
}

// TestFilePanelEnterDirNotDir 测试进入非目录时不操作
func TestFilePanelEnterDirNotDir(t *testing.T) {
	p := NewFilePanel(PanelLeft, nil, "/")
	p.entries = []FileEntry{
		{Info: FileInfo{Name: "file.txt", IsDir: false}},
	}
	p.cursor = 0

	p2, cmd := p.EnterDir()
	if cmd != nil {
		t.Error("进入非目录时不应有 cmd")
	}
	if p2.cwd != "/" {
		t.Error("进入非目录时 cwd 不应改变")
	}
}

// TestFilePanelEnterDirOutOfBounds 测试光标越界时进入目录
func TestFilePanelEnterDirOutOfBounds(t *testing.T) {
	p := NewFilePanel(PanelLeft, nil, "/")
	p.cursor = 5 // 无条目
	p2, cmd := p.EnterDir()
	if cmd != nil {
		t.Error("光标越界时不应有 cmd")
	}
	_ = p2
}

// TestFilePanelGoParentAtRoot 测试在根目录时返回上级
func TestFilePanelGoParentAtRoot(t *testing.T) {
	p := NewFilePanel(PanelLeft, nil, "/")
	p2, cmd := p.GoParent()
	if cmd != nil {
		t.Error("在根目录时不应有 cmd")
	}
	if p2.cwd != "/" {
		t.Error("在根目录时 cwd 不应改变")
	}
}

// TestFilePanelRenderScrollIndicator 测试滚动指示器渲染
func TestFilePanelRenderScrollIndicator(t *testing.T) {
	p := NewFilePanel(PanelLeft, nil, "/")
	p.width = 40

	// 空列表
	result := p.renderScrollIndicator(40)
	// 不应 panic
	_ = result

	// 有条目时
	p.entries = []FileEntry{
		{Info: FileInfo{Name: "file1"}},
	}
	p.cursor = 0
	result = p.renderScrollIndicator(40)
	if result == "" {
		t.Error("有条目时滚动指示器不应为空")
	}
}

// TestFilePanelRenderHeader 测试表头渲染
func TestFilePanelRenderHeader(t *testing.T) {
	p := NewFilePanel(PanelLeft, nil, "/")

	// 不同宽度
	for _, w := range []int{20, 35, 45, 60} {
		result := p.renderHeader(w)
		if result == "" {
			t.Errorf("宽度 %d 时表头不应为空", w)
		}
	}
}

// TestFilePanelRenderEntry 测试条目渲染
func TestFilePanelRenderEntry(t *testing.T) {
	p := NewFilePanel(PanelLeft, nil, "/")
	p.width = 80
	p.height = 30
	p.entries = []FileEntry{
		{Info: FileInfo{Name: "dir1", IsDir: true, Mode: os.ModeDir | 0755}},
		{Info: FileInfo{Name: "file.txt", IsDir: false, Size: 1024, Mode: 0644}},
		{Info: FileInfo{Name: ".hidden", IsDir: false, Mode: 0644}},
		{Info: FileInfo{Name: "exec", IsDir: false, Mode: 0755}},
		{Info: FileInfo{Name: "link", IsDir: false, LinkTarget: "/target", Mode: os.ModeSymlink}},
	}

	for i := range p.entries {
		// 不应 panic
		result := p.renderEntry(i, 60)
		if result == "" {
			t.Errorf("条目 %d 渲染不应为空", i)
		}
	}

	// 选中状态
	p.entries[1].Selected = true
	result := p.renderEntry(1, 60)
	if result == "" {
		t.Error("选中条目渲染不应为空")
	}
}

// TestFilePanelView 测试 View 渲染
func TestFilePanelView(t *testing.T) {
	p := NewFilePanel(PanelLeft, nil, "/tmp")
	p.width = 60
	p.height = 20

	// 空目录
	view := p.View()
	if view == "" {
		t.Error("View 不应返回空字符串")
	}

	// Loading 状态
	p.loading = true
	view = p.View()
	if view == "" {
		t.Error("Loading 状态 View 不应返回空字符串")
	}

	// 错误状态
	p.loading = false
	p.err = fmt.Errorf("test error")
	view = p.View()
	if view == "" {
		t.Error("错误状态 View 不应返回空字符串")
	}

	// 有条目
	p.err = nil
	p.entries = []FileEntry{
		{Info: FileInfo{Name: "file1.txt", IsDir: false, Size: 100, Mode: 0644}},
	}
	view = p.View()
	if view == "" {
		t.Error("有条目时 View 不应返回空字符串")
	}
}

// TestShortenPathEdgeCases 测试路径缩短边界情况
func TestShortenPathEdgeCases(t *testing.T) {
	p := NewFilePanel(PanelLeft, nil, "/")

	// 极短 maxLen
	result := p.shortenPath("/very/long/path", 3)
	if len(result) > 3 {
		t.Errorf("缩短后长度应 <= 3，实际 %d", len(result))
	}
}

// TestDefaultKeyMap 测试默认快捷键
func TestDefaultKeyMap(t *testing.T) {
	km := DefaultKeyMap()
	// 验证一些核心键绑定不为空
	if len(km.Up.Keys()) == 0 {
		t.Error("Up 键绑定不应为空")
	}
	if len(km.Down.Keys()) == 0 {
		t.Error("Down 键绑定不应为空")
	}
	if len(km.Quit.Keys()) == 0 {
		t.Error("Quit 键绑定不应为空")
	}
	if len(km.SwitchPanel.Keys()) == 0 {
		t.Error("SwitchPanel 键绑定不应为空")
	}
}

// TestModelExecuteCommandQuit 测试退出命令
func TestModelExecuteCommandQuit(t *testing.T) {
	m := NewModel(&session.Session{
		Host: "192.168.1.1", Port: 22, User: "root",
		AuthType: session.AuthTypePassword, Password: "pass", Valid: true,
	})
	m.connected = true

	result, _ := m.executeCommand("q")
	m2 := result.(Model)
	if m2.mode != ModeSelector {
		t.Errorf("执行 :q 后应进入选择器模式，实际: %d", m2.mode)
	}
	if m2.connected {
		t.Error("执行 :q 后应断开连接")
	}

	// 测试 :quit
	m3 := NewModel(&session.Session{
		Host: "192.168.1.1", Port: 22, User: "root",
		AuthType: session.AuthTypePassword, Password: "pass", Valid: true,
	})
	result2, _ := m3.executeCommand("quit")
	m4 := result2.(Model)
	if m4.mode != ModeSelector {
		t.Errorf("执行 :quit 后应进入选择器模式，实际: %d", m4.mode)
	}
}

// TestModelExecuteCommandUnknown 测试未知命令
func TestModelExecuteCommandUnknown(t *testing.T) {
	m := NewModel(&session.Session{
		Host: "192.168.1.1", Port: 22, User: "root",
		AuthType: session.AuthTypePassword, Password: "pass", Valid: true,
	})

	result, cmd := m.executeCommand("foobar")
	m2 := result.(Model)
	if cmd != nil {
		t.Error("未知命令不应有 cmd")
	}
	if m2.statusMsg != "未知命令: foobar" {
		t.Errorf("状态消息应包含命令名，实际: %s", m2.statusMsg)
	}
}

// TestModelExecuteCommandReconnectNoSession 测试无会话时重连
func TestModelExecuteCommandReconnectNoSession(t *testing.T) {
	m := NewModel(&session.Session{
		Host: "192.168.1.1", Port: 22, User: "root",
		AuthType: session.AuthTypePassword, Password: "pass", Valid: true,
	})
	m.session = nil

	result, cmd := m.executeCommand("reconnect")
	m2 := result.(Model)
	if cmd != nil {
		t.Error("无会话时不应有 cmd")
	}
	if m2.statusMsg != "无活跃会话" {
		t.Errorf("状态消息应为 '无活跃会话'，实际: %s", m2.statusMsg)
	}
}

// TestModelRenderHelp 测试帮助页面渲染
func TestModelRenderHelp(t *testing.T) {
	m := NewModel(&session.Session{
		Host: "192.168.1.1", Port: 22, User: "root",
		AuthType: session.AuthTypePassword, Password: "pass", Valid: true,
	})
	m.width = 80
	m.height = 40

	help := m.renderHelp()
	if help == "" {
		t.Error("帮助页面不应为空")
	}
}

// TestModelRenderError 测试错误弹窗渲染
func TestModelRenderError(t *testing.T) {
	m := NewModel(&session.Session{
		Host: "192.168.1.1", Port: 22, User: "root",
		AuthType: session.AuthTypePassword, Password: "pass", Valid: true,
	})
	m.width = 80
	m.height = 40

	// 无错误信息
	m.err = nil
	result := m.renderError()
	if result == "" {
		t.Error("renderError 不应返回空")
	}

	// 有错误信息
	m.err = fmt.Errorf("connection failed")
	result = m.renderError()
	if result == "" {
		t.Error("有错误时 renderError 不应为空")
	}

	// 未连接时显示不同提示
	m.connected = false
	result = m.renderError()
	if result == "" {
		t.Error("未连接时 renderError 不应为空")
	}
}

// TestModelRenderTransferResult 测试传输结果渲染
func TestModelRenderTransferResult(t *testing.T) {
	m := NewModel(&session.Session{
		Host: "192.168.1.1", Port: 22, User: "root",
		AuthType: session.AuthTypePassword, Password: "pass", Valid: true,
	})
	m.width = 80
	m.height = 40

	// nil 结果
	m.transferResult = nil
	result := m.renderTransferResult()
	if result != "" {
		t.Error("nil transferResult 应返回空字符串")
	}

	// 有结果
	m.transferResult = &TransferResultMsg{
		Files:      3,
		Dirs:       1,
		TotalBytes: 1024,
		Failed:     0,
	}
	result = m.renderTransferResult()
	if result == "" {
		t.Error("有结果时 renderTransferResult 不应为空")
	}
}

// TestModelRenderStatusBar 测试状态栏渲染
func TestModelRenderStatusBar(t *testing.T) {
	m := NewModel(&session.Session{
		Host: "192.168.1.1", Port: 22, User: "root",
		AuthType: session.AuthTypePassword, Password: "pass", Valid: true,
	})
	m.width = 100
	m.height = 40
	m.statusMsg = "已连接"

	result := m.renderStatusBar()
	if result == "" {
		t.Error("状态栏不应为空")
	}

	// 有选中文件
	m.localPanel.entries = []FileEntry{
		{Info: FileInfo{Name: "a.txt"}, Selected: true},
		{Info: FileInfo{Name: "b.txt"}, Selected: false},
	}
	result = m.renderStatusBar()
	if result == "" {
		t.Error("有选中文件时状态栏不应为空")
	}
}

// TestModelRenderPanel 测试面板渲染
func TestModelRenderPanel(t *testing.T) {
	m := NewModel(&session.Session{
		Host: "192.168.1.1", Port: 22, User: "root",
		AuthType: session.AuthTypePassword, Password: "pass", Valid: true,
	})
	m.width = 100
	m.height = 40
	m.updatePanelSizes()

	// 渲染左面板
	left := m.renderPanel(PanelLeft)
	if left == "" {
		t.Error("左面板渲染不应为空")
	}

	// 渲染右面板
	right := m.renderPanel(PanelRight)
	if right == "" {
		t.Error("右面板渲染不应为空")
	}
}

// TestModelRenderSearchBar 测试搜索栏渲染
func TestModelRenderSearchBar(t *testing.T) {
	m := NewModel(&session.Session{
		Host: "192.168.1.1", Port: 22, User: "root",
		AuthType: session.AuthTypePassword, Password: "pass", Valid: true,
	})
	m.width = 80

	result := m.renderSearchBar()
	if result == "" {
		t.Error("搜索栏不应为空")
	}
}

// TestModelRenderConfirmBar 测试确认对话框渲染
func TestModelRenderConfirmBar(t *testing.T) {
	m := NewModel(&session.Session{
		Host: "192.168.1.1", Port: 22, User: "root",
		AuthType: session.AuthTypePassword, Password: "pass", Valid: true,
	})
	m.width = 80

	// 单个文件
	m.confirmFiles = []confirmEntry{{Name: "test.txt"}}
	result := m.renderConfirmBar()
	if result == "" {
		t.Error("单文件确认栏不应为空")
	}

	// 多个文件
	m.confirmFiles = []confirmEntry{{Name: "a.txt"}, {Name: "b.txt"}}
	result = m.renderConfirmBar()
	if result == "" {
		t.Error("多文件确认栏不应为空")
	}
}

// TestModelRenderOverwriteConfirmBar 测试覆盖确认渲染
func TestModelRenderOverwriteConfirmBar(t *testing.T) {
	m := NewModel(&session.Session{
		Host: "192.168.1.1", Port: 22, User: "root",
		AuthType: session.AuthTypePassword, Password: "pass", Valid: true,
	})
	m.width = 80
	m.overwriteConflicts = []string{"file1.txt", "file2.txt"}

	result := m.renderOverwriteConfirmBar()
	if result == "" {
		t.Error("覆盖确认栏不应为空")
	}
}

// TestModelRenderInputBar 测试输入对话框渲染
func TestModelRenderInputBar(t *testing.T) {
	m := NewModel(&session.Session{
		Host: "192.168.1.1", Port: 22, User: "root",
		AuthType: session.AuthTypePassword, Password: "pass", Valid: true,
	})
	m.width = 80

	result := m.renderInputBar()
	if result == "" {
		t.Error("输入栏不应为空")
	}
}

// TestModelRenderCmdBar 测试命令栏渲染
func TestModelRenderCmdBar(t *testing.T) {
	m := NewModel(&session.Session{
		Host: "192.168.1.1", Port: 22, User: "root",
		AuthType: session.AuthTypePassword, Password: "pass", Valid: true,
	})
	m.width = 80

	result := m.renderCmdBar()
	if result == "" {
		t.Error("命令栏不应为空")
	}
}

// TestModelViewModes 测试不同模式下的 View
func TestModelViewModes(t *testing.T) {
	s := &session.Session{
		Host: "192.168.1.1", Port: 22, User: "root",
		AuthType: session.AuthTypePassword, Password: "pass", Valid: true,
	}

	tests := []struct {
		name string
		mode Mode
	}{
		{"帮助模式", ModeHelp},
		{"错误模式", ModeError},
		{"命令模式", ModeCommand},
		{"搜索模式", ModeSearch},
		{"确认模式", ModeConfirm},
		{"覆盖确认模式", ModeOverwriteConfirm},
		{"输入模式", ModeInput},
		{"传输结果模式", ModeTransferResult},
		{"右键菜单模式", ModeContextMenu},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := NewModel(s)
			m.width = 100
			m.height = 40
			m.mode = tt.mode
			m.updatePanelSizes()

			// 为传输结果模式设置数据
			if tt.mode == ModeTransferResult {
				m.transferResult = &TransferResultMsg{Files: 1, TotalBytes: 100}
			}
			// 为确认模式设置数据
			if tt.mode == ModeConfirm {
				m.confirmFiles = []confirmEntry{{Name: "test.txt"}}
			}
			if tt.mode == ModeOverwriteConfirm {
				m.overwriteConflicts = []string{"conflict.txt"}
			}
			// 为右键菜单模式设置数据
			if tt.mode == ModeContextMenu {
				m.contextMenu = ContextMenu{
					visible: true,
					items: []ContextMenuItem{
						{Label: "复制", Key: "y", Action: "yank"},
						{Label: "删除", Key: "D", Action: "delete"},
					},
					cursor: 0,
				}
			}

			view := m.View()
			if view == "" {
				t.Errorf("模式 %d 的 View 不应为空", tt.mode)
			}
		})
	}
}

// TestModelViewNormalMode 测试正常模式下的 View 渲染
func TestModelViewNormalMode(t *testing.T) {
	m := NewModel(&session.Session{
		Host: "192.168.1.1", Port: 22, User: "root",
		AuthType: session.AuthTypePassword, Password: "pass", Valid: true,
	})
	m.width = 100
	m.height = 40
	m.mode = ModeNormal
	m.updatePanelSizes()

	view := m.View()
	if view == "" {
		t.Error("正常模式 View 不应为空")
	}
}

// TestModelHandleDirLoaded 测试目录加载完成处理
func TestModelHandleDirLoaded(t *testing.T) {
	m := NewModel(&session.Session{
		Host: "192.168.1.1", Port: 22, User: "root",
		AuthType: session.AuthTypePassword, Password: "pass", Valid: true,
	})
	m.width = 100
	m.height = 40

	// 左面板
	msg := DirLoadedMsg{Panel: PanelLeft, Entries: []FileInfo{{Name: "test.txt"}}}
	result, _ := m.handleDirLoaded(msg)
	_ = result

	// 右面板
	msg2 := DirLoadedMsg{Panel: PanelRight, Entries: []FileInfo{{Name: "remote.txt"}}}
	result2, _ := m.handleDirLoaded(msg2)
	_ = result2
}

// TestModelHandleDirLoadErr 测试目录加载失败处理
func TestModelHandleDirLoadErr(t *testing.T) {
	m := NewModel(&session.Session{
		Host: "192.168.1.1", Port: 22, User: "root",
		AuthType: session.AuthTypePassword, Password: "pass", Valid: true,
	})
	m.width = 100
	m.height = 40

	msg := DirLoadErrMsg{Panel: PanelLeft, Err: fmt.Errorf("permission denied")}
	result, _ := m.handleDirLoadErr(msg)
	m2 := result.(Model)
	if m2.statusMsg == "" {
		t.Error("错误后状态消息不应为空")
	}

	// 右面板错误
	msg2 := DirLoadErrMsg{Panel: PanelRight, Err: fmt.Errorf("network error")}
	result2, _ := m.handleDirLoadErr(msg2)
	_ = result2
}

// TestModelUpdatePanelSizesWithTransfer 测试有活跃传输时的面板尺寸
func TestModelUpdatePanelSizesWithTransfer(t *testing.T) {
	m := NewModel(&session.Session{
		Host: "192.168.1.1", Port: 22, User: "root",
		AuthType: session.AuthTypePassword, Password: "pass", Valid: true,
	})
	m.width = 100
	m.height = 40

	// 设置活跃传输任务
	m.transfer = NewTransferManager()
	m.transfer.active = &TransferTask{
		Source:      "/local/test.txt",
		Dest:        "/remote/test.txt",
		Size:        1024,
		Transferred: 512,
	}
	m.updatePanelSizes()

	// 面板高度应比无传输时少 1
	m2 := NewModel(&session.Session{
		Host: "192.168.1.1", Port: 22, User: "root",
		AuthType: session.AuthTypePassword, Password: "pass", Valid: true,
	})
	m2.width = 100
	m2.height = 40
	m2.updatePanelSizes()

	if m.localPanel.height >= m2.localPanel.height {
		t.Error("有活跃传输时面板高度应更小")
	}
}

// TestFormatPermDir 测试目录权限格式化
func TestFormatPermDir(t *testing.T) {
	result := formatPerm(os.ModeDir | os.FileMode(0755))
	if result[0] != 'd' {
		t.Errorf("目录第一个字符应为 'd'，实际: %c", result[0])
	}
	if result != "drwxr-xr-x" {
		t.Errorf("formatPerm = %s, want drwxr-xr-x", result)
	}
}

// TestModeAllConstants 测试所有 Mode 常量
func TestModeAllConstants(t *testing.T) {
	modes := []Mode{
		ModeNormal, ModeSearch, ModeCommand, ModeHelp,
		ModeError, ModeConfirm, ModeInput, ModeSelector,
		ModeTransferResult, ModeOverwriteConfirm, ModeContextMenu,
	}
	for i, m := range modes {
		if int(m) != i {
			t.Errorf("Mode %d 值应为 %d，实际: %d", i, i, int(m))
		}
	}
}

// TestTransferResultMsg 测试传输结果消息结构体
func TestTransferResultMsg(t *testing.T) {
	msg := TransferResultMsg{
		Files:      5,
		Dirs:       2,
		TotalBytes: 10240,
		Failed:     1,
	}
	if msg.Files != 5 {
		t.Errorf("Files = %d, want 5", msg.Files)
	}
	if msg.Failed != 1 {
		t.Errorf("Failed = %d, want 1", msg.Failed)
	}
}

// TestConnectedMsg 测试连接成功消息
func TestConnectedMsg(t *testing.T) {
	msg := ConnectedMsg{}
	if msg.RemoteFS != nil {
		t.Error("未设置时 RemoteFS 应为 nil")
	}
}

// TestDirLoadedMsg 测试目录加载完成消息
func TestDirLoadedMsg(t *testing.T) {
	msg := DirLoadedMsg{
		Panel:   PanelLeft,
		Entries: []FileInfo{{Name: "file1.txt"}},
	}
	if msg.Panel != PanelLeft {
		t.Error("Panel 应为 PanelLeft")
	}
	if len(msg.Entries) != 1 {
		t.Errorf("Entries 长度应为 1，实际 %d", len(msg.Entries))
	}
}

// TestDirLoadErrMsg 测试目录加载失败消息
func TestDirLoadErrMsg(t *testing.T) {
	msg := DirLoadErrMsg{
		Panel: PanelRight,
		Err:   fmt.Errorf("permission denied"),
	}
	if msg.Panel != PanelRight {
		t.Error("Panel 应为 PanelRight")
	}
	if msg.Err == nil {
		t.Error("Err 不应为 nil")
	}
}

func TestSelectorViewNeverExceedsTerminalHeight(t *testing.T) {
	model := NewModel(nil)
	model.width = 80
	model.height = 20
	model.selector.loading = false
	model.selector.warningMessage = strings.Repeat("XShell source directory is unavailable ", 10)
	model.menu.OpenMenu(0, topMenus)

	view := model.View()
	if got := lipgloss.Height(view); got > model.height {
		t.Fatalf("selector view height = %d, terminal height = %d", got, model.height)
	}
	if got := lipgloss.Width(view); got > model.width {
		t.Fatalf("selector view width = %d, terminal width = %d", got, model.width)
	}
}

func TestFileManagerViewNeverExceedsTerminalHeight(t *testing.T) {
	model := newMouseTestXftpModel()
	model.width = 60
	model.height = 15
	model.mode = ModeSearch
	model.statusMsg = strings.Repeat("long status ", 20)
	model.menu.OpenMenu(1, topMenus)

	view := model.View()
	if got := lipgloss.Height(view); got > model.height {
		t.Fatalf("file manager view height = %d, terminal height = %d", got, model.height)
	}
	if got := lipgloss.Width(view); got > model.width {
		t.Fatalf("file manager view width = %d, terminal width = %d", got, model.width)
	}
}

func TestSessionSelectionTransitionKeepsTopUIVisible(t *testing.T) {
	model := NewModel(nil)
	model.width = 80
	model.height = 20
	selected, _ := model.handleSessionSelected(&session.Session{
		Host: "127.0.0.1", Port: 22, User: "root", AuthType: session.AuthTypeAgent, Valid: true,
	})
	updated := selected.(Model)
	updated.localPanel.cwd = "/" + strings.Repeat("很长的本地目录/", 12)
	updated.remotePanel.cwd = "/" + strings.Repeat("很长的远程目录/", 12)
	longEntry := FileEntry{Info: FileInfo{Name: strings.Repeat("超长文件名", 20) + ".log"}}
	updated.localPanel.entries = []FileEntry{longEntry}
	updated.remotePanel.entries = []FileEntry{longEntry}
	updated.connected = true

	view := updated.View()
	if got := lipgloss.Height(view); got > updated.height {
		t.Fatalf("selected-session view height = %d, terminal height = %d", got, updated.height)
	}
	if got := lipgloss.Width(view); got > updated.width {
		t.Fatalf("selected-session view width = %d, terminal width = %d", got, updated.width)
	}
	lines := strings.Split(view, "\n")
	if len(lines) < 4 {
		t.Fatalf("selected-session view has only %d lines", len(lines))
	}
	if !strings.Contains(lines[0], "File") {
		t.Fatalf("top menu is not visible after session selection: %q", lines[0])
	}
	if !strings.Contains(lines[1], "╭") || !strings.Contains(lines[1], "╮") {
		t.Fatalf("panel top borders are not visible: %q", lines[1])
	}
	if !strings.Contains(lines[2], "Local:") || !strings.Contains(lines[2], "Remote:") {
		t.Fatalf("panel titles are not visible: %q", lines[2])
	}
	if !strings.Contains(lines[3], "Name") {
		t.Fatalf("panel headers are not visible: %q", lines[3])
	}
}

func TestTopMenuDropdownOverlaysWithoutReflow(t *testing.T) {
	model := newMouseTestXftpModel()
	model.width = 80
	model.height = 20

	closed := model.View()
	model.menu.OpenMenu(1, topMenus)
	opened := model.View()
	if lipgloss.Height(opened) != lipgloss.Height(closed) {
		t.Fatalf("menu changed height from %d to %d", lipgloss.Height(closed), lipgloss.Height(opened))
	}
	closedLines := strings.Split(closed, "\n")
	openedLines := strings.Split(opened, "\n")
	uncoveredLine := 1 + len(topMenus[model.menu.Active].Items) + 2 // 上下边框
	if !strings.Contains(openedLines[1], "┌") || !strings.Contains(openedLines[1], "┐") {
		t.Fatalf("floating menu top border missing: %q", openedLines[1])
	}
	if !strings.Contains(opened, "└") || !strings.Contains(opened, "┘") {
		t.Fatal("floating menu bottom border missing")
	}
	if closedLines[uncoveredLine] != openedLines[uncoveredLine] {
		t.Fatalf("content below floating menu moved or changed")
	}
}

func TestSelectorMenuDropdownOverlaysWithoutReflow(t *testing.T) {
	model := NewModel(nil)
	model.width = 80
	model.height = 20
	model.selector.loading = false

	closed := model.View()
	model.menu.OpenMenu(0, topMenus)
	opened := model.View()
	if lipgloss.Height(opened) != lipgloss.Height(closed) {
		t.Fatalf("selector menu changed height from %d to %d", lipgloss.Height(closed), lipgloss.Height(opened))
	}
}

func TestTopMenuBarShowsVersion(t *testing.T) {
	model := newMouseTestXftpModel()
	model.width = 80
	line := model.renderTopMenuBar()
	if !strings.Contains(line, "xftp dev") {
		t.Fatalf("xftp version missing from menu bar: %q", line)
	}
	if lipgloss.Width(line) != model.width {
		t.Fatalf("menu bar width = %d, want %d", lipgloss.Width(line), model.width)
	}
}
