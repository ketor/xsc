package xftp

import (
	"fmt"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/ketor/xsc/internal/session"
)

// newMouseTestXftpModel 创建带有文件条目的 xftp Model，用于鼠标测试
func newMouseTestXftpModel() Model {
	m := NewModel(&session.Session{
		Host: "192.168.1.1", Port: 22, User: "root",
		AuthType: session.AuthTypePassword, Password: "pass", Valid: true,
	})
	m.width = 100
	m.height = 40
	m.connected = true
	m.updatePanelSizes()

	m.localPanel.entries = []FileEntry{
		{Info: FileInfo{Name: "dir1", IsDir: true}},
		{Info: FileInfo{Name: "file1.txt", IsDir: false, Size: 100}},
		{Info: FileInfo{Name: "file2.txt", IsDir: false, Size: 200}},
		{Info: FileInfo{Name: "file3.txt", IsDir: false, Size: 300}},
		{Info: FileInfo{Name: "file4.txt", IsDir: false, Size: 400}},
	}
	m.localPanel.allEntries = make([]FileEntry, len(m.localPanel.entries))
	copy(m.localPanel.allEntries, m.localPanel.entries)

	m.remotePanel.entries = []FileEntry{
		{Info: FileInfo{Name: "remote-dir", IsDir: true}},
		{Info: FileInfo{Name: "remote1.txt", IsDir: false, Size: 500}},
		{Info: FileInfo{Name: "remote2.txt", IsDir: false, Size: 600}},
	}
	m.remotePanel.allEntries = make([]FileEntry, len(m.remotePanel.entries))
	copy(m.remotePanel.allEntries, m.remotePanel.entries)

	return m
}

// newMouseTestSelector 创建带有会话树的 Selector，用于鼠标测试
func newMouseTestSelector() Selector {
	s := NewSelector()
	s.width = 120
	s.height = 40
	s.loading = false
	s.tree = &session.SessionNode{
		Name: "root", IsDir: true, Expanded: true,
		Children: []*session.SessionNode{
			{Name: "dir1", IsDir: true, Expanded: true, Children: []*session.SessionNode{
				{Name: "server-1", IsDir: false, Session: &session.Session{
					Host: "10.0.0.1", Port: 22, User: "root", Valid: true,
				}},
			}},
			{Name: "server-2", IsDir: false, Session: &session.Session{
				Host: "10.0.0.2", Port: 22, User: "root", Valid: true,
			}},
		},
	}
	s.tree.SetParent(nil)
	s.updateFlatNodes()
	return s
}

// ============================================================
// B1: panelHeaderLines 常量使用
// ============================================================

// TestHandleMouseHeaderOffset 测试鼠标点击使用 panelHeaderLines 常量偏移
func TestHandleMouseHeaderOffset(t *testing.T) {
	m := newMouseTestXftpModel()

	// 点击 Y=panelHeaderLines（=3）应映射到第一个文件（index 0）
	msg := tea.MouseMsg{
		X: 5, Y: panelHeaderLines + 1,
		Button: tea.MouseButtonLeft, Action: tea.MouseActionPress,
	}
	result, _ := m.Update(msg)
	m2 := result.(Model)

	if m2.localPanel.cursor != 0 {
		t.Errorf("Y=%d 应映射到文件索引 0，实际光标: %d", panelHeaderLines, m2.localPanel.cursor)
	}

	// 点击 Y=panelHeaderLines+1 应映射到第二个文件（index 1）
	msg2 := tea.MouseMsg{
		X: 5, Y: panelHeaderLines + 2,
		Button: tea.MouseButtonLeft, Action: tea.MouseActionPress,
	}
	result2, _ := m2.Update(msg2)
	m3 := result2.(Model)

	if m3.localPanel.cursor != 1 {
		t.Errorf("Y=%d 应映射到文件索引 1，实际光标: %d", panelHeaderLines+1, m3.localPanel.cursor)
	}
}

