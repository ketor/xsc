package xftp

import (
	"errors"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/ketor/xsc/internal/session"
)

// TestNewSelector 测试创建选择器
func TestNewSelector(t *testing.T) {
	s := NewSelector()
	if !s.loading {
		t.Error("新选择器应处于 loading 状态")
	}
	if s.showPassword {
		t.Error("showPassword 默认应为 false")
	}
	if s.searching {
		t.Error("新选择器不应处于搜索状态")
	}
	if s.commanding {
		t.Error("新选择器不应处于命令状态")
	}
	if s.showHelp {
		t.Error("新选择器不应处于帮助状态")
	}
	if s.cursor != 0 {
		t.Error("初始光标位置应为 0")
	}
	if s.offset != 0 {
		t.Error("初始偏移量应为 0")
	}
}

// TestSelectorCommandRegistration 测试命令注册
func TestSelectorCommandRegistration(t *testing.T) {
	found := false
	for _, cmd := range selectorCommands {
		if cmd.Name == "pw" {
			found = true
			break
		}
	}
	if !found {
		t.Error("selectorCommands 应包含 'pw' 命令")
	}
}

// TestMatchSelectorCommand 测试命令匹配
func TestMatchSelectorCommand(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"q", "q"},
		{"quit", "q"},
		{"noh", "noh"},
		{"nohlsearch", "noh"},
		{"pw", "pw"},
		{"password", "pw"},
		{"unknown", ""},
		{"", ""},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := matchSelectorCommand(tt.input)
			if result != tt.expected {
				t.Errorf("matchSelectorCommand(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

// TestGetSelectorCommandCompletions 测试命令补全
func TestGetSelectorCommandCompletions(t *testing.T) {
	// 空前缀返回所有命令
	all := getSelectorCommandCompletions("")
	if len(all) != len(selectorCommands) {
		t.Errorf("空前缀应返回 %d 个命令，实际返回 %d", len(selectorCommands), len(all))
	}

	// "p" 前缀应匹配 "pw"
	pCmds := getSelectorCommandCompletions("p")
	found := false
	for _, cmd := range pCmds {
		if cmd.Name == "pw" {
			found = true
		}
	}
	if !found {
		t.Error("前缀 'p' 应匹配 'pw' 命令")
	}

	// "z" 前缀应无匹配
	zCmds := getSelectorCommandCompletions("z")
	if len(zCmds) != 0 {
		t.Errorf("前缀 'z' 应无匹配，实际返回 %d", len(zCmds))
	}

	// "q" 前缀应匹配 "q"（精确前缀匹配）
	qCmds := getSelectorCommandCompletions("q")
	if len(qCmds) == 0 {
		t.Error("前缀 'q' 应至少匹配一个命令")
	}

	// "no" 前缀应匹配 "noh"
	noCmds := getSelectorCommandCompletions("no")
	foundNoh := false
	for _, cmd := range noCmds {
		if cmd.Name == "noh" {
			foundNoh = true
		}
	}
	if !foundNoh {
		t.Error("前缀 'no' 应匹配 'noh' 命令")
	}
}

// TestGetSelectorCommandCompletionsAlias 测试别名补全
func TestGetSelectorCommandCompletionsAlias(t *testing.T) {
	// "pass" 前缀应通过别名匹配 "pw"
	cmds := getSelectorCommandCompletions("pass")
	found := false
	for _, cmd := range cmds {
		if cmd.Name == "pw" {
			found = true
		}
	}
	if !found {
		t.Error("前缀 'pass' 应通过别名匹配 'pw' 命令")
	}
}

// TestSelectorTogglePassword 测试 :pw 命令切换密码显示
func TestSelectorTogglePassword(t *testing.T) {
	s := NewSelector()

	if s.showPassword {
		t.Error("初始 showPassword 应为 false")
	}

	// 模拟输入 :pw
	s.commanding = true
	s.commandInput.SetValue("pw")
	msg := tea.KeyMsg{Type: tea.KeyEnter}
	s, _ = s.handleCommandKey(msg)

	if !s.showPassword {
		t.Error(":pw 后 showPassword 应为 true")
	}

	// 再次切换
	s.commanding = true
	s.commandInput.SetValue("pw")
	s, _ = s.handleCommandKey(msg)

	if s.showPassword {
		t.Error("再次 :pw 后 showPassword 应为 false")
	}
}

// TestSelectorTogglePasswordAlias 测试 :password 别名
func TestSelectorTogglePasswordAlias(t *testing.T) {
	s := NewSelector()

	s.commanding = true
	s.commandInput.SetValue("password")
	msg := tea.KeyMsg{Type: tea.KeyEnter}
	s, _ = s.handleCommandKey(msg)

	if !s.showPassword {
		t.Error(":password 别名应切换 showPassword 为 true")
	}
}

// TestSelectorCommandNoh 测试 :noh 命令清除搜索过滤
func TestSelectorCommandNoh(t *testing.T) {
	s := NewSelector()
	s.filter = "test"
	s.searchInput.SetValue("test")

	s.commanding = true
	s.commandInput.SetValue("noh")
	msg := tea.KeyMsg{Type: tea.KeyEnter}
	s, _ = s.handleCommandKey(msg)

	if s.filter != "" {
		t.Errorf(":noh 后 filter 应为空，实际: %s", s.filter)
	}
}

// TestSelectorCommandNohAlias 测试 :nohlsearch 别名
func TestSelectorCommandNohAlias(t *testing.T) {
	s := NewSelector()
	s.filter = "test"

	s.commanding = true
	s.commandInput.SetValue("nohlsearch")
	msg := tea.KeyMsg{Type: tea.KeyEnter}
	s, _ = s.handleCommandKey(msg)

	if s.filter != "" {
		t.Errorf(":nohlsearch 后 filter 应为空，实际: %s", s.filter)
	}
}

// TestSelectorCommandEsc 测试 Esc 退出命令模式
func TestSelectorCommandEsc(t *testing.T) {
	s := NewSelector()
	s.commanding = true
	s.commandInput.SetValue("pw")

	msg := tea.KeyMsg{Type: tea.KeyEsc}
	s, _ = s.handleCommandKey(msg)

	if s.commanding {
		t.Error("Esc 后应退出命令模式")
	}
}

// TestSelectorCommandLineNumber 测试命令模式输入行号跳转
func TestSelectorCommandLineNumber(t *testing.T) {
	s := NewSelector()
	s.height = 30
	s.flatNodes = make([]*session.SessionNode, 10)
	for i := range s.flatNodes {
		s.flatNodes[i] = &session.SessionNode{Name: "node"}
	}

	s.commanding = true
	s.commandInput.SetValue("5")
	msg := tea.KeyMsg{Type: tea.KeyEnter}
	s, _ = s.handleCommandKey(msg)

	if s.cursor != 4 { // 行号从 1 开始，索引从 0 开始
		t.Errorf("输入行号 5 后光标应为 4，实际: %d", s.cursor)
	}
}

// TestSelectorRenderNodeHighlight 测试选中节点的完整高亮
func TestSelectorRenderNodeHighlight(t *testing.T) {
	s := NewSelector()

	node := &session.SessionNode{
		Name:  "test-server",
		IsDir: false,
		Session: &session.Session{
			Host:  "192.168.1.1",
			Port:  22,
			User:  "root",
			Valid: true,
		},
	}

	selectedLine := s.renderNode(node, true, 80)
	unselectedLine := s.renderNode(node, false, 80)

	if selectedLine == unselectedLine {
		t.Error("选中行和非选中行应该不同")
	}

	if !strings.Contains(selectedLine, "192.168.1.1") {
		t.Error("选中行应包含主机地址")
	}
}

// TestSelectorRenderNodeDirHighlight 测试选中目录节点的高亮
func TestSelectorRenderNodeDirHighlight(t *testing.T) {
	s := NewSelector()

	parent := &session.SessionNode{
		Name:  "securecrt",
		IsDir: true,
	}
	node := &session.SessionNode{
		Name:     "01.WRT",
		IsDir:    true,
		Expanded: true,
		Parent:   parent,
	}

	selectedLine := s.renderNode(node, true, 60)
	if !strings.Contains(selectedLine, "[CRT]") {
		t.Error("选中的 SecureCRT 目录应包含 [CRT] 标记")
	}
}

// TestSelectorRenderNodeXShellDir 测试 XShell 目录渲染
func TestSelectorRenderNodeXShellDir(t *testing.T) {
	s := NewSelector()

	parent := &session.SessionNode{Name: "xshell", IsDir: true}
	node := &session.SessionNode{
		Name:     "servers",
		IsDir:    true,
		Expanded: false,
		Parent:   parent,
	}

	selectedLine := s.renderNode(node, true, 60)
	if !strings.Contains(selectedLine, "[XSH]") {
		t.Error("选中的 XShell 目录应包含 [XSH] 标记")
	}
}

// TestSelectorRenderNodeMobaXtermDir 测试 MobaXterm 目录渲染
func TestSelectorRenderNodeMobaXtermDir(t *testing.T) {
	s := NewSelector()

	parent := &session.SessionNode{Name: "mobaxterm", IsDir: true}
	node := &session.SessionNode{
		Name:     "bookmarks",
		IsDir:    true,
		Expanded: true,
		Parent:   parent,
	}

	selectedLine := s.renderNode(node, true, 60)
	if !strings.Contains(selectedLine, "[MXT]") {
		t.Error("选中的 MobaXterm 目录应包含 [MXT] 标记")
	}
}

// TestSelectorRenderNodeInvalidSession 测试无效会话节点渲染
func TestSelectorRenderNodeInvalidSession(t *testing.T) {
	s := NewSelector()

	node := &session.SessionNode{
		Name:  "broken",
		IsDir: false,
		Session: &session.Session{
			Valid: false,
		},
	}

	line := s.renderNode(node, false, 60)
	if !strings.Contains(line, "invalid") {
		t.Error("无效会话应显示 [invalid] 标记")
	}
}

// TestSelectorRenderNodeNoSession 测试无 Session 数据的节点渲染
func TestSelectorRenderNodeNoSession(t *testing.T) {
	s := NewSelector()

	node := &session.SessionNode{
		Name:    "orphan",
		IsDir:   false,
		Session: nil,
	}

	// 应不 panic
	line := s.renderNode(node, false, 60)
	if !strings.Contains(line, "orphan") {
		t.Error("渲染结果应包含节点名称")
	}
}

// TestSelectorCountSessions 测试会话计数
func TestSelectorCountSessions(t *testing.T) {
	s := NewSelector()
	tree := &session.SessionNode{
		IsDir: true,
		Children: []*session.SessionNode{
			{Name: "srv1", IsDir: false},
			{Name: "srv2", IsDir: false},
			{
				Name:  "group",
				IsDir: true,
				Children: []*session.SessionNode{
					{Name: "srv3", IsDir: false},
				},
			},
		},
	}

	count := s.countSessions(tree)
	if count != 3 {
		t.Errorf("期望 3 个会话，实际 %d", count)
	}
}

// TestSelectorCountSessionsEmpty 测试空树计数
func TestSelectorCountSessionsEmpty(t *testing.T) {
	s := NewSelector()
	tree := &session.SessionNode{IsDir: true, Children: []*session.SessionNode{}}

	count := s.countSessions(tree)
	if count != 0 {
		t.Errorf("期望 0 个会话，实际 %d", count)
	}
}

// TestSelectorViewHeight 测试可视高度计算
func TestSelectorViewHeight(t *testing.T) {
	s := NewSelector()

	s.height = 30
	h := s.viewHeight()
	if h != 26 { // 30 - 4
		t.Errorf("期望高度 26，实际 %d", h)
	}

	s.height = 2
	h = s.viewHeight()
	if h != 1 {
		t.Errorf("最小高度应为 1，实际 %d", h)
	}

	s.height = 0
	h = s.viewHeight()
	if h != 1 {
		t.Errorf("高度 0 时应返回 1，实际 %d", h)
	}
}

// TestSelectorMoveCursor 测试光标移动
func TestSelectorMoveCursor(t *testing.T) {
	s := NewSelector()
	s.height = 30
	s.flatNodes = make([]*session.SessionNode, 10)

	s.moveCursor(3)
	if s.cursor != 3 {
		t.Errorf("期望光标位置 3，实际 %d", s.cursor)
	}

	s.moveCursor(-10)
	if s.cursor != 0 {
		t.Errorf("光标不应小于 0，实际 %d", s.cursor)
	}

	s.moveCursor(100)
	if s.cursor != 9 {
		t.Errorf("光标不应超过 %d，实际 %d", 9, s.cursor)
	}
}

// TestSelectorMoveCursorEmpty 测试空列表时光标移动
func TestSelectorMoveCursorEmpty(t *testing.T) {
	s := NewSelector()
	s.height = 30
	s.flatNodes = nil

	// 不应 panic
	s.moveCursor(5)
	if s.cursor != 0 {
		t.Errorf("空列表时光标应为 0，实际 %d", s.cursor)
	}
}

// TestSelectorSearchNext 测试搜索下一个
func TestSelectorSearchNext(t *testing.T) {
	s := NewSelector()
	s.height = 30
	s.filter = "test"
	s.flatNodes = []*session.SessionNode{
		{Name: "other1", IsDir: false, Session: &session.Session{}},
		{Name: "test-1", IsDir: false, Session: &session.Session{}},
		{Name: "other2", IsDir: false, Session: &session.Session{}},
		{Name: "test-2", IsDir: false, Session: &session.Session{}},
	}
	s.cursor = 0

	s.searchNext(1)
	if s.cursor != 1 {
		t.Errorf("期望光标跳到 1，实际 %d", s.cursor)
	}

	s.searchNext(1)
	if s.cursor != 3 {
		t.Errorf("期望光标跳到 3，实际 %d", s.cursor)
	}
}

// TestSelectorSearchNextWrap 测试搜索循环
func TestSelectorSearchNextWrap(t *testing.T) {
	s := NewSelector()
	s.height = 30
	s.filter = "match"
	s.flatNodes = []*session.SessionNode{
		{Name: "match-1", IsDir: false, Session: &session.Session{}},
		{Name: "other", IsDir: false, Session: &session.Session{}},
		{Name: "match-2", IsDir: false, Session: &session.Session{}},
	}
	s.cursor = 2 // 在最后一个匹配项

	// 搜索下一个应循环到第一个匹配项
	s.searchNext(1)
	if s.cursor != 0 {
		t.Errorf("搜索应循环到索引 0，实际: %d", s.cursor)
	}
}

// TestSelectorSearchPrevious 测试反向搜索
func TestSelectorSearchPrevious(t *testing.T) {
	s := NewSelector()
	s.height = 30
	s.filter = "match"
	s.flatNodes = []*session.SessionNode{
		{Name: "match-1", IsDir: false, Session: &session.Session{}},
		{Name: "other", IsDir: false, Session: &session.Session{}},
		{Name: "match-2", IsDir: false, Session: &session.Session{}},
	}
	s.cursor = 2

	s.searchNext(-1)
	if s.cursor != 0 {
		t.Errorf("反向搜索应跳到索引 0，实际: %d", s.cursor)
	}
}

// TestSelectorSearchNextNoFilter 测试无过滤条件时搜索不移动
func TestSelectorSearchNextNoFilter(t *testing.T) {
	s := NewSelector()
	s.height = 30
	s.filter = ""
	s.flatNodes = make([]*session.SessionNode, 5)
	s.cursor = 2

	s.searchNext(1)
	if s.cursor != 2 {
		t.Errorf("无过滤条件时搜索不应移动光标，实际: %d", s.cursor)
	}
}

// TestSelectorGetIndent 测试缩进计算
func TestSelectorGetIndent(t *testing.T) {
	s := NewSelector()

	root := &session.SessionNode{Name: "root", IsDir: true}
	child := &session.SessionNode{Name: "child", IsDir: true, Parent: root}
	grandchild := &session.SessionNode{Name: "gc", IsDir: false, Parent: child}

	if indent := s.getIndent(root); indent != "" {
		t.Errorf("根节点缩进应为空，实际 %q", indent)
	}
	if indent := s.getIndent(child); indent != "  " {
		t.Errorf("子节点缩进应为 2 空格，实际 %q", indent)
	}
	if indent := s.getIndent(grandchild); indent != "    " {
		t.Errorf("孙节点缩进应为 4 空格，实际 %q", indent)
	}
}

// TestSelectorExpandAll 测试展开所有目录
func TestSelectorExpandAll(t *testing.T) {
	s := NewSelector()
	tree := &session.SessionNode{
		Name:     "root",
		IsDir:    true,
		Expanded: false,
		Children: []*session.SessionNode{
			{
				Name:     "dir1",
				IsDir:    true,
				Expanded: false,
				Children: []*session.SessionNode{
					{Name: "file1", IsDir: false},
				},
			},
		},
	}

	s.expandAll(tree)

	if !tree.Expanded {
		t.Error("根目录应展开")
	}
	if !tree.Children[0].Expanded {
		t.Error("子目录应展开")
	}
}

// TestSelectorCollapseAll 测试折叠所有目录
func TestSelectorCollapseAll(t *testing.T) {
	s := NewSelector()
	tree := &session.SessionNode{
		Name:     "root",
		IsDir:    true,
		Expanded: true,
		Children: []*session.SessionNode{
			{
				Name:     "dir1",
				IsDir:    true,
				Expanded: true,
				Children: []*session.SessionNode{
					{Name: "file1", IsDir: false},
				},
			},
		},
	}

	s.collapseAll(tree)

	if tree.Expanded {
		t.Error("根目录应折叠")
	}
	if tree.Children[0].Expanded {
		t.Error("子目录应折叠")
	}
}

// TestSelectorUpdateFlatNodes 测试展平节点更新
func TestSelectorUpdateFlatNodes(t *testing.T) {
	s := NewSelector()
	s.tree = &session.SessionNode{
		Name:     "root",
		IsDir:    true,
		Expanded: true,
		Children: []*session.SessionNode{
			{Name: "file1", IsDir: false, Session: &session.Session{Host: "10.0.0.1"}},
			{
				Name:     "dir1",
				IsDir:    true,
				Expanded: true,
				Children: []*session.SessionNode{
					{Name: "file2", IsDir: false, Session: &session.Session{Host: "10.0.0.2"}},
				},
			},
		},
	}
	s.tree.SetParent(nil)

	s.updateFlatNodes()
	if len(s.flatNodes) != 3 {
		t.Errorf("期望 3 个展平节点，实际: %d", len(s.flatNodes))
	}
}

// TestSelectorUpdateFlatNodesWithFilter 测试带过滤的展平
func TestSelectorUpdateFlatNodesWithFilter(t *testing.T) {
	s := NewSelector()
	s.tree = &session.SessionNode{
		Name:     "root",
		IsDir:    true,
		Expanded: true,
		Children: []*session.SessionNode{
			{Name: "web-server", IsDir: false, Session: &session.Session{Host: "10.0.0.1"}},
			{Name: "db-server", IsDir: false, Session: &session.Session{Host: "10.0.0.2"}},
			{Name: "web-proxy", IsDir: false, Session: &session.Session{Host: "10.0.0.3"}},
		},
	}
	s.tree.SetParent(nil)

	s.filter = "web"
	s.updateFlatNodes()
	if len(s.flatNodes) != 2 {
		t.Errorf("过滤 'web' 后应有 2 个节点，实际: %d", len(s.flatNodes))
	}
}

// TestSelectorUpdateFlatNodesNilTree 测试 tree 为 nil 时
func TestSelectorUpdateFlatNodesNilTree(t *testing.T) {
	s := NewSelector()
	s.tree = nil
	s.updateFlatNodes()
	if s.flatNodes != nil {
		t.Error("tree 为 nil 时 flatNodes 应为 nil")
	}
}

// TestSelectorMatchesFilter 测试过滤匹配
func TestSelectorMatchesFilter(t *testing.T) {
	s := NewSelector()

	// 匹配名称
	node := &session.SessionNode{
		Name:    "web-server",
		IsDir:   false,
		Session: &session.Session{Host: "10.0.0.1", User: "admin"},
	}
	if !s.matchesFilter(node, "web") {
		t.Error("名称包含 'web' 应匹配")
	}

	// 匹配 Host
	if !s.matchesFilter(node, "10.0") {
		t.Error("Host 包含 '10.0' 应匹配")
	}

	// 匹配 User
	if !s.matchesFilter(node, "admin") {
		t.Error("User 包含 'admin' 应匹配")
	}

	// 不匹配
	if s.matchesFilter(node, "xyz") {
		t.Error("'xyz' 不应匹配任何字段")
	}

	// matchesFilter 的 filterLower 参数应为小写（调用方负责转换）
	// 验证节点名称中的大小写被正确处理
	upperNode := &session.SessionNode{
		Name:    "WEB-Server",
		IsDir:   false,
		Session: &session.Session{Host: "10.0.0.1"},
	}
	if !s.matchesFilter(upperNode, "web") {
		t.Error("matchesFilter 应大小写不敏感匹配节点名称")
	}
}

// TestSelectorMatchesFilterNoSession 测试无 Session 的节点过滤
func TestSelectorMatchesFilterNoSession(t *testing.T) {
	s := NewSelector()
	node := &session.SessionNode{Name: "test", IsDir: false, Session: nil}

	if !s.matchesFilter(node, "test") {
		t.Error("名称匹配时应返回 true")
	}
	if s.matchesFilter(node, "xyz") {
		t.Error("不匹配时应返回 false")
	}
}

// TestSelectorCurrentNode 测试获取当前节点
func TestSelectorCurrentNode(t *testing.T) {
	s := NewSelector()

	// 空列表
	if s.currentNode() != nil {
		t.Error("空列表时 currentNode 应返回 nil")
	}

	node := &session.SessionNode{Name: "test"}
	s.flatNodes = []*session.SessionNode{node}
	s.cursor = 0

	if s.currentNode() != node {
		t.Error("应返回当前光标指向的节点")
	}

	// 越界
	s.cursor = 5
	if s.currentNode() != nil {
		t.Error("光标越界时应返回 nil")
	}
}

// TestSelectorHasPasswordAuth 测试密码认证检查
func TestSelectorHasPasswordAuth(t *testing.T) {
	s := NewSelector()

	sess := &session.Session{
		AuthMethods: []session.AuthMethod{
			{Type: "password"},
			{Type: "key"},
		},
	}
	if !s.hasPasswordAuth(sess) {
		t.Error("包含密码认证应返回 true")
	}

	sess2 := &session.Session{
		AuthMethods: []session.AuthMethod{
			{Type: "key"},
			{Type: "agent"},
		},
	}
	if s.hasPasswordAuth(sess2) {
		t.Error("不包含密码认证应返回 false")
	}

	sess3 := &session.Session{}
	if s.hasPasswordAuth(sess3) {
		t.Error("无 AuthMethods 应返回 false")
	}
}

// TestSelectorSetSize 测试设置尺寸
func TestSelectorSetSize(t *testing.T) {
	s := NewSelector()
	s.SetSize(100, 50)
	if s.width != 100 {
		t.Errorf("期望 width 100，实际 %d", s.width)
	}
	if s.height != 50 {
		t.Errorf("期望 height 50，实际 %d", s.height)
	}
}

// TestSelectorEnsureVisible 测试光标可见性确保
func TestSelectorEnsureVisible(t *testing.T) {
	s := NewSelector()
	s.height = 10 // viewHeight = 10 - 4 = 6
	s.flatNodes = make([]*session.SessionNode, 20)

	s.cursor = 15
	s.ensureVisible()
	// offset 应调整到使 cursor 可见
	if s.cursor < s.offset || s.cursor >= s.offset+s.viewHeight() {
		t.Errorf("cursor %d 应在可视范围 [%d, %d) 内", s.cursor, s.offset, s.offset+s.viewHeight())
	}
}

// TestSelectorSelectCurrentDir 测试选择目录时切换展开状态
func TestSelectorSelectCurrentDir(t *testing.T) {
	s := NewSelector()
	s.height = 30

	dirNode := &session.SessionNode{
		Name:     "dir",
		IsDir:    true,
		Expanded: false,
		Children: []*session.SessionNode{
			{Name: "file1", IsDir: false},
		},
	}
	s.tree = &session.SessionNode{
		Name:     "root",
		IsDir:    true,
		Expanded: true,
		Children: []*session.SessionNode{dirNode},
	}
	s.tree.SetParent(nil)
	s.updateFlatNodes()
	s.cursor = 0

	s, _ = s.selectCurrent()
	if !dirNode.Expanded {
		t.Error("选择目录应切换展开状态")
	}
}

// TestSelectorSelectCurrentNilNode 测试光标无效时的选择
func TestSelectorSelectCurrentNilNode(t *testing.T) {
	s := NewSelector()
	s.flatNodes = nil

	// 不应 panic
	s, _ = s.selectCurrent()
}

func TestSelectorPartialSourceFailureIsNonBlocking(t *testing.T) {
	selector := NewSelector()
	tree := &session.SessionNode{Name: "sessions", IsDir: true}
	updated, _ := selector.Update(sessionsLoadedMsg{
		tree: tree,
		err:  errors.New("XShell source unavailable"),
	})
	if updated.showError {
		t.Fatal("optional source failure must not open blocking error modal")
	}
	if !strings.Contains(updated.warningMessage, "XShell") {
		t.Fatalf("warning message = %q", updated.warningMessage)
	}
}
