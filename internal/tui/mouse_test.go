package tui

import (
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/ketor/xsc/internal/session"
)

// newMouseTestModel 创建带有测试会话树的 Model，用于鼠标测试
// 可见节点（展开后）：dir1, server-1, server-2, dir2, server-local → 5 个节点
func newMouseTestModel() Model {
	m := initialModel()
	m.width = 120
	m.height = 40
	m.tree = &session.SessionNode{
		Name: "root", IsDir: true, Expanded: true,
		Children: []*session.SessionNode{
			{Name: "dir1", IsDir: true, Expanded: true, Children: []*session.SessionNode{
				{Name: "server-1", IsDir: false, Session: &session.Session{
					Host: "10.0.0.1", Port: 22, User: "root",
					AuthType: session.AuthTypePassword, Password: "pass", Valid: true,
				}},
				{Name: "server-2", IsDir: false, Session: &session.Session{
					Host: "10.0.0.2", Port: 22, User: "root",
					AuthType: session.AuthTypePassword, Password: "pass", Valid: true,
				}},
			}},
			{Name: "dir2", IsDir: true, Expanded: false, Children: []*session.SessionNode{
				{Name: "server-3", IsDir: false, Session: &session.Session{
					Host: "10.0.0.3", Port: 22, User: "root", Valid: true,
				}},
			}},
			{Name: "server-local", IsDir: false, Session: &session.Session{
				Host: "localhost", Port: 22, User: "root",
				AuthType: session.AuthTypePassword, Password: "pass", Valid: true,
			}},
		},
	}
	m.tree.SetParent(nil)
	return m
}

// --- B2: 负坐标边界检查 ---

// TestHandleMouseNegativeY 测试 Y 坐标为负值时不崩溃且光标不变
func TestHandleMouseNegativeY(t *testing.T) {
	m := newMouseTestModel()
	m.cursor = 0

	msg := tea.MouseMsg{X: 5, Y: -1, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress}
	result, _ := m.Update(msg)
	m2 := result.(Model)

	if m2.cursor != 0 {
		t.Errorf("Y=-1 时光标不应改变，实际: %d", m2.cursor)
	}
}

// TestHandleMouseNegativeX 测试 X 坐标为负值时不崩溃且光标不变
func TestHandleMouseNegativeX(t *testing.T) {
	m := newMouseTestModel()
	m.cursor = 0

	msg := tea.MouseMsg{X: -1, Y: 2, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress}
	result, _ := m.Update(msg)
	m2 := result.(Model)

	if m2.cursor != 0 {
		t.Errorf("X=-1 时光标不应改变，实际: %d", m2.cursor)
	}
}

// TestHandleMouseOutOfBoundsRight 测试点击在树区域右侧（详情面板）时光标不变
func TestHandleMouseOutOfBoundsRight(t *testing.T) {
	m := newMouseTestModel()
	m.cursor = 0

	// treeWidth = 120 * 70 / 100 = 84，点击 X=100 在详情面板区域
	msg := tea.MouseMsg{X: 100, Y: 2, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress}
	result, _ := m.Update(msg)
	m2 := result.(Model)

	if m2.cursor != 0 {
		t.Errorf("点击详情面板区域时光标不应改变，实际: %d", m2.cursor)
	}
}

// TestHandleMouseOutOfBoundsBottom 测试点击超出内容高度时光标不变
func TestHandleMouseOutOfBoundsBottom(t *testing.T) {
	m := newMouseTestModel()
	m.cursor = 0

	// contentHeight = 40 - 2 = 38，Y=39 超出
	msg := tea.MouseMsg{X: 5, Y: 39, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress}
	result, _ := m.Update(msg)
	m2 := result.(Model)

	if m2.cursor != 0 {
		t.Errorf("Y 超出内容高度时光标不应改变，实际: %d", m2.cursor)
	}
}

// --- 滚轮测试 ---

// TestHandleMouseWheelUp 测试滚轮上滚
func TestHandleMouseWheelUp(t *testing.T) {
	m := newMouseTestModel()
	m.cursor = 3

	msg := tea.MouseMsg{Button: tea.MouseButtonWheelUp}
	result, _ := m.Update(msg)
	m2 := result.(Model)

	if m2.cursor != 0 {
		t.Errorf("滚轮上滚 3 后光标应为 0，实际: %d", m2.cursor)
	}
}

// TestHandleMouseWheelDown 测试滚轮下滚
func TestHandleMouseWheelDown(t *testing.T) {
	m := newMouseTestModel()
	m.cursor = 0

	msg := tea.MouseMsg{Button: tea.MouseButtonWheelDown}
	result, _ := m.Update(msg)
	m2 := result.(Model)

	if m2.cursor != 3 {
		t.Errorf("滚轮下滚 3 后光标应为 3，实际: %d", m2.cursor)
	}
}