// TestHandleMouseHeaderClickIgnored 测试点击面板头部区域（Y < panelHeaderLines）时光标不变
func TestHandleMouseHeaderClickIgnored(t *testing.T) {
	m := newMouseTestXftpModel()
	m.localPanel.cursor = 2

	// panelHeaderLines=3，测试 Y=0, 1, 2 都应被忽略
	for y := 0; y < panelHeaderLines; y++ {
		msg := tea.MouseMsg{
			X: 5, Y: y + 1,
			Button: tea.MouseButtonLeft, Action: tea.MouseActionPress,
		}
		result, _ := m.Update(msg)
		m2 := result.(Model)

		// 点击头部区域仍然切换面板，但不改变光标
		if m2.localPanel.cursor != 2 {
			t.Errorf("Y=%d（头部区域）不应改变光标，期望 2，实际: %d", y, m2.localPanel.cursor)
		}
	}
}

// ============================================================
// 滚轮测试
// ============================================================

// TestHandleMouseWheelSwitchesPanel 测试滚轮事件自动切换到滚轮所在面板
func TestHandleMouseWheelSwitchesPanel(t *testing.T) {
	m := newMouseTestXftpModel()
	m.activePanel = PanelLeft

	// 在右面板区域滚动（X >= width/2 = 50）
	msg := tea.MouseMsg{X: 60, Y: 5, Button: tea.MouseButtonWheelDown}
	result, _ := m.Update(msg)
	m2 := result.(Model)

	if m2.activePanel != PanelRight {
		t.Errorf("右侧滚轮后 activePanel 应为 PanelRight，实际: %d", m2.activePanel)
	}
}

// TestHandleMouseWheelLeftPanel 测试左面板滚轮不切换面板
func TestHandleMouseWheelLeftPanel(t *testing.T) {
	m := newMouseTestXftpModel()
	m.activePanel = PanelRight

	// 在左面板区域滚动（X < 50）
	msg := tea.MouseMsg{X: 10, Y: 5, Button: tea.MouseButtonWheelUp}
	result, _ := m.Update(msg)
	m2 := result.(Model)

	if m2.activePanel != PanelLeft {
		t.Errorf("左侧滚轮后 activePanel 应为 PanelLeft，实际: %d", m2.activePanel)
	}
}

// ============================================================
// 单击测试
// ============================================================

// TestHandleMouseLeftClickSetsFile 测试左键单击设置文件光标
func TestHandleMouseLeftClickSetsFile(t *testing.T) {
	m := newMouseTestXftpModel()
	m.localPanel.cursor = 0

	// 点击第三个文件（index 2）
	msg := tea.MouseMsg{
		X: 5, Y: panelHeaderLines + 3,
		Button: tea.MouseButtonLeft, Action: tea.MouseActionPress,
	}
	result, _ := m.Update(msg)
	m2 := result.(Model)

	if m2.localPanel.cursor != 2 {
		t.Errorf("单击后光标应为 2，实际: %d", m2.localPanel.cursor)
	}
}

// TestHandleMouseLeftClickRightPanel 测试点击右面板切换活跃面板
func TestHandleMouseLeftClickRightPanel(t *testing.T) {
	m := newMouseTestXftpModel()
	m.activePanel = PanelLeft

	// 点击右面板区域的第二个文件（index 1）
	msg := tea.MouseMsg{
		X: 60, Y: panelHeaderLines + 2,
		Button: tea.MouseButtonLeft, Action: tea.MouseActionPress,
	}
	result, _ := m.Update(msg)
	m2 := result.(Model)

	if m2.activePanel != PanelRight {
		t.Errorf("点击右面板后 activePanel 应为 PanelRight，实际: %d", m2.activePanel)
	}
	if m2.remotePanel.cursor != 1 {
		t.Errorf("点击右面板后远程光标应为 1，实际: %d", m2.remotePanel.cursor)
	}
}

// TestHandleMouseDisconnectedRemote 测试右面板未连接时点击不处理
func TestHandleMouseDisconnectedRemote(t *testing.T) {
	m := newMouseTestXftpModel()
	m.connected = false
	m.remotePanel.cursor = 0

	msg := tea.MouseMsg{
		X: 60, Y: panelHeaderLines + 2,
		Button: tea.MouseButtonLeft, Action: tea.MouseActionPress,
	}
	result, _ := m.Update(msg)
	m2 := result.(Model)

	if m2.remotePanel.cursor != 0 {
		t.Errorf("右面板未连接时光标不应改变，实际: %d", m2.remotePanel.cursor)
	}
}

