package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"time"

	internalssh "github.com/ketor/xsc/internal/ssh"
	"github.com/pkg/sftp"
	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

// connectSFTP 建立 SFTP 连接
// 返回 sftpClient 和清理函数（调用方必须 defer cleanup()）
func connectSFTP(sessionPath string) (*sftp.Client, func(), error) {
	s, err := findSessionByPath(sessionPath)
	if err != nil {
		return nil, nil, err
	}

	sshClient, sshCleanup, err := internalssh.Dial(s)
	if err != nil {
		return nil, nil, fmt.Errorf("SSH 连接失败: %w", err)
	}

	sftpClient, err := sftp.NewClient(sshClient)
	if err != nil {
		sshClient.Close()
		if sshCleanup != nil {
			sshCleanup()
		}
		return nil, nil, fmt.Errorf("SFTP 连接失败: %w", err)
	}

	cleanup := func() {
		sftpClient.Close()
		sshClient.Close()
		if sshCleanup != nil {
			sshCleanup()
		}
	}
	return sftpClient, cleanup, nil
}

// --- sftp_list ---

// FileEntry 文件条目信息
type FileEntry struct {
	Name    string `json:"name"`
	Size    int64  `json:"size"`
	Mode    string `json:"mode"`
	ModTime string `json:"mod_time"`
	IsDir   bool   `json:"is_dir"`
}

func sftpListTool() *mcpsdk.Tool {
	return &mcpsdk.Tool{
		Name:        "sftp_list",
		Description: "列出远程服务器上指定目录的文件和子目录，包含大小、权限、修改时间。",
		InputSchema: json.RawMessage(`{
			"type": "object",
			"properties": {
				"session_path": {
					"type": "string",
					"description": "会话路径"
				},
				"remote_path": {
					"type": "string",
					"description": "远程目录路径，默认 home 目录",
					"default": "."
				}
			},
			"required": ["session_path"]
		}`),
	}
}

func handleSFTPList(_ context.Context, req *mcpsdk.CallToolRequest) (*mcpsdk.CallToolResult, error) {
	var args struct {
		SessionPath string `json:"session_path"`
		RemotePath  string `json:"remote_path"`
	}
	if err := json.Unmarshal(req.Params.Arguments, &args); err != nil {
		return nil, fmt.Errorf("参数解析失败: %w", err)
	}

	sftpClient, cleanup, err := connectSFTP(args.SessionPath)
	if err != nil {
		return errorResult(err.Error()), nil
	}
	defer cleanup()

	remotePath := args.RemotePath
	if remotePath == "" || remotePath == "." {
		remotePath, _ = sftpClient.Getwd()
	}

	entries, err := sftpClient.ReadDir(remotePath)
	if err != nil {
		return errorResult(fmt.Sprintf("读取目录失败 '%s': %v", remotePath, err)), nil
	}

	var results []FileEntry
	for _, entry := range entries {
		results = append(results, FileEntry{
			Name:    entry.Name(),
			Size:    entry.Size(),
			Mode:    entry.Mode().String(),
			ModTime: entry.ModTime().Format(time.RFC3339),
			IsDir:   entry.IsDir(),
		})
	}

	data, _ := json.MarshalIndent(results, "", "  ")
	return &mcpsdk.CallToolResult{
		Content: []mcpsdk.Content{&mcpsdk.TextContent{Text: string(data)}},
	}, nil
}

// --- sftp_read ---

func sftpReadTool() *mcpsdk.Tool {
	return &mcpsdk.Tool{
		Name:        "sftp_read",
		Description: "读取远程服务器上的文件内容。适用于查看配置文件、日志片段等。大文件会截断到 max_size 字节。",
		InputSchema: json.RawMessage(`{
			"type": "object",
			"properties": {
				"session_path": {
					"type": "string",
					"description": "会话路径"
				},
				"remote_path": {
					"type": "string",
					"description": "远程文件路径"
				},
				"max_size": {
					"type": "integer",
					"description": "最大读取字节数，默认 1MB",
					"default": 1048576
				},
				"offset": {
					"type": "integer",
					"description": "读取起始偏移量（字节），用于大文件分段读取",
					"default": 0
				}
			},
			"required": ["session_path", "remote_path"]
		}`),
	}
}

