package tui

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/ketor/xsc/internal/session"
	"github.com/ketor/xsc/internal/shared"
	internalssh "github.com/ketor/xsc/internal/ssh"
)

// loadSessions 加载会话
func (m *Model) loadSessions() tea.Cmd {
	return func() tea.Msg {
		tree, sessionsDir, err := shared.LoadSessionTree()
		return sessionsLoadedMsg{tree: tree, sessionsDir: sessionsDir, err: err}
	}
}

// newSession 创建新会话
func (m Model) newSession() tea.Cmd {
	return func() tea.Msg {
		selected := m.getSelectedNode()
		var dir string

		if selected != nil {
			if selected.IsDir {
				dir = filepath.Join(m.sessionsDir, selected.GetPath())
			} else if selected.Parent != nil {
				dir = filepath.Join(m.sessionsDir, selected.Parent.GetPath())
			}
		}

		if dir == "" {
			dir = m.sessionsDir
		}

		// 创建模板文件
		templatePath := filepath.Join(dir, "new-session.yaml")
		template := &session.Session{
			Host:     "example.com",
			Port:     22,
			User:     "root",
			AuthType: "agent",
		}

		if err := session.SaveSession(template, templatePath); err != nil {
			return nil
		}

		// 加载刚创建的会话并打开编辑器
		newSession, err := session.LoadSession(templatePath)
		if err != nil {
			return nil
		}

		// 使用 execEditCommand 打开编辑器
		return m.execEditCommand(newSession)()
	}
}

// prepareNewSession 准备新建会话，返回消息触发状态改变
func (m Model) prepareNewSession() tea.Cmd {
	return func() tea.Msg {
		selected := m.getSelectedNode()
		var dir string

		if selected != nil {
			// 检查是否在 SecureCRT 目录下
			if selected.IsReadOnly() {
				return showErrorMsg{err: fmt.Errorf("cannot create session in imported directory (read-only)")}
			}

			if selected.IsDir {
				// 如果选中的是目录，在该目录下创建
				dir = filepath.Join(m.sessionsDir, selected.GetPath())
			} else if selected.Parent != nil {
				// 如果选中的是会话文件，在父目录下创建
				parentPath := selected.Parent.GetPath()
				// 根节点的GetPath返回"sessions"，需要特殊处理
				if parentPath == "sessions" {
					dir = m.sessionsDir
				} else {
					dir = filepath.Join(m.sessionsDir, parentPath)
				}
			}
		}

		if dir == "" {
			dir = m.sessionsDir
		}

		return prepareNewSessionMsg{dir: dir}
	}
}

// createNewSession 创建新会话 - 第一步：准备临时文件
func (m Model) createNewSession(dir, filename string) tea.Cmd {
	targetPath := filepath.Join(dir, filename)

	// 检查文件是否已存在
	if _, err := os.Stat(targetPath); err == nil {
		return func() tea.Msg {
			return showErrorMsg{err: fmt.Errorf("file already exists: %s", filename)}
		}
	}

	// 创建临时文件
	tempFile, err := os.CreateTemp("", "xsc-new-session-*.yaml")
	if err != nil {
		return func() tea.Msg {
			return showErrorMsg{err: fmt.Errorf("failed to create temp file: %w", err)}
		}
	}
	tempPath := tempFile.Name()
	tempFile.Close()

	// 写入模板内容
	template := &session.Session{
		Host:     "example.com",
		Port:     22,
		User:     "root",
		AuthType: "agent",
	}

	if err := session.SaveSession(template, tempPath); err != nil {
		os.Remove(tempPath)
		return func() tea.Msg {
			return showErrorMsg{err: fmt.Errorf("failed to write template: %w", err)}
		}
	}

	// 使用 tea.Exec 打开编辑器（这会暂停 TUI）
	// 通过消息传递上下文，避免全局/共享状态
	return tea.Exec(newSessionEditorProcess{tempPath: tempPath}, func(err error) tea.Msg {
		return newSessionEditorCompleteMsg{
			err:        err,
			tempPath:   tempPath,
			targetPath: targetPath,
		}
	})
}