// TestHandleMouseMotionIgnored 测试鼠标移动事件被忽略
func TestHandleMouseMotionIgnored(t *testing.T) {
	m := newMouseTestXftpModel()
	m.localPanel.cursor = 0

	msg := tea.MouseMsg{
		X: 5, Y: panelHeaderLines + 3,
		Button: tea.MouseButtonLeft, Action: tea.MouseActionMotion,
	}
	result, _ := m.Update(msg)
	m2 := result.(Model)

	if m2.localPanel.cursor != 0 {
		t.Errorf("鼠标移动不应改变光标，实际: %d", m2.localPanel.cursor)
	}
}

// ============================================================
// F2: xftp 模态关闭
// ============================================================

// TestHandleMouseDismissHelp 测试左键点击关闭帮助模态
func TestHandleMouseDismissHelp(t *testing.T) {
	m := newMouseTestXftpModel()
	m.mode = ModeHelp

	msg := tea.MouseMsg{X: 10, Y: 10, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress}
	result, _ := m.Update(msg)
	m2 := result.(Model)

	if m2.mode != ModeNormal {
		t.Errorf("左键点击帮助模态后应返回 ModeNormal，实际: %d", m2.mode)
	}
}

// TestHandleMouseDismissError 测试左键点击关闭错误模态
func TestHandleMouseDismissError(t *testing.T) {
	m := newMouseTestXftpModel()
	m.mode = ModeError
	m.err = nil

	msg := tea.MouseMsg{X: 10, Y: 10, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress}
	result, _ := m.Update(msg)
	m2 := result.(Model)

	if m2.mode != ModeNormal {
		t.Errorf("左键点击错误模态后应返回 ModeNormal，实际: %d", m2.mode)
	}
}

// TestHandleMouseDismissTransferResult 测试左键点击关闭传输结果模态
func TestHandleMouseDismissTransferResult(t *testing.T) {
	m := newMouseTestXftpModel()
	m.mode = ModeTransferResult
	m.transferResult = &TransferResultMsg{Files: 1, TotalBytes: 100}

	msg := tea.MouseMsg{X: 10, Y: 10, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress}
	result, _ := m.Update(msg)
	m2 := result.(Model)

	if m2.mode != ModeNormal {
		t.Errorf("左键点击传输结果模态后应返回 ModeNormal，实际: %d", m2.mode)
	}
	if m2.transferResult != nil {
		t.Error("关闭传输结果后 transferResult 应为 nil")
	}
}

// TestHandleMouseIgnoredInNonNormalMode 测试非普通模式下鼠标被忽略
func TestHandleMouseIgnoredInNonNormalMode(t *testing.T) {
	modes := []struct {
		name string
		mode Mode
	}{
		{"搜索模式", ModeSearch},
		{"命令模式", ModeCommand},
		{"输入模式", ModeInput},
	}

	for _, tt := range modes {
		t.Run(tt.name, func(t *testing.T) {
			m := newMouseTestXftpModel()
			m.mode = tt.mode
			m.localPanel.cursor = 0

			msg := tea.MouseMsg{
				X: 5, Y: panelHeaderLines + 3,
				Button: tea.MouseButtonLeft, Action: tea.MouseActionPress,
			}
			result, _ := m.Update(msg)
			m2 := result.(Model)

			if m2.localPanel.cursor != 0 {
				t.Errorf("%s 下鼠标不应改变光标，实际: %d", tt.name, m2.localPanel.cursor)
			}
		})
	}
}

// ============================================================
// F3: 确认栏点击
// ============================================================

// confirmBarNoButtonX 计算确认栏中 No 按钮的 X 坐标（用于测试）
func confirmBarNoButtonX(msgText string) int {
	yesBtn := ConfirmYesBtnStyle.Render("Yes(y)")
	noStart := 1 + lipgloss.Width(msgText) + 2 + lipgloss.Width(yesBtn) + 2
	return noStart
}

