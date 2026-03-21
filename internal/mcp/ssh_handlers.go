package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"time"

	internalssh "github.com/ketor/xsc/internal/ssh"
	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"golang.org/x/crypto/ssh"
)

// SSHExecOutput ssh_exec 工具的输出结构
type SSHExecOutput struct {
	Stdout   string `json:"stdout"`
	Stderr   string `json:"stderr"`
	ExitCode int    `json:"exit_code"`
	Error    string `json:"error,omitempty"`
}

func sshExecTool() *mcpsdk.Tool {
	return &mcpsdk.Tool{
		Name:        "ssh_exec",
		Description: "通过 SSH 在远程服务器上执行命令并返回输出。适用于故障定位、状态检查、日志查看等运维操作。注意：每次调用会建立新的 SSH 连接，不支持交互式命令。",
		InputSchema: json.RawMessage(`{
			"type": "object",
			"properties": {
				"session_path": {
					"type": "string",
					"description": "会话路径，如 'prod/web/server-01'"
				},
				"command": {
					"type": "string",
					"description": "要执行的 shell 命令"
				},
				"timeout": {
					"type": "integer",
					"description": "命令超时时间（秒），默认 30，最大 300",
					"default": 30
				}
			},
			"required": ["session_path", "command"]
		}`),
	}
}

func handleSSHExec(ctx context.Context, req *mcpsdk.CallToolRequest) (*mcpsdk.CallToolResult, error) {
	var args struct {
		SessionPath string `json:"session_path"`
		Command     string `json:"command"`
		Timeout     int    `json:"timeout"`
	}
	if err := json.Unmarshal(req.Params.Arguments, &args); err != nil {
		return nil, fmt.Errorf("参数解析失败: %w", err)
	}

	// 参数校验
	timeout := args.Timeout
	if timeout <= 0 {
		timeout = 30
	}
	if timeout > 300 {
		timeout = 300
	}

	// 查找会话
	s, err := findSessionByPath(args.SessionPath)
	if err != nil {
		return errorResult(err.Error()), nil
	}

	// 建立 SSH 连接
	client, cleanup, err := internalssh.Dial(s)
	if err != nil {
		return errorResult(fmt.Sprintf("SSH 连接失败: %v", err)), nil
	}
	defer func() {
		client.Close()
		if cleanup != nil {
			cleanup()
		}
	}()

	// 创建 SSH 会话
	sshSession, err := client.NewSession()
	if err != nil {
		return errorResult(fmt.Sprintf("创建 SSH 会话失败: %v", err)), nil
	}
	defer sshSession.Close()

	var stdout, stderr bytes.Buffer
	sshSession.Stdout = &stdout
	sshSession.Stderr = &stderr

	// 带超时执行
	execCtx, cancel := context.WithTimeout(ctx, time.Duration(timeout)*time.Second)
	defer cancel()

	done := make(chan error, 1)
	go func() {
		done <- sshSession.Run(args.Command)
	}()

	var output SSHExecOutput
	select {
	case err := <-done:
		output.Stdout = stdout.String()
		output.Stderr = stderr.String()
		if err != nil {
			if exitErr, ok := err.(*ssh.ExitError); ok {
				output.ExitCode = exitErr.ExitStatus()
			} else {
				output.Error = err.Error()
				output.ExitCode = -1
			}
		}
	case <-execCtx.Done():
		// 超时，尝试发送 SIGKILL
		_ = sshSession.Signal(ssh.SIGKILL)
		output.Stdout = stdout.String()
		output.Stderr = stderr.String()
		output.ExitCode = -1
		output.Error = fmt.Sprintf("命令执行超时（%d秒）", timeout)
	}

	data, _ := json.MarshalIndent(output, "", "  ")
	return &mcpsdk.CallToolResult{
		Content: []mcpsdk.Content{&mcpsdk.TextContent{Text: string(data)}},
	}, nil
}