// --- 单击测试 ---

// TestHandleMouseLeftClickSetsCursor 测试左键单击设置光标
func TestHandleMouseLeftClickSetsCursor(t *testing.T) {
	m := newMouseTestModel()
	m.cursor = 0

	msg := tea.MouseMsg{X: 5, Y: 3, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress}
	result, _ := m.Update(msg)
	m2 := result.(Model)

	if m2.cursor != 2 {
		t.Errorf("单击内容行 2 后光标应为 2，实际: %d", m2.cursor)
	}
}

// TestHandleMouseReleaseIgnored 测试鼠标释放事件被忽略
func TestHandleMouseReleaseIgnored(t *testing.T) {
	m := newMouseTestModel()
	m.cursor = 0

	msg := tea.MouseMsg{X: 5, Y: 2, Button: tea.MouseButtonLeft, Action: tea.MouseActionRelease}
	result, _ := m.Update(msg)
	m2 := result.(Model)

	if m2.cursor != 0 {
		t.Errorf("鼠标释放事件不应改变光标，实际: %d", m2.cursor)
	}
}

// --- A3: 双击光标更新 ---

// TestHandleMouseDoubleClickUpdatesCursor 测试双击更新光标位置
func TestHandleMouseDoubleClickUpdatesCursor(t *testing.T) {
	m := newMouseTestModel()
	m.cursor = 0

	// 第一次点击内容行 2（server-2，是会话节点）
	msg := tea.MouseMsg{X: 5, Y: 3, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress}
	result, _ := m.Update(msg)
	m = result.(Model)

	if m.cursor != 2 {
		t.Errorf("第一次单击后光标应为 2，实际: %d", m.cursor)
	}

	// 第二次点击同一位置（双击）
	result, _ = m.Update(msg)
	m = result.(Model)

	// 双击应先更新光标到 clickedIndex=2
	if m.cursor != 2 {
		t.Errorf("双击后光标应为 2，实际: %d", m.cursor)
	}
}

// TestHandleMouseDoubleClickDirToggle 测试双击目录切换展开状态
func TestHandleMouseDoubleClickDirToggle(t *testing.T) {
	m := newMouseTestModel()
	m.cursor = 0

	// 找到 dir1 节点（index 0），初始为 Expanded=true
	visibleNodes := m.getVisibleNodes()
	if !visibleNodes[0].Expanded {
		t.Fatal("dir1 初始应为展开状态")
	}

	// 第一次点击内容行 0（dir1）
	msg := tea.MouseMsg{X: 5, Y: 1, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress}
	result, _ := m.Update(msg)
	m = result.(Model)

	// 第二次点击同一位置（双击）
	result, _ = m.Update(msg)
	m = result.(Model)

	// 双击目录应切换展开状态
	visibleNodes = m.getVisibleNodes()
	// dir1 现在是 index 0，但它可能已折叠
	if m.tree.Children[0].Expanded {
		t.Error("双击 dir1 后应变为折叠状态")
	}
}

// --- F1: 模态关闭 ---

// TestHandleMouseDismissHelp 测试左键点击关闭帮助模态
func TestHandleMouseDismissHelp(t *testing.T) {
	m := newMouseTestModel()
	m.showHelp = true

	msg := tea.MouseMsg{X: 10, Y: 10, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress}
	result, _ := m.Update(msg)
	m2 := result.(Model)

	if m2.showHelp {
		t.Error("左键点击后应关闭帮助模态")
	}
}

// TestHandleMouseDismissError 测试左键点击关闭错误模态
func TestHandleMouseDismissError(t *testing.T) {
	m := newMouseTestModel()
	m.showError = true
	m.errorMessage = "test error"

	msg := tea.MouseMsg{X: 10, Y: 10, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress}
	result, _ := m.Update(msg)
	m2 := result.(Model)

	if m2.showError {
		t.Error("左键点击后应关闭错误模态")
	}
	if m2.errorMessage != "" {
		t.Errorf("错误消息应被清空，实际: %s", m2.errorMessage)
	}
}

// TestHandleMouseModalSearchIgnored 测试搜索模式下鼠标被忽略
func TestHandleMouseModalSearchIgnored(t *testing.T) {
	m := newMouseTestModel()
	m.cursor = 0

	modes := []struct {
		name  string
		setup func(*Model)
	}{
		{"searchMode", func(m *Model) { m.searchMode = true }},
		{"lineNumMode", func(m *Model) { m.lineNumMode = true }},
		{"newSessionMode", func(m *Model) { m.newSessionMode = true }},
		{"renameMode", func(m *Model) { m.renameMode = true }},
		{"deleteConfirmMode", func(m *Model) { m.deleteConfirmMode = true }},
	}

	for _, mode := range modes {
		t.Run(mode.name, func(t *testing.T) {
			m := newMouseTestModel()
			m.cursor = 0
			mode.setup(&m)

			msg := tea.MouseMsg{X: 5, Y: 2, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress}
			result, _ := m.Update(msg)
			m2 := result.(Model)

			if m2.cursor != 0 {
				t.Errorf("%s 模式下鼠标点击不应改变光标，实际: %d", mode.name, m2.cursor)
			}
		})
	}
}