// TestHandleMouseConfirmClickNo 测试确认栏 No 按钮点击取消
func TestHandleMouseConfirmClickNo(t *testing.T) {
	m := newMouseTestXftpModel()
	m.mode = ModeConfirm
	m.confirmFiles = []confirmEntry{{Name: "test.txt", Path: "/tmp/test.txt"}}

	// 计算 No 按钮的实际 X 位置
	msgText := fmt.Sprintf("确认删除 \"%s\"？", "test.txt")
	noX := confirmBarNoButtonX(msgText)

	barY := m.height - 1
	msg := tea.MouseMsg{
		X: noX, Y: barY,
		Button: tea.MouseButtonLeft, Action: tea.MouseActionPress,
	}
	result, _ := m.Update(msg)
	m2 := result.(Model)

	if m2.mode != ModeNormal {
		t.Errorf("点击取消后应返回 ModeNormal，实际: %d", m2.mode)
	}
	if m2.statusMsg != "已取消" {
		t.Errorf("取消后状态消息应为 '已取消'，实际: %s", m2.statusMsg)
	}
}

// TestHandleMouseOverwriteConfirmClickNo 测试覆盖确认栏 No 按钮点击取消
func TestHandleMouseOverwriteConfirmClickNo(t *testing.T) {
	m := newMouseTestXftpModel()
	m.mode = ModeOverwriteConfirm
	m.overwriteConflicts = []string{"file.txt"}

	// 计算 No 按钮的实际 X 位置
	msgText := fmt.Sprintf("目标已存在 %d 个同名文件/目录，是否覆盖？", 1)
	noX := confirmBarNoButtonX(msgText)

	barY := m.height - 1
	msg := tea.MouseMsg{
		X: noX, Y: barY,
		Button: tea.MouseButtonLeft, Action: tea.MouseActionPress,
	}
	result, _ := m.Update(msg)
	m2 := result.(Model)

	if m2.mode != ModeNormal {
		t.Errorf("覆盖取消后应返回 ModeNormal，实际: %d", m2.mode)
	}
	if m2.statusMsg != "已取消" {
		t.Errorf("取消后状态消息应为 '已取消'，实际: %s", m2.statusMsg)
	}
}

// TestHandleMouseConfirmClickOutsideBar 测试确认模式下点击确认栏外区域不操作
func TestHandleMouseConfirmClickOutsideBar(t *testing.T) {
	m := newMouseTestXftpModel()
	m.mode = ModeConfirm
	m.confirmFiles = []confirmEntry{{Name: "test.txt", Path: "/tmp/test.txt"}}

	// 点击远离确认栏的区域
	msg := tea.MouseMsg{
		X: 10, Y: 5,
		Button: tea.MouseButtonLeft, Action: tea.MouseActionPress,
	}
	result, _ := m.Update(msg)
	m2 := result.(Model)

	if m2.mode != ModeConfirm {
		t.Errorf("点击确认栏外区域不应改变模式，实际: %d", m2.mode)
	}
}

// ============================================================
// F4: 面板头部点击切换面板
// ============================================================

// TestHandleMousePanelHeaderSwitchesActivePanel 测试点击面板头部区域切换激活面板
func TestHandleMousePanelHeaderSwitchesActivePanel(t *testing.T) {
	m := newMouseTestXftpModel()
	m.activePanel = PanelLeft

	// 点击右面板头部区域（Y=1 < panelHeaderLines，X 在右面板区域）
	msg := tea.MouseMsg{
		X: 60, Y: 1,
		Button: tea.MouseButtonLeft, Action: tea.MouseActionPress,
	}
	result, _ := m.Update(msg)
	m2 := result.(Model)

	// 点击头部区域 fileY < 0，不会修改光标，但会切换面板
	if m2.activePanel != PanelRight {
		t.Errorf("点击右面板头部后 activePanel 应为 PanelRight，实际: %d", m2.activePanel)
	}
}

// ============================================================
// F5: Shift+Click 范围选择
// ============================================================

