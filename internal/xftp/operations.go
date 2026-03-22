package xftp

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

// InputOp 输入操作类型（用于 ModeInput）
type InputOp int

const (
	InputOpNone   InputOp = iota
	InputOpMkdir          // 新建目录
	InputOpRename         // 重命名
)

// ModeInput 输入模式（追加到 Mode 枚举外部管理，避免修改 Mode iota）
// 在 model 中用 inputOp 字段来区分

// handleYank 标记当前选中（或光标处）文件到 yank 缓冲区
func (m Model) handleYank() (tea.Model, tea.Cmd) {
	panel := m.activeFilterPanel()
	var files []FileEntry

	// 优先使用多选
	selected := panel.SelectedFiles()
	if len(selected) > 0 {
		files = selected
	} else if entry := panel.CurrentEntry(); entry != nil {
		files = []FileEntry{*entry}
	}

	if len(files) == 0 {
		m.statusMsg = "没有可标记的文件"
		return m, nil
	}

	m.yankFiles = make([]yankEntry, 0, len(files))
	for _, f := range files {
		m.yankFiles = append(m.yankFiles, yankEntry{
			Name:  f.Info.Name,
			Path:  path.Join(panel.cwd, f.Info.Name),
			Size:  f.Info.Size,
			IsDir: f.Info.IsDir,
		})
	}
	m.yankSide = m.activePanel
	m.yankDir = panel.cwd

	m.statusMsg = fmt.Sprintf("已标记 %d 个文件", len(m.yankFiles))
	panel.ClearSelection()
	return m, nil
}

// handlePaste 将 yank 缓冲区的文件传输到对面面板
func (m Model) handlePaste() (tea.Model, tea.Cmd) {
	if len(m.yankFiles) == 0 {
		m.statusMsg = "没有标记的文件（先用 y 标记）"
		return m, nil
	}

	if !m.connected || m.remoteFS == nil {
		m.statusMsg = "远程未连接，无法传输"
		return m, nil
	}

	// 确定传输方向
	var dir Direction
	var destDir string
	if m.yankSide == PanelLeft {
		// 本地→远程
		dir = Upload
		destDir = m.remotePanel.cwd
	} else {
		// 远程→本地
		dir = Download
		destDir = m.localPanel.cwd
	}

	// 检查目标是否存在同名文件
	var targetFS FileSystem
	if m.yankSide == PanelLeft {
		targetFS = m.remoteFS
	} else {
		targetFS = m.localPanel.fs
	}

	var conflicts []string
	for _, yf := range m.yankFiles {
		destPath := path.Join(destDir, yf.Name)
		if _, err := targetFS.Stat(destPath); err == nil {
			conflicts = append(conflicts, yf.Name)
		}
	}

	if len(conflicts) > 0 {
		// 存在冲突，进入覆盖确认模式
		m.overwriteConflicts = conflicts
		m.pendingPasteDir = dir
		m.pendingPasteDestDir = destDir
		m.mode = ModeOverwriteConfirm
		m.statusMsg = fmt.Sprintf("目标已存在 %d 个同名文件/目录，是否覆盖？(y/n)", len(conflicts))
		return m, nil
	}

	// 无冲突，直接执行
	return m.executePaste(dir, destDir)
}

// executePaste 执行粘贴传输
func (m Model) executePaste(dir Direction, destDir string) (tea.Model, tea.Cmd) {
	// 添加传输任务（目录递归展开为文件）
	dirCount := 0
	for _, yf := range m.yankFiles {
		if yf.IsDir {
			dirCount++
			m.expandDirTasks(yf, destDir, dir)
		} else {
			dest := path.Join(destDir, yf.Name)
			m.transfer.AddTask(yf.Path, dest, dir, yf.Size)
		}
	}
	if dirCount > 0 {
		m.transfer.RecordDirComplete(dirCount)
	}

	// 清空 yank 缓冲区
	m.yankFiles = nil
	m.statusMsg = "传输已加入队列"

	// 启动传输
	return m, tea.Batch(
		m.transfer.StartNext(m.remoteFS.SFTPClient()),
		m.transfer.ListenProgress(),
	)
}

