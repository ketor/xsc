package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/ketor/xsc/internal/session"
)

// TestRender_PublicKeySessionNotInvalid 回归保护 bug 1：
// YAML 写 auth_type: publickey 时（SSH 标准术语），TUI 不应显示 [invalid]。
func TestRender_PublicKeySessionNotInvalid(t *testing.T) {
	s := &session.Session{
		Host:     "10.0.0.1",
		Port:     22,
		User:     "ops",
		AuthType: session.AuthType("publickey"),
	}
	if err := s.Validate(); err != nil {
		s.Valid = false
		s.Error = err
	} else {
		s.Valid = true
	}

	leaf := &session.SessionNode{Name: "test-publickey", IsDir: false, Session: s}
	root := &session.SessionNode{Name: "sessions", IsDir: true, Children: []*session.SessionNode{leaf}}
	leaf.Parent = root

	m := initialModel()
	line := m.renderNode(leaf, false)

	if strings.Contains(line, "[invalid]") {
		t.Fatalf("publickey session shown as [invalid]: %q (Valid=%v Err=%v)", line, s.Valid, s.Error)
	}
}

// TestRender_MultiAuthMethodsAllVisible 回归保护 bug 2/3：
// 多认证方式 session 必须在详情面板显示全部方法（不只一种）。
func TestRender_MultiAuthMethodsAllVisible(t *testing.T) {
	s := &session.Session{
		Host:     "10.0.0.2",
		Port:     22,
		User:     "ops",
		AuthType: session.AuthTypePassword,
		Valid:    true,
		Password: "decrypted-pwd",
		AuthMethods: []session.AuthMethod{
			{Type: "password", Priority: 0, Password: "decrypted-pwd"},
			{Type: "publickey", Priority: 1},
			{Type: "keyboard-interactive", Priority: 2},
			{Type: "gssapi", Priority: 3},
		},
	}
	leaf := &session.SessionNode{Name: "test-multi", IsDir: false, Session: s}
	root := &session.SessionNode{Name: "sessions", IsDir: true, Children: []*session.SessionNode{leaf}}
	leaf.Parent = root

	m := initialModel()
	m.tree = root
	m.cursor = 0

	detail := m.renderDetail(80, 30)

	// All four method type labels must be present
	wantSubstrings := []string{"Password", "Public Key", "Keyboard Interactive", "GSSAPI"}
	for _, want := range wantSubstrings {
		if !strings.Contains(detail, want) {
			t.Errorf("auth method %q missing from detail panel; got:\n%s", want, detail)
		}
	}
}

// TestRender_PasswordToggleShowsPlaintext 回归保护 bug 4：
// :pw 切换为 showPassword=true 时，明文密码必须可见。
func TestRender_PasswordToggleShowsPlaintext(t *testing.T) {
	s := &session.Session{
		Host:     "10.0.0.3",
		Port:     22,
		User:     "ops",
		AuthType: session.AuthTypePassword,
		Valid:    true,
		Password: "MySecretP@ss",
	}
	leaf := &session.SessionNode{Name: "test-pwd", IsDir: false, Session: s}
	root := &session.SessionNode{Name: "sessions", IsDir: true, Children: []*session.SessionNode{leaf}}
	leaf.Parent = root

	m := initialModel()
	m.tree = root
	m.cursor = 0

	// Hidden by default
	m.showPassword = false
	detail := m.renderDetail(80, 30)
	if strings.Contains(detail, "MySecretP@ss") {
		t.Errorf("password leaked when showPassword=false")
	}

	// Toggle on
	m.showPassword = true
	detail2 := m.renderDetail(80, 30)
	if !strings.Contains(detail2, "MySecretP@ss") {
		t.Errorf("password not shown when showPassword=true; got:\n%s", detail2)
	}
}

func TestViewNeverExceedsTerminalDimensions(t *testing.T) {
	model := initialModel()
	model.width = 60
	model.height = 15
	model.loadWarning = strings.Repeat("XShell source directory is unavailable ", 10)
	model.menu.OpenMenu(0, topMenus)

	view := model.View()
	if got := lipgloss.Height(view); got > model.height {
		t.Fatalf("view height = %d, terminal height = %d", got, model.height)
	}
	if got := lipgloss.Width(view); got > model.width {
		t.Fatalf("view width = %d, terminal width = %d", got, model.width)
	}
}

func TestTopMenuDropdownOverlaysWithoutReflow(t *testing.T) {
	model := initialModel()
	model.width = 80
	model.height = 20

	closed := model.View()
	model.menu.OpenMenu(0, topMenus)
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

func TestTopMenuBarShowsVersion(t *testing.T) {
	model := initialModel()
	model.width = 80
	line := model.renderTopMenuBar()
	if !strings.Contains(line, "xssh dev") {
		t.Fatalf("xssh version missing from menu bar: %q", line)
	}
	if lipgloss.Width(line) != model.width {
		t.Fatalf("menu bar width = %d, want %d", lipgloss.Width(line), model.width)
	}
}