// TestShiftClickRangeSelect 测试 Shift+Click 范围选择
func TestShiftClickRangeSelect(t *testing.T) {
	m := newMouseTestXftpModel()

	// 先单击 index 1 设置选择锚点
	msg1 := tea.MouseMsg{
		X: 5, Y: panelHeaderLines + 2,
		Button: tea.MouseButtonLeft, Action: tea.MouseActionPress,
	}
	result, _ := m.Update(msg1)
	m = result.(Model)

	if m.selectionAnchor != 1 {
		t.Fatalf("单击后 selectionAnchor 应为 1，实际: %d", m.selectionAnchor)
	}

	// Shift+Click index 3
	msg2 := tea.MouseMsg{
		X: 5, Y: panelHeaderLines + 4,
		Shift:  true,
		Button: tea.MouseButtonLeft, Action: tea.MouseActionPress,
	}
	// 需要足够时间间隔避免触发双击
	m.lastClickTime = time.Now().Add(-1 * time.Second)
	result, _ = m.Update(msg2)
	m = result.(Model)

	// 索引 1, 2, 3 应被选中
	for i := 1; i <= 3; i++ {
		if !m.localPanel.entries[i].Selected {
			t.Errorf("Shift+Click 后索引 %d 应被选中", i)
		}
	}
	// 索引 0 不应被选中
	if m.localPanel.entries[0].Selected {
		t.Error("索引 0 不应被选中")
	}
}

// TestShiftClickRangeSelectReverse 测试反向 Shift+Click 范围选择
func TestShiftClickRangeSelectReverse(t *testing.T) {
	m := newMouseTestXftpModel()

	// 先单击 index 3 设置选择锚点
	msg1 := tea.MouseMsg{
		X: 5, Y: panelHeaderLines + 4,
		Button: tea.MouseButtonLeft, Action: tea.MouseActionPress,
	}
	result, _ := m.Update(msg1)
	m = result.(Model)

	// Shift+Click index 1（反向）
	msg2 := tea.MouseMsg{
		X: 5, Y: panelHeaderLines + 2,
		Shift:  true,
		Button: tea.MouseButtonLeft, Action: tea.MouseActionPress,
	}
	m.lastClickTime = time.Now().Add(-1 * time.Second)
	result, _ = m.Update(msg2)
	m = result.(Model)

	// 索引 1, 2, 3 应被选中
	for i := 1; i <= 3; i++ {
		if !m.localPanel.entries[i].Selected {
			t.Errorf("反向 Shift+Click 后索引 %d 应被选中", i)
		}
	}
}

// TestShiftClickSameIndex 测试同一位置 Shift+Click
func TestShiftClickSameIndex(t *testing.T) {
	m := newMouseTestXftpModel()

	// 先单击 index 2
	msg1 := tea.MouseMsg{
		X: 5, Y: panelHeaderLines + 3,
		Button: tea.MouseButtonLeft, Action: tea.MouseActionPress,
	}
	result, _ := m.Update(msg1)
	m = result.(Model)

	// Shift+Click 同一位置
	msg2 := tea.MouseMsg{
		X: 5, Y: panelHeaderLines + 3,
		Shift:  true,
		Button: tea.MouseButtonLeft, Action: tea.MouseActionPress,
	}
	m.lastClickTime = time.Now().Add(-1 * time.Second)
	result, _ = m.Update(msg2)
	m = result.(Model)

	// 仅索引 2 被选中
	if !m.localPanel.entries[2].Selected {
		t.Error("Shift+Click 同一位置后该文件应被选中")
	}
	if m.localPanel.entries[1].Selected {
		t.Error("索引 1 不应被选中")
	}
	if m.localPanel.entries[3].Selected {
		t.Error("索引 3 不应被选中")
	}
}

// ============================================================
// F7: xftp 右键上下文菜单
// ============================================================

// TestXftpContextMenuRightClick 测试右键点击文件弹出上下文菜单
func TestXftpContextMenuRightClick(t *testing.T) {
	m := newMouseTestXftpModel()

	// 右键点击第二个文件（index 1）
	msg := tea.MouseMsg{
		X: 5, Y: panelHeaderLines + 2,
		Button: tea.MouseButtonRight, Action: tea.MouseActionPress,
	}
	result, _ := m.Update(msg)
	m2 := result.(Model)

	if m2.mode != ModeContextMenu {
		t.Errorf("右键点击后应进入 ModeContextMenu，实际: %d", m2.mode)
	}
	if !m2.contextMenu.visible {
		t.Error("右键点击后上下文菜单应可见")
	}
	if len(m2.contextMenu.items) == 0 {
		t.Error("上下文菜单应有菜单项")
	}
}