func handleSFTPRead(_ context.Context, req *mcpsdk.CallToolRequest) (*mcpsdk.CallToolResult, error) {
	var args struct {
		SessionPath string `json:"session_path"`
		RemotePath  string `json:"remote_path"`
		MaxSize     int    `json:"max_size"`
		Offset      int64  `json:"offset"`
	}
	if err := json.Unmarshal(req.Params.Arguments, &args); err != nil {
		return nil, fmt.Errorf("参数解析失败: %w", err)
	}

	maxSize := args.MaxSize
	if maxSize <= 0 {
		maxSize = 1048576 // 1MB
	}

	sftpClient, cleanup, err := connectSFTP(args.SessionPath)
	if err != nil {
		return errorResult(err.Error()), nil
	}
	defer cleanup()

	f, err := sftpClient.Open(args.RemotePath)
	if err != nil {
		return errorResult(fmt.Sprintf("打开文件失败 '%s': %v", args.RemotePath, err)), nil
	}
	defer f.Close()

	stat, err := f.Stat()
	if err != nil {
		return errorResult(fmt.Sprintf("获取文件信息失败: %v", err)), nil
	}

	if args.Offset > 0 {
		if _, err := f.Seek(args.Offset, io.SeekStart); err != nil {
			return errorResult(fmt.Sprintf("文件偏移失败: %v", err)), nil
		}
	}

	buf := make([]byte, maxSize)
	n, _ := io.ReadFull(f, buf)
	truncated := stat.Size() > args.Offset+int64(n)

	result := struct {
		Content   string `json:"content"`
		Size      int64  `json:"size"`
		Truncated bool   `json:"truncated"`
	}{
		Content:   string(buf[:n]),
		Size:      stat.Size(),
		Truncated: truncated,
	}

	data, _ := json.MarshalIndent(result, "", "  ")
	return &mcpsdk.CallToolResult{
		Content: []mcpsdk.Content{&mcpsdk.TextContent{Text: string(data)}},
	}, nil
}

// --- sftp_write ---

func sftpWriteTool() *mcpsdk.Tool {
	return &mcpsdk.Tool{
		Name:        "sftp_write",
		Description: "将内容写入远程服务器上的文件。可用于修改配置文件。会覆盖已有文件。",
		InputSchema: json.RawMessage(`{
			"type": "object",
			"properties": {
				"session_path": {
					"type": "string",
					"description": "会话路径"
				},
				"remote_path": {
					"type": "string",
					"description": "远程文件路径"
				},
				"content": {
					"type": "string",
					"description": "要写入的文件内容"
				},
				"mode": {
					"type": "string",
					"description": "文件权限，如 '0644'",
					"default": "0644"
				}
			},
			"required": ["session_path", "remote_path", "content"]
		}`),
	}
}

func handleSFTPWrite(_ context.Context, req *mcpsdk.CallToolRequest) (*mcpsdk.CallToolResult, error) {
	var args struct {
		SessionPath string `json:"session_path"`
		RemotePath  string `json:"remote_path"`
		Content     string `json:"content"`
		Mode        string `json:"mode"`
	}
	if err := json.Unmarshal(req.Params.Arguments, &args); err != nil {
		return nil, fmt.Errorf("参数解析失败: %w", err)
	}

	sftpClient, cleanup, err := connectSFTP(args.SessionPath)
	if err != nil {
		return errorResult(err.Error()), nil
	}
	defer cleanup()

	f, err := sftpClient.Create(args.RemotePath)
	if err != nil {
		return errorResult(fmt.Sprintf("创建文件失败 '%s': %v", args.RemotePath, err)), nil
	}

	n, err := f.Write([]byte(args.Content))
	f.Close()
	if err != nil {
		return errorResult(fmt.Sprintf("写入文件失败: %v", err)), nil
	}

	// 设置文件权限
	if args.Mode != "" {
		mode, err := strconv.ParseUint(args.Mode, 8, 32)
		if err == nil {
			_ = sftpClient.Chmod(args.RemotePath, os.FileMode(mode))
		}
	}

	result := struct {
		Written int    `json:"bytes_written"`
		Path    string `json:"path"`
	}{
		Written: n,
		Path:    args.RemotePath,
	}
	data, _ := json.MarshalIndent(result, "", "  ")
	return &mcpsdk.CallToolResult{
		Content: []mcpsdk.Content{&mcpsdk.TextContent{Text: string(data)}},
	}, nil
}

// --- sftp_upload ---

func sftpUploadTool() *mcpsdk.Tool {
	return &mcpsdk.Tool{
		Name:        "sftp_upload",
		Description: "将本地文件上传到远程服务器。",
		InputSchema: json.RawMessage(`{
			"type": "object",
			"properties": {
				"session_path": {
					"type": "string",
					"description": "会话路径"
				},
				"local_path": {
					"type": "string",
					"description": "本地文件路径"
				},
				"remote_path": {
					"type": "string",
					"description": "远程目标路径"
				}
			},
			"required": ["session_path", "local_path", "remote_path"]
		}`),
	}
}