// handleNewSessionComplete 处理新建会话编辑器关闭后的逻辑
func (m Model) handleNewSessionComplete(msg newSessionEditorCompleteMsg) tea.Cmd {
	return func() tea.Msg {
		tempPath := msg.tempPath
		targetPath := msg.targetPath

		if tempPath == "" {
			return editorCompleteMsg{err: nil}
		}

		// 编辑器非正常退出（如 :q!），删除临时文件
		if msg.err != nil {
			os.Remove(tempPath)
			return editorCompleteMsg{err: nil} // 不显示错误，因为是用户取消
		}

		// 检查临时文件是否还有效（用户可能删除了内容）
		if _, err := os.Stat(tempPath); os.IsNotExist(err) {
			return editorCompleteMsg{err: nil}
		}

		// 验证文件内容
		newSession, err := session.LoadSession(tempPath)
		if err != nil {
			os.Remove(tempPath)
			return showErrorMsg{err: fmt.Errorf("failed to load session: %w", err)}
		}

		// 如果验证通过，移动到目标位置
		if err := session.SaveSession(newSession, targetPath); err != nil {
			os.Remove(tempPath)
			return showErrorMsg{err: fmt.Errorf("failed to save session: %w", err)}
		}

		// 删除临时文件
		os.Remove(tempPath)

		return editorCompleteMsg{err: nil}
	}
}

// prepareRenameSession 准备重命名会话，返回消息触发状态改变
func (m Model) prepareRenameSession(node *session.SessionNode) tea.Cmd {
	return func() tea.Msg {
		return prepareRenameSessionMsg{node: node}
	}
}

// renameSession 执行会话重命名
func (m Model) renameSession(node *session.SessionNode, newName string) tea.Cmd {
	return func() tea.Msg {
		if node.Session == nil || node.Session.FilePath == "" {
			return showErrorMsg{err: fmt.Errorf("invalid session")}
		}

		oldPath := node.Session.FilePath
		dir := filepath.Dir(oldPath)
		newPath := filepath.Join(dir, newName)

		// 检查目标文件是否已存在
		if _, err := os.Stat(newPath); err == nil {
			return showErrorMsg{err: fmt.Errorf("file already exists: %s", newName)}
		}

		// 执行重命名
		if err := os.Rename(oldPath, newPath); err != nil {
			return showErrorMsg{err: fmt.Errorf("failed to rename: %w", err)}
		}

		return editorCompleteMsg{err: nil}
	}
}

// deleteSession 删除会话（带确认）
func (m Model) deleteSession(node *session.SessionNode) tea.Cmd {
	return func() tea.Msg {
		if node.Session == nil {
			return nil
		}

		err := os.Remove(node.Session.FilePath)
		if err != nil {
			return showErrorMsg{err: fmt.Errorf("failed to delete session: %w", err)}
		}

		return m.loadSessions()()
	}
}

// prepareDeleteConfirm 准备删除确认，返回消息触发状态改变
func (m Model) prepareDeleteConfirm(node *session.SessionNode) tea.Cmd {
	return func() tea.Msg {
		return prepareDeleteConfirmMsg{node: node}
	}
}

// sshProcess 实现 tea.ExecCommand 接口，使用纯 Go 建立 SSH 连接
type sshProcess struct {
	session *session.Session
}

func (p sshProcess) Run() error {
	return internalssh.Connect(p.session)
}

func (p sshProcess) SetStdin(r io.Reader)  {}
func (p sshProcess) SetStdout(w io.Writer) {}
func (p sshProcess) SetStderr(w io.Writer) {}

// execSSHCommand 通过 tea.Exec 执行 SSH 连接
// tea.Exec 会让 Bubble Tea 正确暂停 TUI 并恢复终端到正常状态，
// 然后执行 SSH 连接，结束后重新进入 TUI
func (m Model) execSSHCommand(s *session.Session) tea.Cmd {
	return tea.Exec(sshProcess{session: s}, func(err error) tea.Msg {
		return connectCompleteMsg{err: err}
	})
}