// TestXftpContextMenuItems 测试上下文菜单包含正确的操作项
func TestXftpContextMenuItems(t *testing.T) {
	m := newMouseTestXftpModel()

	msg := tea.MouseMsg{
		X: 5, Y: panelHeaderLines + 2,
		Button: tea.MouseButtonRight, Action: tea.MouseActionPress,
	}
	result, _ := m.Update(msg)
	m2 := result.(Model)

	actions := make(map[string]bool)
	for _, item := range m2.contextMenu.items {
		actions[item.Action] = true
	}

	expected := []string{"yank", "delete", "mkdir", "rename"}
	for _, action := range expected {
		if !actions[action] {
			t.Errorf("上下文菜单应包含 %q 操作", action)
		}
	}
}

// TestXftpContextMenuDismissOnLeftClick 测试左键点击关闭上下文菜单
func TestXftpContextMenuDismissOnLeftClick(t *testing.T) {
	m := newMouseTestXftpModel()
	m.mode = ModeContextMenu
	m.contextMenu = ContextMenu{
		visible: true,
		items:   []ContextMenuItem{{Label: "复制", Key: "y", Action: "yank"}},
		cursor:  0,
	}

	msg := tea.MouseMsg{X: 10, Y: 10, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress}
	result, _ := m.Update(msg)
	m2 := result.(Model)

	if m2.mode != ModeNormal {
		t.Errorf("左键点击后应返回 ModeNormal，实际: %d", m2.mode)
	}
	if m2.contextMenu.visible {
		t.Error("左键点击后应关闭上下文菜单")
	}
}

// TestXftpContextMenuKeyNav 测试上下文菜单键盘导航
func TestXftpContextMenuKeyNav(t *testing.T) {
	m := newMouseTestXftpModel()
	m.mode = ModeContextMenu
	m.contextMenu = ContextMenu{
		visible: true,
		items: []ContextMenuItem{
			{Label: "复制", Key: "y", Action: "yank"},
			{Label: "删除", Key: "D", Action: "delete"},
			{Label: "重命名", Key: "r", Action: "rename"},
		},
		cursor: 0,
	}

	// j 下移
	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	m = result.(Model)
	if m.contextMenu.cursor != 1 {
		t.Errorf("j 后菜单光标应为 1，实际: %d", m.contextMenu.cursor)
	}

	// k 上移
	result, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
	m = result.(Model)
	if m.contextMenu.cursor != 0 {
		t.Errorf("k 后菜单光标应为 0，实际: %d", m.contextMenu.cursor)
	}

	// Esc 关闭
	result, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = result.(Model)
	if m.mode != ModeNormal {
		t.Errorf("Esc 后应返回 ModeNormal，实际: %d", m.mode)
	}
	if m.contextMenu.visible {
		t.Error("Esc 后应关闭上下文菜单")
	}
}

// TestXftpContextMenuRightClickReplace 测试右键点击不同位置替换菜单
func TestXftpContextMenuRightClickReplace(t *testing.T) {
	m := newMouseTestXftpModel()

	// 右键打开菜单
	msg1 := tea.MouseMsg{
		X: 5, Y: panelHeaderLines + 2,
		Button: tea.MouseButtonRight, Action: tea.MouseActionPress,
	}
	result, _ := m.Update(msg1)
	m = result.(Model)

	if m.localPanel.cursor != 1 {
		t.Fatalf("第一次右键后光标应为 1，实际: %d", m.localPanel.cursor)
	}

	// 右键点击另一个位置
	msg2 := tea.MouseMsg{
		X: 5, Y: panelHeaderLines + 4,
		Button: tea.MouseButtonRight, Action: tea.MouseActionPress,
	}
	result, _ = m.Update(msg2)
	m = result.(Model)

	if m.mode != ModeContextMenu {
		t.Errorf("替换右键后应在 ModeContextMenu，实际: %d", m.mode)
	}
	if m.localPanel.cursor != 3 {
		t.Errorf("替换右键后光标应为 3，实际: %d", m.localPanel.cursor)
	}
}

