package shared

import (
	"fmt"
	"os"
	"syscall"

	"golang.org/x/term"

	"github.com/ketor/xsc/pkg/config"
)

// isTerminalFn 报告 stdin 是否为 TTY；测试可替换。
var isTerminalFn = func() bool {
	return term.IsTerminal(int(os.Stdin.Fd()))
}

// passwordPromptFn 在终端无回显地读取一行密码；测试可替换。
var passwordPromptFn = func(promptMsg string) (string, error) {
	fmt.Fprint(os.Stderr, promptMsg)
	pw, err := term.ReadPassword(int(syscall.Stdin))
	fmt.Fprintln(os.Stderr)
	if err != nil {
		return "", err
	}
	return string(pw), nil
}

// EnsureMasterPasswords 检查所有启用但密码仍为空的导入源，
// 在 TTY 环境下交互式提示用户输入主密码，将结果写回 cfg。
//
// 调用时机：xssh / xftp 中真正需要解密外部 session 的命令前
// （connect / exec / tui / ping / import-* 等）。
//
// 行为说明：
//   - 非 TTY：直接返回 nil（保持现有行为，错误延迟到 ResolvePassword 报）
//   - 对应源未启用：跳过
//   - 对应源已有密码（来自 YAML 或环境变量）：跳过
//   - prompter 返回错误：向上传播
func EnsureMasterPasswords(cfg *config.GlobalConfig) error {
	if cfg == nil || !isTerminalFn() {
		return nil
	}
	type entry struct {
		enabled bool
		ptr     *string
		label   string
	}
	for _, e := range []entry{
		{cfg.SecureCRT.Enabled, &cfg.SecureCRT.Password, "SecureCRT master password: "},
		{cfg.XShell.Enabled, &cfg.XShell.Password, "Xshell master password: "},
		{cfg.MobaXterm.Enabled, &cfg.MobaXterm.Password, "MobaXterm master password: "},
	} {
		if !e.enabled || *e.ptr != "" {
			continue
		}
		pw, err := passwordPromptFn(e.label)
		if err != nil {
			return fmt.Errorf("read master password: %w", err)
		}
		*e.ptr = pw
	}
	return nil
}
