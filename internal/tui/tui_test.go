package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/user/xsc/internal/session"
)

// TestShowPasswordDefaultFalse 测试 showPassword 默认值为 false
func TestShowPasswordDefaultFalse(t *testing.T) {
	m := initialModel()
	if m.showPassword {
		t.Error("showPassword should default to false")
	}
}

// TestTogglePasswordCommand 测试 :pw 命令切换密码显示
func TestTogglePasswordCommand(t *testing.T) {
	m := initialModel()

	// 模拟输入 :pw
	m.lineNumMode = true
	m.lineNumInput.SetValue("pw")

	result, _ := m.handleLineNumInput(tea.KeyMsg{Type: tea.KeyEnter})
	model := result.(Model)

	if !model.showPassword {
		t.Error("showPassword should be true after :pw command")
	}
	if model.lineNumMode {
		t.Error("lineNumMode should be false after command execution")
	}

	// 再次切换
	model.lineNumMode = true
	model.lineNumInput.SetValue("pw")

	result, _ = model.handleLineNumInput(tea.KeyMsg{Type: tea.KeyEnter})
	model = result.(Model)

	if model.showPassword {
		t.Error("showPassword should be false after second :pw command")
	}
}

// TestTogglePasswordCommandAlias 测试 :password 别名
func TestTogglePasswordCommandAlias(t *testing.T) {
	m := initialModel()

	m.lineNumMode = true
	m.lineNumInput.SetValue("password")

	result, _ := m.handleLineNumInput(tea.KeyMsg{Type: tea.KeyEnter})
	model := result.(Model)

	if !model.showPassword {
		t.Error("showPassword should be true after :password command")
	}
}

// newTestModel 创建一个带有测试会话的 Model
func newTestModel(s *session.Session) Model {
	m := initialModel()
	m.width = 120
	m.height = 40
	m.tree = &session.SessionNode{
		Name:     "root",
		IsDir:    true,
		Expanded: true,
		Children: []*session.SessionNode{
			{
				Name:    "test-session",
				IsDir:   false,
				Session: s,
			},
		},
	}
	m.tree.SetParent(nil)
	m.cursor = 0
	return m
}

// TestRenderDetailMasksPassword 测试密码隐藏时显示 ********
func TestRenderDetailMasksPassword(t *testing.T) {
	s := &session.Session{
		Host:     "example.com",
		Port:     22,
		User:     "root",
		AuthType: session.AuthTypePassword,
		Password: "mysecret",
		Valid:    true,
	}
	m := newTestModel(s)

	// showPassword=false 时应该显示 ********
	detail := m.renderDetail(40, 20)
	if !strings.Contains(detail, "********") {
		t.Error("password should be masked when showPassword is false")
	}
	if strings.Contains(detail, "mysecret") {
		t.Error("actual password should not appear when showPassword is false")
	}
}

// TestRenderDetailShowsPassword 测试密码显示时显示明文
func TestRenderDetailShowsPassword(t *testing.T) {
	s := &session.Session{
		Host:     "example.com",
		Port:     22,
		User:     "root",
		AuthType: session.AuthTypePassword,
		Password: "mysecret",
		Valid:    true,
	}
	m := newTestModel(s)
	m.showPassword = true

	detail := m.renderDetail(40, 20)
	if strings.Contains(detail, "********") {
		t.Error("password should not be masked when showPassword is true")
	}
	if !strings.Contains(detail, "mysecret") {
		t.Error("actual password should appear when showPassword is true")
	}
}

// TestRenderDetailEncryptedPasswordSkipsDecrypt 测试隐藏时跳过解密
func TestRenderDetailEncryptedPasswordSkipsDecrypt(t *testing.T) {
	s := &session.Session{
		Host:              "example.com",
		Port:              22,
		User:              "root",
		AuthType:          session.AuthTypePassword,
		EncryptedPassword: "02:somefakeencrypteddata",
		Valid:             true,
	}
	m := newTestModel(s)

	// showPassword=false 时，不应调用 ResolvePassword，密码字段应该保持为空
	detail := m.renderDetail(40, 20)
	if !strings.Contains(detail, "********") {
		t.Error("encrypted password should show ******** when showPassword is false")
	}
	// 验证密码没有被解密（Password 字段仍为空）
	if s.Password != "" {
		t.Error("ResolvePassword should not have been called when showPassword is false")
	}
}

// TestStatusBarShowsPWIndicator 测试状态栏显示 [PW] 指示符
func TestStatusBarShowsPWIndicator(t *testing.T) {
	m := initialModel()
	m.width = 120
	m.height = 40

	// showPassword=false 时不应显示 [PW]
	bar := m.renderStatusBar()
	if strings.Contains(bar, "[PW]") {
		t.Error("[PW] should not appear when showPassword is false")
	}

	// showPassword=true 时应显示 [PW]
	m.showPassword = true
	bar = m.renderStatusBar()
	if !strings.Contains(bar, "[PW]") {
		t.Error("[PW] should appear when showPassword is true")
	}
}