// TestXftpContextMenuDisconnectedRemote 测试右面板未连接时右键不弹出菜单
func TestXftpContextMenuDisconnectedRemote(t *testing.T) {
	m := newMouseTestXftpModel()
	m.connected = false

	msg := tea.MouseMsg{
		X: 60, Y: panelHeaderLines + 2,
		Button: tea.MouseButtonRight, Action: tea.MouseActionPress,
	}
	result, _ := m.Update(msg)
	m2 := result.(Model)

	if m2.mode == ModeContextMenu {
		t.Error("右面板未连接时不应弹出上下文菜单")
	}
}

// ============================================================
// 选择器鼠标测试
// ============================================================

// TestSelectorDismissHelp 测试选择器左键点击关闭帮助模态
func TestSelectorDismissHelp(t *testing.T) {
	s := newMouseTestSelector()
	s.showHelp = true

	msg := tea.MouseMsg{X: 10, Y: 10, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress}
	s2, _ := s.handleMouse(msg)

	if s2.showHelp {
		t.Error("左键点击后应关闭帮助模态")
	}
}

// TestSelectorDismissError 测试选择器左键点击关闭错误模态
func TestSelectorDismissError(t *testing.T) {
	s := newMouseTestSelector()
	s.showError = true
	s.errorMessage = "连接失败"

	msg := tea.MouseMsg{X: 10, Y: 10, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress}
	s2, _ := s.handleMouse(msg)

	if s2.showError {
		t.Error("左键点击后应关闭错误模态")
	}
	if s2.errorMessage != "" {
		t.Errorf("错误消息应被清空，实际: %s", s2.errorMessage)
	}
}

// TestSelectorMouseIgnoredInSearchMode 测试搜索模式下鼠标被忽略
func TestSelectorMouseIgnoredInSearchMode(t *testing.T) {
	s := newMouseTestSelector()
	s.searching = true
	s.cursor = 0

	msg := tea.MouseMsg{X: 5, Y: 2, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress}
	s2, _ := s.handleMouse(msg)

	if s2.cursor != 0 {
		t.Errorf("搜索模式下鼠标不应改变光标，实际: %d", s2.cursor)
	}
}

// TestSelectorMouseIgnoredInCommandMode 测试命令模式下鼠标被忽略
func TestSelectorMouseIgnoredInCommandMode(t *testing.T) {
	s := newMouseTestSelector()
	s.commanding = true
	s.cursor = 0

	msg := tea.MouseMsg{X: 5, Y: 2, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress}
	s2, _ := s.handleMouse(msg)

	if s2.cursor != 0 {
		t.Errorf("命令模式下鼠标不应改变光标，实际: %d", s2.cursor)
	}
}

// TestSelectorMouseWheelUp 测试选择器滚轮上滚
func TestSelectorMouseWheelUp(t *testing.T) {
	s := newMouseTestSelector()
	// flatNodes: dir1, server-1, server-2 → 3 个节点
	s.cursor = 2

	msg := tea.MouseMsg{Button: tea.MouseButtonWheelUp}
	s2, _ := s.handleMouse(msg)

	if s2.cursor >= 2 {
		t.Errorf("滚轮上滚后光标应小于 2，实际: %d", s2.cursor)
	}
}

// TestSelectorSingleClickSetsCursor 测试选择器单击设置光标
func TestSelectorSingleClickSetsCursor(t *testing.T) {
	s := newMouseTestSelector()
	s.cursor = 0

	// Y=1 在边框内第一个节点，Y=2 第二个节点
	msg := tea.MouseMsg{X: 5, Y: 2, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress}
	s2, _ := s.handleMouse(msg)

	// clickedIndex = offset(0) + (Y-1) = 0 + 1 = 1
	if s2.cursor != 1 {
		t.Errorf("单击 Y=2 后光标应为 1，实际: %d", s2.cursor)
	}
}

// TestSelectorDoubleClickDir 测试选择器双击目录切换展开
func TestSelectorDoubleClickDir(t *testing.T) {
	s := newMouseTestSelector()
	// flatNodes[0] = dir1, flatNodes[1] = server-1, flatNodes[2] = server-2
	// dir1 初始为 Expanded=true
	if !s.flatNodes[0].Expanded {
		t.Fatal("dir1 初始应为展开状态")
	}

	// 第一次点击 Y=1（dir1）
	msg := tea.MouseMsg{X: 5, Y: 1, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress}
	s, _ = s.handleMouse(msg)

	// 第二次点击（双击）
	s, _ = s.handleMouse(msg)

	// 双击目录应折叠
	if s.tree.Children[0].Expanded {
		t.Error("双击 dir1 后应变为折叠状态")
	}
}

