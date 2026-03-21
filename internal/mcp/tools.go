package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/ketor/xsc/internal/session"
	"github.com/ketor/xsc/internal/shared"
	"github.com/ketor/xsc/pkg/config"
	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

// RegisterTools 注册所有 MCP 工具到 Server
func RegisterTools(server *mcpsdk.Server) {
	// 会话管理
	server.AddTool(listSessionsTool(), handleListSessions)
	server.AddTool(getSessionTool(), handleGetSession)

	// 远程命令执行
	server.AddTool(sshExecTool(), handleSSHExec)

	// SFTP 文件操作
	server.AddTool(sftpListTool(), handleSFTPList)
	server.AddTool(sftpReadTool(), handleSFTPRead)
	server.AddTool(sftpWriteTool(), handleSFTPWrite)
	server.AddTool(sftpUploadTool(), handleSFTPUpload)
	server.AddTool(sftpDownloadTool(), handleSFTPDownload)
}

// SessionInfo 会话信息（不含密码）
type SessionInfo struct {
	Path     string `json:"path"`
	Host     string `json:"host"`
	Port     int    `json:"port"`
	User     string `json:"user"`
	AuthType string `json:"auth_type"`
	Source   string `json:"source,omitempty"` // 会话来源: local, securecrt, xshell, mobaxterm
	Desc     string `json:"description,omitempty"`
}

// collectSessions 从会话树中递归收集所有叶子节点（会话）
func collectSessions(node *session.SessionNode, prefix string) []SessionInfo {
	var results []SessionInfo
	for _, child := range node.Children {
		childPath := child.Name
		if prefix != "" {
			childPath = prefix + "/" + child.Name
		}

		if child.IsDir {
			results = append(results, collectSessions(child, childPath)...)
		} else if child.Session != nil {
			source := "local"
			if child.Session.PasswordSource != "" {
				source = child.Session.PasswordSource
			}
			results = append(results, SessionInfo{
				Path:     childPath,
				Host:     child.Session.Host,
				Port:     child.Session.Port,
				User:     child.Session.User,
				AuthType: string(child.Session.AuthType),
				Source:   source,
				Desc:     child.Session.Description,
			})
		}
	}
	return results
}

// findSessionInTree 在会话树中按路径查找会话（支持模糊匹配）
// 先精确匹配，再模糊匹配（路径包含或名称匹配）
func findSessionInTree(tree *session.SessionNode, path string) *session.Session {
	allSessions := collectSessionNodes(tree, "")

	// 精确匹配
	for _, entry := range allSessions {
		if entry.path == path {
			return entry.session
		}
	}

	// 模糊匹配：路径包含 或 名称匹配
	for _, entry := range allSessions {
		if strings.Contains(entry.path, path) || path == filepath.Base(entry.path) {
			return entry.session
		}
	}

	return nil
}

type sessionEntry struct {
	path    string
	session *session.Session
}

// collectSessionNodes 递归收集所有会话叶子节点及其路径
func collectSessionNodes(node *session.SessionNode, prefix string) []sessionEntry {
	var results []sessionEntry
	for _, child := range node.Children {
		childPath := child.Name
		if prefix != "" {
			childPath = prefix + "/" + child.Name
		}

		if child.IsDir {
			results = append(results, collectSessionNodes(child, childPath)...)
		} else if child.Session != nil {
			results = append(results, sessionEntry{
				path:    childPath,
				session: child.Session,
			})
		}
	}
	return results
}

// findSessionByPath 查找会话并解密密码
// 先在本地 YAML 会话中查找，再在 SecureCRT/XShell/MobaXterm 中查找
func findSessionByPath(sessionPath string) (*session.Session, error) {
	// 先尝试本地 YAML 会话（快速路径）
	sessionsDir, err := config.GetSessionsDir()
	if err == nil {
		if s, err := session.FindSession(sessionsDir, sessionPath); err == nil {
			resolveSessionPassword(s)
			return s, nil
		}
	}

	// 在完整会话树中查找（包括 SecureCRT/XShell/MobaXterm）
	tree, _ := shared.LoadSessionTree()
	if tree != nil {
		if s := findSessionInTree(tree, sessionPath); s != nil {
			resolveSessionPassword(s)
			return s, nil
		}
	}

	return nil, fmt.Errorf("会话未找到 '%s'", sessionPath)
}

// resolveSessionPassword 解密会话密码（如需要）
func resolveSessionPassword(s *session.Session) {
	if s.Password == "" && s.EncryptedPassword != "" {
		_ = s.ResolvePassword()
	}
	// 处理多认证方法中的加密密码
	for i, am := range s.AuthMethods {
		if am.Password == "" && am.EncryptedPassword != "" {
			// ResolvePassword 会在 ssh.Dial 中处理多认证方法的解密
			// 这里预设 MasterPassword 确保 Dial 时能解密
			_ = s.AuthMethods[i].EncryptedPassword // 保留，由 Dial 处理
		}
	}
}

