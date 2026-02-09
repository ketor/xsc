package session

import (
	"testing"
)

func TestSessionNodeIsSecureCRT(t *testing.T) {
	// 创建测试树结构
	// root
	//   ├── local-session
	//   └── securecrt/
	//       └── sc-session

	root := &SessionNode{
		Name:     "sessions",
		IsDir:    true,
		Children: make([]*SessionNode, 0),
	}

	localSession := &SessionNode{
		Name:    "local-session",
		IsDir:   false,
		Session: &Session{},
	}
	root.Children = append(root.Children, localSession)

	securecrtDir := &SessionNode{
		Name:     "securecrt",
		IsDir:    true,
		Children: make([]*SessionNode, 0),
	}
	root.Children = append(root.Children, securecrtDir)

	scSession := &SessionNode{
		Name:    "sc-session",
		IsDir:   false,
		Session: &Session{},
	}
	securecrtDir.Children = append(securecrtDir.Children, scSession)

	// 设置父节点关系
	root.SetParent(nil)

	// 测试本地会话
	if localSession.IsSecureCRT() {
		t.Error("Local session should not be detected as SecureCRT")
	}

	// 测试 SecureCRT 目录
	if !securecrtDir.IsSecureCRT() {
		t.Error("SecureCRT directory should be detected as SecureCRT")
	}

	// 测试 SecureCRT 会话
	if !scSession.IsSecureCRT() {
		t.Error("SecureCRT session should be detected as SecureCRT")
	}

	// 测试根节点
	if root.IsSecureCRT() {
		t.Error("Root node should not be detected as SecureCRT")
	}
}

func TestSessionNodeIsSecureCRTNested(t *testing.T) {
	// 测试嵌套结构
	// root
	//   └── securecrt/
	//       └── folder/
	//           └── nested-session

	root := &SessionNode{
		Name:     "sessions",
		IsDir:    true,
		Children: make([]*SessionNode, 0),
	}

	securecrtDir := &SessionNode{
		Name:     "securecrt",
		IsDir:    true,
		Children: make([]*SessionNode, 0),
	}
	root.Children = append(root.Children, securecrtDir)

	folder := &SessionNode{
		Name:     "folder",
		IsDir:    true,
		Children: make([]*SessionNode, 0),
	}
	securecrtDir.Children = append(securecrtDir.Children, folder)

	nestedSession := &SessionNode{
		Name:    "nested-session",
		IsDir:   false,
		Session: &Session{},
	}
	folder.Children = append(folder.Children, nestedSession)

	// 设置父节点关系
	root.SetParent(nil)

	// 测试嵌套在 SecureCRT 下的会话
	if !nestedSession.IsSecureCRT() {
		t.Error("Nested session under SecureCRT should be detected as SecureCRT")
	}

	// 测试中间层文件夹
	if !folder.IsSecureCRT() {
		t.Error("Folder under SecureCRT should be detected as SecureCRT")
	}
}