func handleSFTPUpload(_ context.Context, req *mcpsdk.CallToolRequest) (*mcpsdk.CallToolResult, error) {
	var args struct {
		SessionPath string `json:"session_path"`
		LocalPath   string `json:"local_path"`
		RemotePath  string `json:"remote_path"`
	}
	if err := json.Unmarshal(req.Params.Arguments, &args); err != nil {
		return nil, fmt.Errorf("参数解析失败: %w", err)
	}

	// 打开本地文件
	localFile, err := os.Open(args.LocalPath)
	if err != nil {
		return errorResult(fmt.Sprintf("打开本地文件失败 '%s': %v", args.LocalPath, err)), nil
	}
	defer localFile.Close()

	localStat, err := localFile.Stat()
	if err != nil {
		return errorResult(fmt.Sprintf("获取本地文件信息失败: %v", err)), nil
	}

	sftpClient, cleanup, err := connectSFTP(args.SessionPath)
	if err != nil {
		return errorResult(err.Error()), nil
	}
	defer cleanup()

	// 确保远程目录存在
	remoteDir := filepath.Dir(args.RemotePath)
	_ = sftpClient.MkdirAll(remoteDir)

	remoteFile, err := sftpClient.Create(args.RemotePath)
	if err != nil {
		return errorResult(fmt.Sprintf("创建远程文件失败 '%s': %v", args.RemotePath, err)), nil
	}

	n, err := io.Copy(remoteFile, localFile)
	remoteFile.Close()
	if err != nil {
		return errorResult(fmt.Sprintf("上传文件失败: %v", err)), nil
	}

	// 保持原文件权限
	_ = sftpClient.Chmod(args.RemotePath, localStat.Mode())

	result := struct {
		BytesTransferred int64  `json:"bytes_transferred"`
		LocalPath        string `json:"local_path"`
		RemotePath       string `json:"remote_path"`
	}{
		BytesTransferred: n,
		LocalPath:        args.LocalPath,
		RemotePath:       args.RemotePath,
	}
	data, _ := json.MarshalIndent(result, "", "  ")
	return &mcpsdk.CallToolResult{
		Content: []mcpsdk.Content{&mcpsdk.TextContent{Text: string(data)}},
	}, nil
}

// --- sftp_download ---

func sftpDownloadTool() *mcpsdk.Tool {
	return &mcpsdk.Tool{
		Name:        "sftp_download",
		Description: "从远程服务器下载文件到本地。",
		InputSchema: json.RawMessage(`{
			"type": "object",
			"properties": {
				"session_path": {
					"type": "string",
					"description": "会话路径"
				},
				"remote_path": {
					"type": "string",
					"description": "远程文件路径"
				},
				"local_path": {
					"type": "string",
					"description": "本地目标路径"
				}
			},
			"required": ["session_path", "remote_path", "local_path"]
		}`),
	}
}

func handleSFTPDownload(_ context.Context, req *mcpsdk.CallToolRequest) (*mcpsdk.CallToolResult, error) {
	var args struct {
		SessionPath string `json:"session_path"`
		RemotePath  string `json:"remote_path"`
		LocalPath   string `json:"local_path"`
	}
	if err := json.Unmarshal(req.Params.Arguments, &args); err != nil {
		return nil, fmt.Errorf("参数解析失败: %w", err)
	}

	sftpClient, cleanup, err := connectSFTP(args.SessionPath)
	if err != nil {
		return errorResult(err.Error()), nil
	}
	defer cleanup()

	remoteFile, err := sftpClient.Open(args.RemotePath)
	if err != nil {
		return errorResult(fmt.Sprintf("打开远程文件失败 '%s': %v", args.RemotePath, err)), nil
	}
	defer remoteFile.Close()

	// 确保本地目录存在
	localDir := filepath.Dir(args.LocalPath)
	if err := os.MkdirAll(localDir, 0755); err != nil {
		return errorResult(fmt.Sprintf("创建本地目录失败: %v", err)), nil
	}

	localFile, err := os.Create(args.LocalPath)
	if err != nil {
		return errorResult(fmt.Sprintf("创建本地文件失败 '%s': %v", args.LocalPath, err)), nil
	}

	n, err := io.Copy(localFile, remoteFile)
	localFile.Close()
	if err != nil {
		return errorResult(fmt.Sprintf("下载文件失败: %v", err)), nil
	}

	result := struct {
		BytesTransferred int64  `json:"bytes_transferred"`
		RemotePath       string `json:"remote_path"`
		LocalPath        string `json:"local_path"`
	}{
		BytesTransferred: n,
		RemotePath:       args.RemotePath,
		LocalPath:        args.LocalPath,
	}
	data, _ := json.MarshalIndent(result, "", "  ")
	return &mcpsdk.CallToolResult{
		Content: []mcpsdk.Content{&mcpsdk.TextContent{Text: string(data)}},
	}, nil
}