// --- F6: 右键上下文菜单 ---

// TestContextMenuRightClick 测试右键点击会话节点弹出上下文菜单
func TestContextMenuRightClick(t *testing.T) {
	m := newMouseTestModel()

	// 右键点击内容行 1（server-1，非目录会话节点）
	msg := tea.MouseMsg{X: 5, Y: 2, Button: tea.MouseButtonRight, Action: tea.MouseActionPress}
	result, _ := m.Update(msg)
	m2 := result.(Model)

	if !m2.contextMenu.visible {
		t.Error("右键点击会话节点应显示上下文菜单")
	}
	if len(m2.contextMenu.items) == 0 {
		t.Error("上下文菜单应有菜单项")
	}
	if m2.cursor != 1 {
		t.Errorf("右键点击后光标应移动到 1，实际: %d", m2.cursor)
	}
}

// TestContextMenuRightClickDir 测试右键点击目录显示目录操作。
func TestContextMenuRightClickDir(t *testing.T) {
	m := newMouseTestModel()

	msg := tea.MouseMsg{X: 5, Y: 1, Button: tea.MouseButtonRight, Action: tea.MouseActionPress}
	result, _ := m.Update(msg)
	m2 := result.(Model)

	if !m2.contextMenu.visible {
		t.Fatal("右键点击目录应显示上下文菜单")
	}
	if m2.contextMenu.items[0].Action != "toggle-folder" {
		t.Errorf("目录菜单首项应切换折叠，实际: %s", m2.contextMenu.items[0].Action)
	}
}

// TestContextMenuDismissOnLeftClick 测试左键点击关闭上下文菜单
func TestContextMenuDismissOnLeftClick(t *testing.T) {
	m := newMouseTestModel()
	m.contextMenu = ContextMenu{
		visible: true,
		items: []ContextMenuItem{
			{Label: "连接", Key: "Enter", Action: "connect"},
		},
		cursor: 0,
	}

	msg := tea.MouseMsg{X: 10, Y: 10, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress}
	result, _ := m.Update(msg)
	m2 := result.(Model)

	if m2.contextMenu.visible {
		t.Error("左键点击后应关闭上下文菜单")
	}
}

// TestContextMenuReplaceOnRightClick 测试右键点击不同节点替换菜单
func TestContextMenuReplaceOnRightClick(t *testing.T) {
	m := newMouseTestModel()

	// 先右键点击内容行 1（server-1）
	msg1 := tea.MouseMsg{X: 5, Y: 2, Button: tea.MouseButtonRight, Action: tea.MouseActionPress}
	result, _ := m.Update(msg1)
	m = result.(Model)

	if !m.contextMenu.visible {
		t.Fatal("第一次右键后应显示菜单")
	}
	firstNode := m.contextMenu.node

	// 再右键点击内容行 2（server-2）
	msg2 := tea.MouseMsg{X: 5, Y: 3, Button: tea.MouseButtonRight, Action: tea.MouseActionPress}
	result, _ = m.Update(msg2)
	m = result.(Model)

	if !m.contextMenu.visible {
		t.Error("第二次右键后应显示菜单")
	}
	if m.contextMenu.node == firstNode {
		t.Error("菜单节点应更新为新点击的节点")
	}
}

// TestContextMenuRightClickOutsideTree 测试右键点击树区域外不弹出菜单
func TestContextMenuRightClickOutsideTree(t *testing.T) {
	m := newMouseTestModel()

	// 右键点击在详情面板区域（X=100 >= treeWidth=84）
	msg := tea.MouseMsg{X: 100, Y: 2, Button: tea.MouseButtonRight, Action: tea.MouseActionPress}
	result, _ := m.Update(msg)
	m2 := result.(Model)

	if m2.contextMenu.visible {
		t.Error("右键点击树区域外不应弹出菜单")
	}
}

// TestContextMenuItems 测试上下文菜单包含正确的操作项
func TestContextMenuItems(t *testing.T) {
	m := newMouseTestModel()

	// 右键点击 server-1（有效的非只读会话）
	msg := tea.MouseMsg{X: 5, Y: 2, Button: tea.MouseButtonRight, Action: tea.MouseActionPress}
	result, _ := m.Update(msg)
	m2 := result.(Model)

	if !m2.contextMenu.visible {
		t.Fatal("应显示上下文菜单")
	}

	actions := make(map[string]bool)
	for _, item := range m2.contextMenu.items {
		actions[item.Action] = true
	}

	expected := []string{"connect", "edit", "rename", "delete"}
	for _, action := range expected {
		if !actions[action] {
			t.Errorf("上下文菜单应包含 %q 操作", action)
		}
	}
}