// --- list_sessions ---

func listSessionsTool() *mcpsdk.Tool {
	return &mcpsdk.Tool{
		Name:        "list_sessions",
		Description: "列出所有已配置的 SSH 会话，包括本地会话和从 SecureCRT、XShell、MobaXterm 导入的会话。返回会话列表，包含路径、主机、端口、用户、认证类型、来源等信息。可通过 filter 参数按关键词过滤。",
		InputSchema: json.RawMessage(`{
			"type": "object",
			"properties": {
				"filter": {
					"type": "string",
					"description": "可选的过滤关键词，匹配会话路径、主机名或用户名"
				}
			}
		}`),
	}
}

func handleListSessions(_ context.Context, req *mcpsdk.CallToolRequest) (*mcpsdk.CallToolResult, error) {
	var args struct {
		Filter string `json:"filter"`
	}
	if len(req.Params.Arguments) > 0 {
		if err := json.Unmarshal(req.Params.Arguments, &args); err != nil {
			return nil, fmt.Errorf("参数解析失败: %w", err)
		}
	}

	// 使用 shared.LoadSessionTree() 加载所有来源的会话
	tree, _ := shared.LoadSessionTree()
	if tree == nil {
		return errorResult("加载会话树失败"), nil
	}

	allSessions := collectSessions(tree, "")

	var results []SessionInfo
	for _, info := range allSessions {
		if args.Filter == "" || matchesFilter(info, args.Filter) {
			results = append(results, info)
		}
	}

	data, _ := json.MarshalIndent(results, "", "  ")
	return &mcpsdk.CallToolResult{
		Content: []mcpsdk.Content{&mcpsdk.TextContent{Text: string(data)}},
	}, nil
}

// --- get_session ---

func getSessionTool() *mcpsdk.Tool {
	return &mcpsdk.Tool{
		Name:        "get_session",
		Description: "获取指定 SSH 会话的详细配置信息（不含密码）。支持本地会话和 SecureCRT/XShell/MobaXterm 会话。支持模糊路径匹配。",
		InputSchema: json.RawMessage(`{
			"type": "object",
			"properties": {
				"session_path": {
					"type": "string",
					"description": "会话路径，如 'prod/web/server-01' 或 'securecrt/folder/session'。支持模糊匹配。"
				}
			},
			"required": ["session_path"]
		}`),
	}
}

func handleGetSession(_ context.Context, req *mcpsdk.CallToolRequest) (*mcpsdk.CallToolResult, error) {
	var args struct {
		SessionPath string `json:"session_path"`
	}
	if err := json.Unmarshal(req.Params.Arguments, &args); err != nil {
		return nil, fmt.Errorf("参数解析失败: %w", err)
	}

	// 在完整会话树中查找
	tree, _ := shared.LoadSessionTree()
	if tree == nil {
		return errorResult("加载会话树失败"), nil
	}

	allSessions := collectSessionNodes(tree, "")
	var found *sessionEntry
	// 精确匹配
	for i, entry := range allSessions {
		if entry.path == args.SessionPath {
			found = &allSessions[i]
			break
		}
	}
	// 模糊匹配
	if found == nil {
		for i, entry := range allSessions {
			if strings.Contains(entry.path, args.SessionPath) || args.SessionPath == filepath.Base(entry.path) {
				found = &allSessions[i]
				break
			}
		}
	}

	if found == nil {
		return errorResult(fmt.Sprintf("会话未找到 '%s'", args.SessionPath)), nil
	}

	s := found.session
	source := "local"
	if s.PasswordSource != "" {
		source = s.PasswordSource
	}

	info := SessionInfo{
		Path:     found.path,
		Host:     s.Host,
		Port:     s.Port,
		User:     s.User,
		AuthType: string(s.AuthType),
		Source:   source,
		Desc:     s.Description,
	}

	data, _ := json.MarshalIndent(info, "", "  ")
	return &mcpsdk.CallToolResult{
		Content: []mcpsdk.Content{&mcpsdk.TextContent{Text: string(data)}},
	}, nil
}

// --- 辅助函数 ---

// matchesFilter 检查会话信息是否匹配过滤关键词
func matchesFilter(info SessionInfo, filter string) bool {
	filter = strings.ToLower(filter)
	return strings.Contains(strings.ToLower(info.Path), filter) ||
		strings.Contains(strings.ToLower(info.Host), filter) ||
		strings.Contains(strings.ToLower(info.User), filter) ||
		strings.Contains(strings.ToLower(info.Source), filter) ||
		strings.Contains(strings.ToLower(info.Desc), filter)
}

// errorResult 创建一个包含错误信息的工具结果
func errorResult(msg string) *mcpsdk.CallToolResult {
	return &mcpsdk.CallToolResult{
		Content: []mcpsdk.Content{&mcpsdk.TextContent{Text: msg}},
		IsError: true,
	}
}