// TestMatchCommand 测试命令匹配
func TestMatchCommand(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"q", "q"},
		{"quit", "q"},
		{"pw", "pw"},
		{"password", "pw"},
		{"noh", "noh"},
		{"nohlsearch", "noh"},
		{"unknown", ""},
		{"", ""},
	}

	for _, tt := range tests {
		got := matchCommand(tt.input)
		if got != tt.want {
			t.Errorf("matchCommand(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

// TestGetCommandCompletions 测试命令补全
func TestGetCommandCompletions(t *testing.T) {
	// 空前缀应返回所有命令
	all := getCommandCompletions("")
	if len(all) != len(commands) {
		t.Errorf("getCommandCompletions(\"\") returned %d commands, want %d", len(all), len(commands))
	}

	// "p" 应匹配 pw（name 以 p 开头）
	pMatches := getCommandCompletions("p")
	found := false
	for _, cmd := range pMatches {
		if cmd.Name == "pw" {
			found = true
		}
	}
	if !found {
		t.Error("getCommandCompletions(\"p\") should include pw")
	}

	// "q" 应匹配 q
	qMatches := getCommandCompletions("q")
	found = false
	for _, cmd := range qMatches {
		if cmd.Name == "q" {
			found = true
		}
	}
	if !found {
		t.Error("getCommandCompletions(\"q\") should include q")
	}

	// "xyz" 应无匹配
	noMatches := getCommandCompletions("xyz")
	if len(noMatches) != 0 {
		t.Errorf("getCommandCompletions(\"xyz\") returned %d commands, want 0", len(noMatches))
	}
}

// TestTabCompletion 测试 Tab 补全功能
func TestTabCompletion(t *testing.T) {
	m := initialModel()
	m.lineNumMode = true
	m.lineNumInput.SetValue("p")

	result, _ := m.handleLineNumInput(tea.KeyMsg{Type: tea.KeyTab})
	model := result.(Model)

	if model.lineNumInput.Value() != "pw" {
		t.Errorf("Tab completion: got %q, want %q", model.lineNumInput.Value(), "pw")
	}
}

// TestRenderHelp 测试帮助视图渲染
func TestRenderHelp(t *testing.T) {
	m := initialModel()
	m.width = 120
	m.height = 40

	helpView := m.renderHelp()

	// 验证包含所有章节标题
	sections := []string{"移动", "折叠", "搜索", "会话操作", "命令 (: 模式)", "其他"}
	for _, section := range sections {
		if !strings.Contains(helpView, section) {
			t.Errorf("renderHelp() should contain section %q", section)
		}
	}

	// 验证包含关键快捷键描述
	keys := []string{"↑/k", "gg", "Space/o", "Enter", ":q", ":pw", ":noh"}
	for _, k := range keys {
		if !strings.Contains(helpView, k) {
			t.Errorf("renderHelp() should contain key %q", k)
		}
	}
}

// TestRenderNodeSecureCRTStyles 测试 SecureCRT 会话的视觉区分
func TestRenderNodeSecureCRTStyles(t *testing.T) {
	m := initialModel()
	m.width = 120
	m.height = 40

	// 创建测试节点
	// SecureCRT 目录
	scDir := &session.SessionNode{
		Name:     "securecrt",
		IsDir:    true,
		Expanded: true,
		Children: make([]*session.SessionNode, 0),
	}

	// SecureCRT 会话
	scSession := &session.SessionNode{
		Name:    "prod-server",
		IsDir:   false,
		Session: &session.Session{Valid: true},
	}
	scDir.Children = append(scDir.Children, scSession)
	scSession.SetParent(scDir)

	// 本地会话
	localSession := &session.SessionNode{
		Name:    "local-server",
		IsDir:   false,
		Session: &session.Session{Valid: true},
	}

	// 设置父节点
	scDir.SetParent(nil)

	// 测试 SecureCRT 目录渲染
	scDirRendered := m.renderNode(scDir, false)
	if !strings.Contains(scDirRendered, "[CRT]") {
		t.Error("SecureCRT directory should contain [CRT] marker")
	}

	// 测试 SecureCRT 会话渲染 - 应该包含 🔒 图标
	scSessionRendered := m.renderNode(scSession, false)
	if !strings.Contains(scSessionRendered, "🔒") {
		t.Error("SecureCRT session should contain 🔒 icon")
	}

	// 测试本地会话渲染 - 不应该有 🔒 图标
	localRendered := m.renderNode(localSession, false)
	if strings.Contains(localRendered, "🔒") {
		t.Error("Local session should not contain 🔒 icon")
	}

	// 测试选中状态的 SecureCRT 会话
	scSelected := m.renderNode(scSession, true)
	if !strings.Contains(scSelected, "🔒") {
		t.Error("Selected SecureCRT session should still show 🔒 icon")
	}
}
