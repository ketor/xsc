package shared

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/term"

	"github.com/ketor/xsc/internal/session"
	"github.com/ketor/xsc/pkg/config"
)

// Command 定义一个 : 模式下的命令（TUI 和 xftp 选择器共用）
type Command struct {
	Name        string   // 主命令名, e.g. "q"
	Aliases     []string // 别名, e.g. ["quit"]
	Description string   // 中文描述
}

// MatchCommand 根据输入和命令注册表返回匹配的命令规范名，无匹配返回空字符串
func MatchCommand(input string, commands []Command) string {
	for _, cmd := range commands {
		if input == cmd.Name {
			return cmd.Name
		}
		for _, alias := range cmd.Aliases {
			if input == alias {
				return cmd.Name
			}
		}
	}
	return ""
}

// GetCommandCompletions 根据前缀和命令注册表返回匹配的命令列表
func GetCommandCompletions(prefix string, commands []Command) []Command {
	if prefix == "" {
		return commands
	}
	var result []Command
	for _, cmd := range commands {
		if strings.HasPrefix(cmd.Name, prefix) {
			result = append(result, cmd)
			continue
		}
		for _, alias := range cmd.Aliases {
			if strings.HasPrefix(alias, prefix) {
				result = append(result, cmd)
				break
			}
		}
	}
	return result
}

// GetIndent 获取节点的缩进字符串（根据父节点深度计算）
func GetIndent(node *session.SessionNode) string {
	depth := 0
	current := node
	for current.Parent != nil {
		depth++
		current = current.Parent
	}
	return strings.Repeat("  ", depth)
}

// LoadSessionTree 加载完整的 session 树，包括本地和外部来源（SecureCRT、XShell、MobaXterm）。
// 返回合并后的树根节点和 sessionsDir 路径。如果加载失败，tree 为 nil。
func LoadSessionTree() (tree *session.SessionNode, sessionsDir string) {
	var err error
	sessionsDir, err = config.GetSessionsDir()
	if err != nil {
		return nil, ""
	}

	tree, err = session.LoadSessionsTree(sessionsDir)
	if err != nil {
		return nil, ""
	}

	// 加载全局配置，添加外部 session 源
	globalConfig, err := config.LoadGlobalConfig()
	if err != nil {
		// 配置加载失败时仍返回本地会话树
		return tree, sessionsDir
	}

	// 如果启用了 SecureCRT，加载 SecureCRT 会话
	if globalConfig.SecureCRT.Enabled {
		scTree, err := session.LoadSecureCRTSessions(globalConfig.SecureCRT)
		if err == nil && scTree != nil {
			tree.Children = append(tree.Children, scTree)
		}
	}

	// 如果启用了 XShell，加载 XShell 会话
	if globalConfig.XShell.Enabled {
		xsTree, err := session.LoadXShellSessions(globalConfig.XShell)
		if err == nil && xsTree != nil {
			tree.Children = append(tree.Children, xsTree)
		}
	}

	// 如果启用了 MobaXterm，加载 MobaXterm 会话
	if globalConfig.MobaXterm.Enabled {
		mxTree, err := session.LoadMobaXtermSessions(globalConfig.MobaXterm)
		if err == nil && mxTree != nil {
			tree.Children = append(tree.Children, mxTree)
		}
	}

	return tree, sessionsDir
}

// SessionEntry 表示一个扁平化的会话条目（路径 + 会话对象）
type SessionEntry struct {
	Path    string           // 会话在树中的路径，如 "securecrt/prod/web-01"
	Session *session.Session // 会话对象
}

// FindSessionAllSources 在所有会话来源中查找会话（本地 YAML + SecureCRT/XShell/MobaXterm）
// 先尝试本地 YAML 快速路径，未找到则在完整会话树中查找（精确匹配 → 模糊匹配）
func FindSessionAllSources(sessionPath string) (*session.Session, error) {
	// 快速路径：先在本地 YAML 中查找
	sessionsDir, err := config.GetSessionsDir()
	if err == nil {
		if s, err := session.FindSession(sessionsDir, sessionPath); err == nil {
			return s, nil
		}
	}

	// 在完整会话树中查找（包括 SecureCRT/XShell/MobaXterm）
	tree, _ := LoadSessionTree()
	if tree == nil {
		return nil, fmt.Errorf("会话未找到: %s", sessionPath)
	}

	entries := collectSessionEntries(tree, "")

	// 精确匹配
	for _, e := range entries {
		if e.Path == sessionPath {
			return e.Session, nil
		}
	}

	// 模糊匹配：路径包含 或 名称匹配
	for _, e := range entries {
		if strings.Contains(e.Path, sessionPath) || sessionPath == filepath.Base(e.Path) {
			return e.Session, nil
		}
	}

	return nil, fmt.Errorf("会话未找到: %s", sessionPath)
}

// LoadAllSessionsFlat 加载所有来源的会话并以扁平列表返回（本地 YAML + SecureCRT/XShell/MobaXterm）
func LoadAllSessionsFlat() []SessionEntry {
	tree, _ := LoadSessionTree()
	if tree == nil {
		return nil
	}
	return collectSessionEntries(tree, "")
}

// collectSessionEntries 递归收集会话树中的所有叶子节点
func collectSessionEntries(node *session.SessionNode, prefix string) []SessionEntry {
	var results []SessionEntry
	for _, child := range node.Children {
		childPath := child.Name
		if prefix != "" {
			childPath = prefix + "/" + child.Name
		}
		if child.IsDir {
			results = append(results, collectSessionEntries(child, childPath)...)
		} else if child.Session != nil {
			results = append(results, SessionEntry{
				Path:    childPath,
				Session: child.Session,
			})
		}
	}
	return results
}

// CountSessions 统计树中的叶子节点（会话）数量
func CountSessions(node *session.SessionNode) int {
	count := 0
	for _, child := range node.Children {
		if child.IsDir {
			count += CountSessions(child)
		} else {
			count++
		}
	}
	return count
}

// ExpandAll 递归展开所有目录节点
func ExpandAll(node *session.SessionNode) {
	if node.IsDir {
		node.Expanded = true
		for _, child := range node.Children {
			ExpandAll(child)
		}
	}
}

// CollapseAll 递归折叠所有目录节点
func CollapseAll(node *session.SessionNode) {
	if node.IsDir {
		node.Expanded = false
		for _, child := range node.Children {
			CollapseAll(child)
		}
	}
}

// ContextMenuItem 定义右键菜单项（TUI 和 xftp 共用）
type ContextMenuItem struct {
	Label  string
	Key    string
	Action string
}

// ResolveSessionPassword 统一处理会话密码解析：
// 1. ResolvePassword() 延迟解密（支持 XSC_MASTER_PASSWORD 环境变量）
// 2. XSC_PASSWORD 环境变量注入
// 3. 非 TTY 环境密码缺失检测
func ResolveSessionPassword(s *session.Session) error {
	if err := s.ResolvePassword(); err != nil {
		return fmt.Errorf("密码解密失败: %w", err)
	}
	// XSC_PASSWORD 环境变量注入
	if s.AuthType == session.AuthTypePassword && s.Password == "" {
		if envPwd := os.Getenv("XSC_PASSWORD"); envPwd != "" {
			s.Password = envPwd
		}
	}
	// 非 TTY 环境下密码缺失，立即失败而非卡死
	if s.AuthType == session.AuthTypePassword && s.Password == "" {
		if !term.IsTerminal(int(os.Stdin.Fd())) {
			return fmt.Errorf("密码未设置且非 TTY 环境（设置 XSC_PASSWORD 环境变量）")
		}
	}
	return nil
}