// expandDirTasks 递归展开目录，将其中所有文件添加为传输任务
func (m *Model) expandDirTasks(yf yankEntry, destDir string, dir Direction) {
	baseDir := path.Dir(yf.Path) // yf.Path 的父目录，用于计算相对路径

	if dir == Upload {
		// 本地→远程：用 filepath.Walk 遍历本地目录
		_ = filepath.Walk(yf.Path, func(p string, info os.FileInfo, err error) error {
			if err != nil {
				return nil // 跳过无法访问的文件
			}
			if info.IsDir() {
				return nil // 跳过目录本身，doUpload 会自动创建远程目录
			}
			rel, _ := filepath.Rel(baseDir, p)
			dest := path.Join(destDir, filepath.ToSlash(rel))
			m.transfer.AddTask(p, dest, dir, info.Size())
			return nil
		})
	} else {
		// 远程→本地：用 sftp.Client.Walk 遍历远程目录
		sftpClient := m.remoteFS.SFTPClient()
		walker := sftpClient.Walk(yf.Path)
		for walker.Step() {
			if walker.Err() != nil {
				continue
			}
			info := walker.Stat()
			if info.IsDir() {
				continue // 跳过目录本身，doDownload 会自动创建本地目录
			}
			remotePath := walker.Path()
			rel, _ := filepath.Rel(baseDir, remotePath)
			dest := filepath.Join(destDir, rel)
			m.transfer.AddTask(remotePath, dest, dir, info.Size())
		}
	}
}

// handleOverwriteConfirmKey 处理覆盖确认按键
func (m Model) handleOverwriteConfirmKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "y", "Y":
		// 确认覆盖，执行传输
		dir := m.pendingPasteDir
		destDir := m.pendingPasteDestDir
		m.mode = ModeNormal
		m.overwriteConflicts = nil
		m.pendingPasteDestDir = ""
		return m.executePaste(dir, destDir)
	case "n", "N", "esc":
		// 取消
		m.mode = ModeNormal
		m.overwriteConflicts = nil
		m.pendingPasteDestDir = ""
		m.statusMsg = "已取消"
		return m, nil
	}
	return m, nil
}

// handleDelete 触发删除确认对话框
func (m Model) handleDelete() (tea.Model, tea.Cmd) {
	panel := m.activeFilterPanel()
	var files []FileEntry

	// 优先使用多选
	selected := panel.SelectedFiles()
	if len(selected) > 0 {
		files = selected
	} else if entry := panel.CurrentEntry(); entry != nil {
		files = []FileEntry{*entry}
	}

	if len(files) == 0 {
		m.statusMsg = "没有可删除的文件"
		return m, nil
	}

	// 保存待删除文件信息，进入确认模式
	m.confirmFiles = make([]confirmEntry, 0, len(files))
	for _, f := range files {
		m.confirmFiles = append(m.confirmFiles, confirmEntry{
			Name:  f.Info.Name,
			Path:  path.Join(panel.cwd, f.Info.Name),
			IsDir: f.Info.IsDir,
		})
	}
	m.confirmPanel = m.activePanel
	m.mode = ModeConfirm

	if len(files) == 1 {
		m.statusMsg = fmt.Sprintf("确认删除 %s？(y/n)", files[0].Info.Name)
	} else {
		m.statusMsg = fmt.Sprintf("确认删除 %d 个文件？(y/n)", len(files))
	}

	return m, nil
}

// handleConfirmKey 处理确认对话框按键
func (m Model) handleConfirmKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "y", "Y":
		// 确认删除
		return m.executeDelete()
	case "n", "N", "esc":
		// 取消
		m.mode = ModeNormal
		m.confirmFiles = nil
		m.statusMsg = "已取消"
		return m, nil
	}
	return m, nil
}

// executeDelete 执行删除操作
func (m Model) executeDelete() (tea.Model, tea.Cmd) {
	files := m.confirmFiles
	panelSide := m.confirmPanel
	m.mode = ModeNormal
	m.confirmFiles = nil

	// 获取文件系统
	var fs FileSystem
	if panelSide == PanelLeft {
		fs = m.localPanel.fs
	} else {
		if m.remoteFS == nil {
			m.statusMsg = "远程未连接"
			return m, nil
		}
		fs = m.remoteFS
	}

	// 异步删除
	return m, func() tea.Msg {
		for _, f := range files {
			if err := fs.Remove(f.Path); err != nil {
				return FileOpErrorMsg{Op: "delete", Err: fmt.Errorf("删除 %s 失败: %w", f.Name, err)}
			}
		}
		return FileOpCompleteMsg{Op: "delete"}
	}
}

