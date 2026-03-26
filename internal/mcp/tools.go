package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/ketor/xsc/internal/session"
	"github.com/ketor/xsc/internal/shared"
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

// findSessionByPath 查找会话并解密密码（支持所有来源）
func findSessionByPath(sessionPath string) (*session.Session, error) {
	s, err := shared.FindSessionAllSources(sessionPath)
	if err != nil {
		return nil, err
	}
	if err := s.ResolvePassword(); err != nil {
		return nil, fmt.Errorf("密码解密失败: %w", err)
	}
	return s, nil
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

	entries := shared.LoadAllSessionsFlat()
	if entries == nil {
		return errorResult("加载会话失败"), nil
	}

	var results []SessionInfo
	for _, e := range entries {
		source := "local"
		if e.Session.PasswordSource != "" {
			source = e.Session.PasswordSource
		}
		info := SessionInfo{
			Path:     e.Path,
			Host:     e.Session.Host,
			Port:     e.Session.Port,
			User:     e.Session.User,
			AuthType: string(e.Session.AuthType),
			Source:   source,
			Desc:     e.Session.Description,
		}
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

	entries := shared.LoadAllSessionsFlat()

	var found *shared.SessionEntry
	// 精确匹配
	for i, e := range entries {
		if e.Path == args.SessionPath {
			found = &entries[i]
			break
		}
	}
	// 模糊匹配
	if found == nil {
		for i, e := range entries {
			if strings.Contains(e.Path, args.SessionPath) || args.SessionPath == filepath.Base(e.Path) {
				found = &entries[i]
				break
			}
		}
	}

	if found == nil {
		return errorResult(fmt.Sprintf("会话未找到 '%s'", args.SessionPath)), nil
	}

	s := found.Session
	source := "local"
	if s.PasswordSource != "" {
		source = s.PasswordSource
	}

	info := SessionInfo{
		Path:     found.Path,
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