// B3: 选择器边界测试

// TestSelectorClickBeyondTreeWidth 测试点击树区域右侧被忽略
func TestSelectorClickBeyondTreeWidth(t *testing.T) {
	s := newMouseTestSelector()
	s.cursor = 0

	// treeWidth = 120 * 70 / 100 = 84
	msg := tea.MouseMsg{X: 90, Y: 2, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress}
	s2, _ := s.handleMouse(msg)

	if s2.cursor != 0 {
		t.Errorf("点击树区域外不应改变光标，实际: %d", s2.cursor)
	}
}

// TestSelectorClickBeyondNodeCount 测试点击超出节点数量的位置
func TestSelectorClickBeyondNodeCount(t *testing.T) {
	s := newMouseTestSelector()
	s.cursor = 0
	// flatNodes 只有 3 个，点击 Y=10 对应 clickedIndex=9 超出
	msg := tea.MouseMsg{X: 5, Y: 10, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress}
	s2, _ := s.handleMouse(msg)

	if s2.cursor != 0 {
		t.Errorf("点击超出节点范围不应改变光标，实际: %d", s2.cursor)
	}
}

// TestSelectorClickAtBorder 测试点击边框区域被忽略
func TestSelectorClickAtBorder(t *testing.T) {
	s := newMouseTestSelector()
	s.cursor = 0

	// Y=0 是上边框，应被忽略
	msg := tea.MouseMsg{X: 5, Y: 0, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress}
	s2, _ := s.handleMouse(msg)

	if s2.cursor != 0 {
		t.Errorf("点击上边框不应改变光标，实际: %d", s2.cursor)
	}
}

func TestXftpTopMenuMouseSearchAction(t *testing.T) {
	m := newMouseTestXftpModel()

	result, _ := m.Update(tea.MouseMsg{
		X: 13, Y: 0,
		Button: tea.MouseButtonLeft, Action: tea.MouseActionPress,
	})
	m = result.(Model)
	if !m.menu.Open || m.menu.Active != 2 {
		t.Fatalf("点击 View 应打开第三个菜单: %+v", m.menu)
	}

	result, _ = m.Update(tea.MouseMsg{
		X: 13, Y: 2,
		Button: tea.MouseButtonLeft, Action: tea.MouseActionPress,
	})
	m = result.(Model)
	if m.mode != ModeSearch {
		t.Fatalf("点击 Search 应进入搜索模式，实际: %d", m.mode)
	}
}

func TestXftpTopMenuHoverSwitchesMenu(t *testing.T) {
	m := newMouseTestXftpModel()
	m.menu.OpenMenu(0, topMenus)

	result, _ := m.Update(tea.MouseMsg{
		X: 19, Y: 0,
		Action: tea.MouseActionMotion,
	})
	m = result.(Model)
	if m.menu.Active != 3 {
		t.Fatalf("悬停 Transfer 应切换第四个菜单，实际: %d", m.menu.Active)
	}
}

func TestXftpTopMenuF10KeyboardNavigation(t *testing.T) {
	m := newMouseTestXftpModel()
	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyF10})
	m = result.(Model)
	if !m.menu.Open {
		t.Fatal("F10 应打开菜单")
	}
	result, _ = m.Update(tea.KeyMsg{Type: tea.KeyRight})
	m = result.(Model)
	if m.menu.Active != 1 {
		t.Fatalf("右方向键应切换菜单，实际: %d", m.menu.Active)
	}
	result, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = result.(Model)
	if m.menu.Open {
		t.Fatal("Esc 应关闭菜单")
	}
}

func TestXftpSelectorTopMenuConsumesClick(t *testing.T) {
	m := NewModel(nil)
	m.width = 100
	m.height = 30

	result, _ := m.Update(tea.MouseMsg{
		X: 2, Y: 0,
		Button: tea.MouseButtonLeft, Action: tea.MouseActionPress,
	})
	m = result.(Model)
	if !m.menu.Open {
		t.Fatal("选择器模式下顶部菜单也应可点击")
	}
}