// handleMkdir 进入 mkdir 输入模式
func (m Model) handleMkdir() (tea.Model, tea.Cmd) {
	m.inputOp = InputOpMkdir
	m.inputPanel = m.activePanel
	m.opInput.SetValue("")
	m.opInput.Placeholder = "新建目录名"
	m.opInput.Prompt = "mkdir: "
	m.opInput.Focus()
	m.mode = ModeInput
	return m, textinput.Blink
}

// handleRename 进入 rename 输入模式
func (m Model) handleRename() (tea.Model, tea.Cmd) {
	panel := m.activeFilterPanel()
	entry := panel.CurrentEntry()
	if entry == nil {
		m.statusMsg = "没有可重命名的文件"
		return m, nil
	}

	m.inputOp = InputOpRename
	m.inputPanel = m.activePanel
	m.inputOldName = entry.Info.Name
	m.opInput.SetValue(entry.Info.Name)
	m.opInput.Placeholder = "新名称"
	m.opInput.Prompt = "rename: "
	m.opInput.Focus()
	m.mode = ModeInput
	return m, textinput.Blink
}

// handleInputKey 处理输入模式按键
func (m Model) handleInputKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEsc:
		// 取消输入
		m.mode = ModeNormal
		m.inputOp = InputOpNone
		m.opInput.Blur()
		m.statusMsg = "已取消"
		return m, nil

	case tea.KeyEnter:
		// 执行操作
		value := m.opInput.Value()
		if value == "" {
			m.statusMsg = "名称不能为空"
			return m, nil
		}
		m.opInput.Blur()
		m.mode = ModeNormal

		switch m.inputOp {
		case InputOpMkdir:
			return m.executeMkdir(value)
		case InputOpRename:
			return m.executeRename(value)
		}
		m.inputOp = InputOpNone
		return m, nil

	default:
		var cmd tea.Cmd
		m.opInput, cmd = m.opInput.Update(msg)
		return m, cmd
	}
}

// validateFileName 验证文件/目录名不包含路径穿越字符
func validateFileName(name string) error {
	if strings.Contains(name, "/") || strings.Contains(name, "..") {
		return fmt.Errorf("名称不能包含 '/' 或 '..'")
	}
	return nil
}

// executeMkdir 执行创建目录
func (m Model) executeMkdir(name string) (tea.Model, tea.Cmd) {
	if err := validateFileName(name); err != nil {
		m.statusMsg = err.Error()
		m.inputOp = InputOpNone
		return m, nil
	}

	panelSide := m.inputPanel
	m.inputOp = InputOpNone

	var fs FileSystem
	var cwd string
	if panelSide == PanelLeft {
		fs = m.localPanel.fs
		cwd = m.localPanel.cwd
	} else {
		if m.remoteFS == nil {
			m.statusMsg = "远程未连接"
			return m, nil
		}
		fs = m.remoteFS
		cwd = m.remotePanel.cwd
	}

	dirPath := path.Join(cwd, name)
	return m, func() tea.Msg {
		if err := fs.Mkdir(dirPath); err != nil {
			return FileOpErrorMsg{Op: "mkdir", Err: err}
		}
		return FileOpCompleteMsg{Op: "mkdir"}
	}
}

// executeRename 执行重命名
func (m Model) executeRename(newName string) (tea.Model, tea.Cmd) {
	if err := validateFileName(newName); err != nil {
		m.statusMsg = err.Error()
		m.inputOp = InputOpNone
		m.inputOldName = ""
		return m, nil
	}

	panelSide := m.inputPanel
	oldName := m.inputOldName
	m.inputOp = InputOpNone
	m.inputOldName = ""

	var fs FileSystem
	var cwd string
	if panelSide == PanelLeft {
		fs = m.localPanel.fs
		cwd = m.localPanel.cwd
	} else {
		if m.remoteFS == nil {
			m.statusMsg = "远程未连接"
			return m, nil
		}
		fs = m.remoteFS
		cwd = m.remotePanel.cwd
	}

	oldPath := path.Join(cwd, oldName)
	newPath := path.Join(cwd, newName)
	return m, func() tea.Msg {
		if err := fs.Rename(oldPath, newPath); err != nil {
			return FileOpErrorMsg{Op: "rename", Err: err}
		}
		return FileOpCompleteMsg{Op: "rename"}
	}
}