// TestContextMenuKeyNav 测试上下文菜单键盘导航
func TestContextMenuKeyNav(t *testing.T) {
	m := newMouseTestModel()
	m.contextMenu = ContextMenu{
		visible: true,
		items: []ContextMenuItem{
			{Label: "连接", Key: "Enter", Action: "connect"},
			{Label: "编辑", Key: "e", Action: "edit"},
			{Label: "删除", Key: "D", Action: "delete"},
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
	if m.contextMenu.visible {
		t.Error("Esc 后应关闭上下文菜单")
	}
}

// TestContextMenuRightClickRelease 测试右键释放事件被忽略
func TestContextMenuRightClickRelease(t *testing.T) {
	m := newMouseTestModel()

	msg := tea.MouseMsg{X: 5, Y: 1, Button: tea.MouseButtonRight, Action: tea.MouseActionRelease}
	result, _ := m.Update(msg)
	m2 := result.(Model)

	if m2.contextMenu.visible {
		t.Error("右键释放不应弹出菜单")
	}
}

// --- 辅助测试 ---

// TestHandleMouseClickBeyondNodes 测试点击超出节点数量的位置
func TestHandleMouseClickBeyondNodes(t *testing.T) {
	m := newMouseTestModel()
	m.cursor = 0

	// 可见节点只有 5 个（Y=0..4），点击 Y=10 应超出
	msg := tea.MouseMsg{X: 5, Y: 10, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress}
	result, _ := m.Update(msg)
	m2 := result.(Model)

	if m2.cursor != 0 {
		t.Errorf("点击超出节点范围时光标不应改变，实际: %d", m2.cursor)
	}
}

// TestHandleMouseWheelUpBoundary 测试滚轮上滚不超出上界
func TestHandleMouseWheelUpBoundary(t *testing.T) {
	m := newMouseTestModel()
	m.cursor = 1

	msg := tea.MouseMsg{Button: tea.MouseButtonWheelUp}
	result, _ := m.Update(msg)
	m2 := result.(Model)

	if m2.cursor < 0 {
		t.Errorf("滚轮上滚不应使光标为负，实际: %d", m2.cursor)
	}
}

// TestHandleMouseDoubleClickTimingReset 测试双击间隔超时后不触发双击
func TestHandleMouseDoubleClickTimingReset(t *testing.T) {
	m := newMouseTestModel()

	// 模拟一次点击，但设置 lastClickTime 为很久之前
	m.lastClickTime = time.Now().Add(-1 * time.Second)
	m.lastClickIndex = 0

	// 在同一位置再次点击，间隔超过 400ms 应视为单击
	msg := tea.MouseMsg{X: 5, Y: 0, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress}
	result, _ := m.Update(msg)
	m2 := result.(Model)

	// 应视为单击，光标设置为 0，dir1 不应折叠
	if m2.cursor != 0 {
		t.Errorf("超时后应视为单击，光标应为 0，实际: %d", m2.cursor)
	}
	if !m2.tree.Children[0].Expanded {
		t.Error("超时后不应触发双击折叠")
	}
}

func TestTopMenuMouseSearchAction(t *testing.T) {
	m := newMouseTestModel()

	result, _ := m.Update(tea.MouseMsg{
		X: 16, Y: 0,
		Button: tea.MouseButtonLeft, Action: tea.MouseActionPress,
	})
	m = result.(Model)
	if !m.menu.Open || m.menu.Active != 2 {
		t.Fatalf("点击 View 应打开第三个菜单: %+v", m.menu)
	}

	result, _ = m.Update(tea.MouseMsg{
		X: 16, Y: 1,
		Button: tea.MouseButtonLeft, Action: tea.MouseActionPress,
	})
	m = result.(Model)
	if !m.searchMode {
		t.Fatal("点击 Search 菜单项应进入搜索模式")
	}
	if m.menu.Open {
		t.Fatal("执行菜单项后应关闭菜单")
	}
}

func TestTopMenuHoverSwitchesMenu(t *testing.T) {
	m := newMouseTestModel()
	m.menu.OpenMenu(0, topMenus)

	result, _ := m.Update(tea.MouseMsg{
		X: 8, Y: 0,
		Action: tea.MouseActionMotion,
	})
	m = result.(Model)
	if m.menu.Active != 1 {
		t.Fatalf("悬停 Session 应切换活动菜单，实际: %d", m.menu.Active)
	}
}

func TestTopMenuF10KeyboardNavigation(t *testing.T) {
	m := newMouseTestModel()
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
