package session

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/user/xsc/internal/securecrt"
	"github.com/user/xsc/pkg/config"
)

// LoadAllSessions 递归加载目录中的所有会话文件
func LoadAllSessions(rootDir string) ([]*Session, error) {
	var sessions []*Session

	err := filepath.Walk(rootDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // 跳过无法访问的文件
		}

		if info.IsDir() {
			return nil
		}

		// 只处理 .yaml 和 .yml 文件
		ext := strings.ToLower(filepath.Ext(path))
		if ext != ".yaml" && ext != ".yml" {
			return nil
		}

		session, err := LoadSession(path)
		if err != nil {
			return nil // 继续加载其他会话
		}

		sessions = append(sessions, session)
		return nil
	})

	if err != nil {
		return nil, err
	}

	return sessions, nil
}

// LoadSessionsTree 以树形结构加载会话
func LoadSessionsTree(rootDir string) (*SessionNode, error) {
	root := &SessionNode{
		Name:     "sessions",
		IsDir:    true,
		Children: make([]*SessionNode, 0),
	}

	err := filepath.Walk(rootDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}

		relPath, err := filepath.Rel(rootDir, path)
		if err != nil {
			return nil
		}

		if relPath == "." {
			return nil
		}

		parts := strings.Split(relPath, string(filepath.Separator))
		current := root

		// 遍历路径部分，构建树结构
		for i, part := range parts {
			if i == len(parts)-1 && !info.IsDir() {
				// 处理文件
				ext := strings.ToLower(filepath.Ext(part))
				if ext != ".yaml" && ext != ".yml" {
					return nil
				}

				session, err := LoadSession(path)
				if err != nil {
					return nil
				}

				node := &SessionNode{
					Name:    strings.TrimSuffix(part, ext),
					IsDir:   false,
					Session: session,
				}
				current.Children = append(current.Children, node)
			} else {
				// 处理目录
				var found *SessionNode
				for _, child := range current.Children {
					if child.IsDir && child.Name == part {
						found = child
						break
					}
				}

				if found == nil {
					found = &SessionNode{
						Name:     part,
						IsDir:    true,
						Children: make([]*SessionNode, 0),
					}
					current.Children = append(current.Children, found)
				}
				current = found
			}
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	return root, nil
}

// SessionNode 表示会话树中的节点
type SessionNode struct {
	Name     string
	IsDir    bool
	Expanded bool
	Session  *Session
	Children []*SessionNode
	Parent   *SessionNode
}

// IsLeaf 检查节点是否为叶子节点（会话文件）
func (n *SessionNode) IsLeaf() bool {
	return !n.IsDir
}

// IsSecureCRT 检查节点或其祖先是否为 SecureCRT 会话
func (n *SessionNode) IsSecureCRT() bool {
	current := n
	for current != nil {
		if current.Name == "securecrt" {
			return true
		}
		current = current.Parent
	}
	return false
}

// GetPath 返回从根节点到当前节点的路径
func (n *SessionNode) GetPath() string {
	if n.Parent == nil {
		return n.Name
	}
	return filepath.Join(n.Parent.GetPath(), n.Name)
}

// FlattenVisible 返回可见的节点列表（考虑展开/折叠状态）
func (n *SessionNode) FlattenVisible() []*SessionNode {
	var result []*SessionNode

	// 遍历所有子节点，无论根节点还是非根节点逻辑相同
	for _, child := range n.Children {
		result = append(result, child)
		if child.IsDir && child.Expanded {
			result = append(result, child.FlattenVisible()...)
		}
	}

	return result
}

// FindNode 根据路径查找节点
func (n *SessionNode) FindNode(path string) *SessionNode {
	if n.GetPath() == path {
		return n
	}

	for _, child := range n.Children {
		if found := child.FindNode(path); found != nil {
			return found
		}
	}

	return nil
}

// SetParent 递归设置父节点引用
func (n *SessionNode) SetParent(parent *SessionNode) {
	n.Parent = parent
	for _, child := range n.Children {
		child.SetParent(n)
	}
}

// GetSessionPath 返回会话的相对路径（用于显示）
func GetSessionPath(rootDir string, session *Session) string {
	relPath, _ := filepath.Rel(rootDir, session.FilePath)
	return strings.TrimSuffix(relPath, ".yaml")
}

// LoadSecureCRTSessions 加载 SecureCRT 会话
func LoadSecureCRTSessions(cfg config.SecureCRTConfig) (*SessionNode, error) {
	if !cfg.Enabled {
		return nil, nil
	}

	scConfig := securecrt.Config{
		SessionPath: cfg.SessionPath,
		Password:    cfg.Password,
	}

	sessions, err := securecrt.LoadSessions(scConfig)
	if err != nil {
		return nil, err
	}

	root := &SessionNode{
		Name:     "securecrt",
		IsDir:    true,
		Expanded: true,
		Children: make([]*SessionNode, 0),
	}

	for _, scSession := range sessions {
		sessionData := scSession.ConvertToXSCSession()
		session := &Session{
			Host:     sessionData["host"].(string),
			Port:     sessionData["port"].(int),
			User:     sessionData["user"].(string),
			AuthType: AuthType(sessionData["auth_type"].(string)),
			Valid:    true,
		}

		if pwd, ok := sessionData["password"].(string); ok && pwd != "" {
			session.Password = pwd
		}

		// 保存加密密码和主密码，用于延迟解密
		if ep, ok := sessionData["encrypted_password"].(string); ok && ep != "" {
			session.EncryptedPassword = ep
			session.MasterPassword = cfg.Password
		}

		node := &SessionNode{
			Name:    scSession.Name,
			IsDir:   false,
			Session: session,
		}

		if scSession.Folder != "" {
			folderPath := strings.Split(scSession.Folder, string(filepath.Separator))
			current := root

			for _, folderName := range folderPath {
				var found *SessionNode
				for _, child := range current.Children {
					if child.IsDir && child.Name == folderName {
						found = child
						break
					}
				}

				if found == nil {
					found = &SessionNode{
						Name:     folderName,
						IsDir:    true,
						Children: make([]*SessionNode, 0),
					}
					current.Children = append(current.Children, found)
				}
				current = found
			}

			current.Children = append(current.Children, node)
		} else {
			root.Children = append(root.Children, node)
		}
	}

	return root, nil
}
