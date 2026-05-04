package shared

import (
	"errors"
	"fmt"
	"testing"

	"github.com/ketor/xsc/pkg/config"
)

// fakePrompter 记录调用并返回预设响应
type fakePrompter struct {
	responses map[string]string
	errOn     map[string]error
	calls     []string
}

func (f *fakePrompter) prompt(msg string) (string, error) {
	f.calls = append(f.calls, msg)
	if err, ok := f.errOn[msg]; ok {
		return "", err
	}
	if r, ok := f.responses[msg]; ok {
		return r, nil
	}
	return "", fmt.Errorf("fakePrompter: no response for %q", msg)
}

// withTestPromptDeps 临时替换 isTerminalFn 和 passwordPromptFn，测试结束后还原
func withTestPromptDeps(t *testing.T, isTTY bool, p *fakePrompter) {
	t.Helper()
	origTerm := isTerminalFn
	origPrompt := passwordPromptFn
	isTerminalFn = func() bool { return isTTY }
	passwordPromptFn = p.prompt
	t.Cleanup(func() {
		isTerminalFn = origTerm
		passwordPromptFn = origPrompt
	})
}

// TestEnsureMasterPasswords_PromptsWhenEnabledEmptyAndTTY 验证：启用且密码空且 TTY 时，prompt
func TestEnsureMasterPasswords_PromptsWhenEnabledEmptyAndTTY(t *testing.T) {
	cfg := &config.GlobalConfig{
		SecureCRT: config.SecureCRTConfig{Enabled: true, Password: ""},
		XShell:    config.XShellConfig{Enabled: false},
		MobaXterm: config.MobaXtermConfig{Enabled: false},
	}
	p := &fakePrompter{responses: map[string]string{
		"SecureCRT master password: ": "user-typed",
	}}
	withTestPromptDeps(t, true, p)

	if err := EnsureMasterPasswords(cfg); err != nil {
		t.Fatalf("EnsureMasterPasswords: %v", err)
	}
	if cfg.SecureCRT.Password != "user-typed" {
		t.Errorf("SecureCRT.Password = %q, want user-typed", cfg.SecureCRT.Password)
	}
	if len(p.calls) != 1 {
		t.Errorf("prompter calls = %d, want 1: %v", len(p.calls), p.calls)
	}
}

// TestEnsureMasterPasswords_NoPromptWhenAlreadySet 验证：已有密码时不 prompt
func TestEnsureMasterPasswords_NoPromptWhenAlreadySet(t *testing.T) {
	cfg := &config.GlobalConfig{
		SecureCRT: config.SecureCRTConfig{Enabled: true, Password: "already-set"},
	}
	p := &fakePrompter{}
	withTestPromptDeps(t, true, p)

	if err := EnsureMasterPasswords(cfg); err != nil {
		t.Fatalf("EnsureMasterPasswords: %v", err)
	}
	if len(p.calls) != 0 {
		t.Errorf("should not prompt when password already set, calls: %v", p.calls)
	}
	if cfg.SecureCRT.Password != "already-set" {
		t.Errorf("password should not be modified, got %q", cfg.SecureCRT.Password)
	}
}

// TestEnsureMasterPasswords_NoPromptWhenSourceDisabled 验证：源未启用时不 prompt
func TestEnsureMasterPasswords_NoPromptWhenSourceDisabled(t *testing.T) {
	cfg := &config.GlobalConfig{
		SecureCRT: config.SecureCRTConfig{Enabled: false, Password: ""},
		XShell:    config.XShellConfig{Enabled: false, Password: ""},
		MobaXterm: config.MobaXtermConfig{Enabled: false, Password: ""},
	}
	p := &fakePrompter{}
	withTestPromptDeps(t, true, p)

	if err := EnsureMasterPasswords(cfg); err != nil {
		t.Fatalf("EnsureMasterPasswords: %v", err)
	}
	if len(p.calls) != 0 {
		t.Errorf("should not prompt for disabled sources, calls: %v", p.calls)
	}
}

// TestEnsureMasterPasswords_NoPromptWhenNotTTY 验证：非 TTY 场景下不 prompt，返回 nil
func TestEnsureMasterPasswords_NoPromptWhenNotTTY(t *testing.T) {
	cfg := &config.GlobalConfig{
		SecureCRT: config.SecureCRTConfig{Enabled: true, Password: ""},
	}
	p := &fakePrompter{}
	withTestPromptDeps(t, false, p)

	if err := EnsureMasterPasswords(cfg); err != nil {
		t.Fatalf("EnsureMasterPasswords: %v", err)
	}
	if len(p.calls) != 0 {
		t.Errorf("should not prompt in non-TTY, calls: %v", p.calls)
	}
	if cfg.SecureCRT.Password != "" {
		t.Errorf("password should remain empty in non-TTY, got %q", cfg.SecureCRT.Password)
	}
}

// TestEnsureMasterPasswords_PromptsAllEnabledEmpty 验证：多个源同时启用且空，逐个 prompt
func TestEnsureMasterPasswords_PromptsAllEnabledEmpty(t *testing.T) {
	cfg := &config.GlobalConfig{
		SecureCRT: config.SecureCRTConfig{Enabled: true},
		XShell:    config.XShellConfig{Enabled: true},
		MobaXterm: config.MobaXtermConfig{Enabled: true},
	}
	p := &fakePrompter{responses: map[string]string{
		"SecureCRT master password: ": "sc",
		"Xshell master password: ":    "xs",
		"MobaXterm master password: ": "mx",
	}}
	withTestPromptDeps(t, true, p)

	if err := EnsureMasterPasswords(cfg); err != nil {
		t.Fatalf("EnsureMasterPasswords: %v", err)
	}
	if cfg.SecureCRT.Password != "sc" {
		t.Errorf("SecureCRT.Password = %q, want sc", cfg.SecureCRT.Password)
	}
	if cfg.XShell.Password != "xs" {
		t.Errorf("XShell.Password = %q, want xs", cfg.XShell.Password)
	}
	if cfg.MobaXterm.Password != "mx" {
		t.Errorf("MobaXterm.Password = %q, want mx", cfg.MobaXterm.Password)
	}
	if len(p.calls) != 3 {
		t.Errorf("calls = %d, want 3: %v", len(p.calls), p.calls)
	}
}

// TestEnsureMasterPasswords_PrompterErrorPropagates 验证：prompter 错误向上传播
func TestEnsureMasterPasswords_PrompterErrorPropagates(t *testing.T) {
	cfg := &config.GlobalConfig{
		SecureCRT: config.SecureCRTConfig{Enabled: true},
	}
	want := errors.New("read failed")
	p := &fakePrompter{errOn: map[string]error{
		"SecureCRT master password: ": want,
	}}
	withTestPromptDeps(t, true, p)

	err := EnsureMasterPasswords(cfg)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, want) {
		t.Errorf("err = %v, want it to wrap %v", err, want)
	}
}