// execEditCommand 执行编辑命令，确保 TUI 完全退出
func (m Model) execEditCommand(s *session.Session) tea.Cmd {
	return tea.Exec(editorProcess{filepath: s.FilePath}, func(err error) tea.Msg {
		return editorCompleteMsg{err: err}
	})
}

// editorProcess 实现 tea.Exec 接口
type editorProcess struct {
	filepath string
}

func (p editorProcess) Run() error {
	editor := os.Getenv("EDITOR")
	if editor == "" {
		editor = "vi"
	}

	cmd := exec.Command(editor, p.filepath)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func (p editorProcess) SetStdin(r io.Reader)  {}
func (p editorProcess) SetStdout(w io.Writer) {}
func (p editorProcess) SetStderr(w io.Writer) {}

// newSessionEditorProcess 实现 tea.Exec 接口用于新建会话
type newSessionEditorProcess struct {
	tempPath string
}

func (p newSessionEditorProcess) Run() error {
	editor := os.Getenv("EDITOR")
	if editor == "" {
		editor = "vi"
	}

	cmd := exec.Command(editor, p.tempPath)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	return cmd.Run()
}

func (p newSessionEditorProcess) SetStdin(r io.Reader)  {}
func (p newSessionEditorProcess) SetStdout(w io.Writer) {}
func (p newSessionEditorProcess) SetStderr(w io.Writer) {}

// moveCursor 移动光标
func (m *Model) moveCursor(delta int) {
	visibleNodes := m.getVisibleNodes()
	if len(visibleNodes) == 0 {
		return
	}

	m.cursor += delta
	if m.cursor < 0 {
		m.cursor = 0
	}
	if m.cursor >= len(visibleNodes) {
		m.cursor = len(visibleNodes) - 1
	}
}

// getSelectedNode 获取当前选中的节点
func (m Model) getSelectedNode() *session.SessionNode {
	visibleNodes := m.getVisibleNodes()
	if m.cursor >= 0 && m.cursor < len(visibleNodes) {
		return visibleNodes[m.cursor]
	}
	return nil
}

// getVisibleNodes 获取可见节点列表（根据搜索查询过滤）
func (m Model) getVisibleNodes() []*session.SessionNode {
	if m.tree == nil {
		return nil
	}

	allNodes := m.tree.FlattenVisible()

	// 如果有搜索查询，过滤节点
	if m.searchQuery != "" {
		query := strings.ToLower(m.searchQuery)
		var filtered []*session.SessionNode
		for _, node := range allNodes {
			if strings.Contains(strings.ToLower(node.Name), query) {
				filtered = append(filtered, node)
			}
		}
		return filtered
	}

	return allNodes
}

// expandAll 展开所有目录
func (m Model) expandAll(node *session.SessionNode) {
	shared.ExpandAll(node)
}

// collapseAll 折叠所有目录
func (m Model) collapseAll(node *session.SessionNode) {
	shared.CollapseAll(node)
}

// searchNext 查找下一个/上一个匹配项
func (m *Model) searchNext(direction int) {
	if m.searchQuery == "" {
		return
	}

	visibleNodes := m.getVisibleNodes()
	if len(visibleNodes) == 0 {
		return
	}

	query := strings.ToLower(m.searchQuery)
	startIdx := m.cursor

	// 从当前位置开始搜索
	for i := 1; i <= len(visibleNodes); i++ {
		idx := startIdx + (i * direction)

		// 循环搜索
		if idx >= len(visibleNodes) {
			idx = idx - len(visibleNodes)
		} else if idx < 0 {
			idx = idx + len(visibleNodes)
		}

		if strings.Contains(strings.ToLower(visibleNodes[idx].Name), query) {
			m.cursor = idx
			return
		}
	}
}

// matchCommand 根据输入返回匹配的命令规范名，无匹配返回空字符串
func matchCommand(input string) string {
	return shared.MatchCommand(input, commands)
}

// getCommandCompletions 根据前缀返回匹配的命令列表
func getCommandCompletions(prefix string) []Command {
	return shared.GetCommandCompletions(prefix, commands)
}
