package shared

import (
	"errors"
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

// LoadSessionTree 加载本地和外部会话树。外部源失败时保留可用树并返回聚合错误。
func LoadSessionTree() (tree *session.SessionNode, sessionsDir string, loadErr error) {
	var err error
	sessionsDir, err = config.GetSessionsDir()
	if err != nil {
		return nil, "", fmt.Errorf("获取会话目录失败: %w", err)
	}
	tree, err = session.LoadSessionsTree(sessionsDir)
	if err != nil {
		return nil, sessionsDir, fmt.Errorf("加载本地会话失败: %w", err)
	}

	globalConfig, err := config.LoadGlobalConfig()
	if err != nil {
		return tree, sessionsDir, fmt.Errorf("加载全局配置失败: %w", err)
	}

	externalTrees, warnings := loadExternalSessionTrees(globalConfig)
	tree.Children = append(tree.Children, externalTrees...)
	tree.SetParent(nil)
	return tree, sessionsDir, warnings
}

func loadExternalSessionTrees(globalConfig *config.GlobalConfig) ([]*session.SessionNode, error) {
	var trees []*session.SessionNode
	var warnings []error
	appendSource := func(name string, sourceTree *session.SessionNode, err error) {
		if err != nil {
			warnings = append(warnings, fmt.Errorf("加载 %s 会话失败: %w", name, err))
			return
		}
		if sourceTree != nil {
			trees = append(trees, sourceTree)
		}
	}
	if globalConfig.SecureCRT.Enabled {
		sourceTree, err := session.LoadSecureCRTSessions(globalConfig.SecureCRT)
		appendSource("SecureCRT", sourceTree, err)
	}
	if globalConfig.XShell.Enabled {
		sourceTree, err := session.LoadXShellSessions(globalConfig.XShell)
		appendSource("XShell", sourceTree, err)
	}
	if globalConfig.MobaXterm.Enabled {
		sourceTree, err := session.LoadMobaXtermSessions(globalConfig.MobaXterm)
		appendSource("MobaXterm", sourceTree, err)
	}
	return trees, errors.Join(warnings...)
}

// SessionEntry 表示一个扁平化的会话条目（路径 + 会话对象）
type SessionEntry struct {
	Path    string           // 会话在树中的路径，如 "securecrt/prod/web-01"
	Session *session.Session // 会话对象
}

// FindSessionAllSources 在所有来源中查找唯一会话。
func FindSessionAllSources(sessionPath string) (*session.Session, error) {
	sessionsDir, err := config.GetSessionsDir()
	if err != nil {
		return nil, fmt.Errorf("获取会话目录失败: %w", err)
	}
	if local, err := session.FindSession(sessionsDir, sessionPath); err == nil {
		return local, nil
	} else if !errors.Is(err, session.ErrSessionNotFound) {
		return nil, err
	}

	globalConfig, configErr := config.LoadGlobalConfig()
	if configErr != nil {
		return nil, configErr
	}
	externalTrees, loadErr := loadExternalSessionTrees(globalConfig)
	root := &session.SessionNode{Name: "external", IsDir: true, Children: externalTrees}
	root.SetParent(nil)
	entries := collectSessionEntries(root, "")
	for _, entry := range entries {
		if entry.Path == sessionPath {
			return entry.Session, nil
		}
	}

	var matches []SessionEntry
	for _, entry := range entries {
		if strings.Contains(entry.Path, sessionPath) || sessionPath == filepath.Base(entry.Path) {
			matches = append(matches, entry)
		}
	}
	if len(matches) == 1 {
		return matches[0].Session, nil
	}
	if len(matches) > 1 {
		paths := make([]string, 0, len(matches))
		for _, match := range matches {
			paths = append(paths, match.Path)
		}
		return nil, fmt.Errorf("%w %q: %s", session.ErrSessionAmbiguous, sessionPath, strings.Join(paths, ", "))
	}
	return nil, errors.Join(fmt.Errorf("%w: %s", session.ErrSessionNotFound, sessionPath), loadErr)
}

// LoadAllSessionsFlat 加载所有来源的会话并返回扁平列表及加载警告。
func LoadAllSessionsFlat() ([]SessionEntry, error) {
	tree, _, err := LoadSessionTree()
	if tree == nil {
		return nil, err
	}
	return collectSessionEntries(tree, ""), err
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
