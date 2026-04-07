package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/ssh"

	"github.com/ketor/xsc/internal/cli"
	"github.com/ketor/xsc/internal/session"
	"github.com/ketor/xsc/internal/shared"
	internalssh "github.com/ketor/xsc/internal/ssh"
	"github.com/ketor/xsc/pkg/config"
)

// SessionInfo 是会话的 JSON 输出结构
type SessionInfo struct {
	Path     string `json:"path"`
	Host     string `json:"host"`
	Port     int    `json:"port"`
	User     string `json:"user"`
	AuthType string `json:"auth_type"`
}

// ListParams 是 list 命令的参数
type ListParams struct {
	JSON bool
}

// handleList 列出所有会话，返回退出码
func handleList(_ context.Context, _ ListParams, p *cli.Printer) int {
	entries := shared.LoadAllSessionsFlat()
	if entries == nil {
		p.PrintErr(cli.NewCLIError(cli.ExitConfig, "Error loading sessions", ""))
		return cli.ExitConfig
	}

	if p.IsJSON() {
		result := make([]SessionInfo, 0, len(entries))
		for _, e := range entries {
			si := SessionInfo{Path: e.Path}
			if e.Session != nil {
				si.Host = e.Session.Host
				si.Port = e.Session.Port
				si.User = e.Session.User
				si.AuthType = string(e.Session.AuthType)
			}
			result = append(result, si)
		}
		p.Print(result)
	} else {
		for _, e := range entries {
			p.Print(e.Path)
		}
	}
	return cli.ExitOK
}

// PingParams 是 ping 命令的参数
type PingParams struct {
	Paths    []string
	JSON     bool
	Timeout  time.Duration
	Parallel int
}

// PingResult 是单个 ping 的结果
type PingResult struct {
	Session   string `json:"session"`
	OK        bool   `json:"ok"`
	LatencyMS int64  `json:"latency_ms,omitempty"`
	Error     string `json:"error,omitempty"`
}

// DialFunc 是 SSH Dial 的函数签名，便于测试注入
type DialFunc func(s *session.Session) (cleanup func(), err error)

// defaultDial 使用真实 SSH 连接
func defaultDial(s *session.Session) (func(), error) {
	client, cleanup, err := internalssh.Dial(s)
	if err != nil {
		return nil, err
	}
	// 关闭 client 连接
	return func() {
		client.Close()
		if cleanup != nil {
			cleanup()
		}
	}, nil
}

// pingOne 对单个会话执行连通性检测
func pingOne(ctx context.Context, path string, dial DialFunc) PingResult {
	result := PingResult{Session: path}

	s, err := shared.FindSessionAllSources(path)
	if err != nil {
		result.Error = "会话未找到"
		return result
	}

	_ = resolveSessionPassword(s)

	start := time.Now()

	// 用 goroutine + select 实现 context 超时
	type dialResult struct {
		cleanup func()
		err     error
	}
	ch := make(chan dialResult, 1)
	go func() {
		cleanup, dialErr := dial(s)
		ch <- dialResult{cleanup, dialErr}
	}()

	select {
	case <-ctx.Done():
		result.LatencyMS = time.Since(start).Milliseconds()
		result.Error = fmt.Sprintf("超时（%s）", ctx.Err())
		return result
	case dr := <-ch:
		result.LatencyMS = time.Since(start).Milliseconds()
		if dr.err != nil {
			result.Error = dr.err.Error()
			return result
		}
		if dr.cleanup != nil {
			dr.cleanup()
		}
		result.OK = true
		return result
	}
}

// handlePing 执行连通性检测，返回退出码
func handlePing(ctx context.Context, params PingParams, p *cli.Printer) int {
	return handlePingWithDial(ctx, params, p, defaultDial)
}

// handlePingWithDial 内部实现，支持注入 DialFunc
func handlePingWithDial(ctx context.Context, params PingParams, p *cli.Printer, dial DialFunc) int {
	if len(params.Paths) == 0 {
		p.PrintErr(cli.NewCLIError(cli.ExitUsage, "未指定会话路径", ""))
		return cli.ExitUsage
	}

	// 单个会话直接执行
	if len(params.Paths) == 1 {
		pingCtx, cancel := context.WithTimeout(ctx, params.Timeout)
		defer cancel()
		result := pingOne(pingCtx, params.Paths[0], dial)
		printPingResult(p, result)
		if !result.OK {
			return cli.ExitConnFailed
		}
		return cli.ExitOK
	}

	// 批量并发执行
	sem := make(chan struct{}, params.Parallel)
	results := make(chan PingResult, len(params.Paths))

	var wg sync.WaitGroup
	for _, path := range params.Paths {
		wg.Add(1)
		go func(p string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			pingCtx, cancel := context.WithTimeout(ctx, params.Timeout)
			defer cancel()
			results <- pingOne(pingCtx, p, dial)
		}(path)
	}

	// 收集结果的 goroutine
	go func() {
		wg.Wait()
		close(results)
	}()

	hasFailure := false
	for result := range results {
		printPingResult(p, result)
		if !result.OK {
			hasFailure = true
		}
	}

	if hasFailure {
		return cli.ExitPartial
	}
	return cli.ExitOK
}

// printPingResult 输出单个 ping 结果
func printPingResult(p *cli.Printer, r PingResult) {
	if p.IsJSON() {
		// NDJSON：逐行输出
		json.NewEncoder(p.Out).Encode(r)
	} else {
		if r.OK {
			fmt.Fprintf(p.Out, "✓ %s (%dms)\n", r.Session, r.LatencyMS)
		} else {
			fmt.Fprintf(p.Out, "✗ %s (%s)\n", r.Session, r.Error)
		}
	}
}

// AddParams 是 add 命令的参数
type AddParams struct {
	Path      string
	Host      string
	Port      int    // 默认 22
	User      string // 默认 root
	AuthType  string // password|key|agent，默认 password
	Password  string
	KeyPath   string
	ProxyJump string
	JSON      bool
}

// AddResult 是 add 命令的输出结构
type AddResult struct {
	Path    string `json:"path"`
	Created bool   `json:"created"`
}

// handleAdd 创建新会话，返回退出码
func handleAdd(_ context.Context, params AddParams, p *cli.Printer) int {
	if params.Host == "" {
		p.PrintErr(cli.NewCLIError(cli.ExitUsage, "缺少 --host 参数", ""))
		return cli.ExitUsage
	}
	if params.Path == "" {
		p.PrintErr(cli.NewCLIError(cli.ExitUsage, "缺少会话路径", ""))
		return cli.ExitUsage
	}

	// 默认值
	port := params.Port
	if port == 0 {
		port = 22
	}
	user := params.User
	if user == "" {
		user = "root"
	}
	authType := params.AuthType
	if authType == "" {
		authType = "password"
	}

	// 构造 Session
	s := &session.Session{
		Host:     params.Host,
		Port:     port,
		User:     user,
		AuthType: session.AuthType(authType),
	}

	// 密码处理
	password := params.Password
	if authType == "password" && password == "" {
		if envPwd := os.Getenv("XSC_PASSWORD"); envPwd != "" {
			password = envPwd
		}
	}
	if password != "" {
		s.Password = password
	}
	if params.KeyPath != "" {
		s.KeyPath = params.KeyPath
	}
	if params.ProxyJump != "" {
		s.ProxyJump = params.ProxyJump
	}

	// 会话文件路径
	sessionsDir, err := config.GetSessionsDir()
	if err != nil {
		p.PrintErr(cli.NewCLIError(cli.ExitConfig, "获取会话目录失败", err.Error()))
		return cli.ExitConfig
	}
	filePath := filepath.Join(sessionsDir, params.Path+".yaml")

	// 原子写入
	if err := cli.WriteYAML(filePath, s); err != nil {
		p.PrintErr(cli.NewCLIError(cli.ExitFileOp, "写入会话失败", err.Error()))
		return cli.ExitFileOp
	}

	result := AddResult{Path: params.Path, Created: true}
	if p.IsJSON() {
		p.Print(result)
	} else {
		p.Print(fmt.Sprintf("✓ 会话已创建: %s", params.Path))
	}
	return cli.ExitOK
}

// ShowParams 是 show 命令的参数
type ShowParams struct {
	Path string
	JSON bool
}

// ShowResult 是 show 命令的输出结构
type ShowResult struct {
	Path     string `json:"path"`
	Host     string `json:"host"`
	Port     int    `json:"port"`
	User     string `json:"user"`
	AuthType string `json:"auth_type"`
	KeyPath  string `json:"key_path,omitempty"`
	Source   string `json:"source,omitempty"`
}

// handleShow 查看会话详情，返回退出码
func handleShow(_ context.Context, params ShowParams, p *cli.Printer) int {
	if params.Path == "" {
		p.PrintErr(cli.NewCLIError(cli.ExitUsage, "缺少会话路径", ""))
		return cli.ExitUsage
	}

	s, err := shared.FindSessionAllSources(params.Path)
	if err != nil {
		p.PrintErr(cli.NewCLIError(cli.ExitNotFound, "会话未找到", params.Path))
		return cli.ExitNotFound
	}

	source := "local"
	if s.PasswordSource != "" {
		source = s.PasswordSource
	}

	result := ShowResult{
		Path:     params.Path,
		Host:     s.Host,
		Port:     s.Port,
		User:     s.User,
		AuthType: string(s.AuthType),
		KeyPath:  s.KeyPath,
		Source:   source,
	}

	if p.IsJSON() {
		p.Print(result)
	} else {
		p.Print(fmt.Sprintf("Path:      %s", result.Path))
		p.Print(fmt.Sprintf("Host:      %s", result.Host))
		p.Print(fmt.Sprintf("Port:      %d", result.Port))
		p.Print(fmt.Sprintf("User:      %s", result.User))
		p.Print(fmt.Sprintf("AuthType:  %s", result.AuthType))
		if result.KeyPath != "" {
			p.Print(fmt.Sprintf("KeyPath:   %s", result.KeyPath))
		}
		if result.Source != "" {
			p.Print(fmt.Sprintf("Source:    %s", result.Source))
		}
	}
	return cli.ExitOK
}

// ExecMultiParams 是 exec 批量执行的参数
type ExecMultiParams struct {
	Paths        []string
	Command      string
	JSON         bool
	Timeout      time.Duration
	Parallel     int
	FailFast     bool
	IgnoreErrors bool
}

// ExecMultiResult 是单个 exec 的结果
type ExecMultiResult struct {
	Session  string `json:"session"`
	ExitCode int    `json:"exit_code"`
	Stdout   string `json:"stdout"`
	Stderr   string `json:"stderr,omitempty"`
}

// ExecResult 是 exec 的结果（包含 Error 字段用于内部传递）
type ExecResult struct {
	Session  string `json:"session"`
	ExitCode int    `json:"exit_code"`
	Stdout   string `json:"stdout"`
	Stderr   string `json:"stderr,omitempty"`
	Error    string `json:"error,omitempty"`
}

// ExecMultiSummary 是批量执行的汇总
type ExecMultiSummary struct {
	Type    string `json:"_type"`
	Total   int    `json:"total"`
	Success int    `json:"success"`
	Failed  int    `json:"failed"`
}

// ExecFunc 是远程命令执行的函数签名，便于测试注入
type ExecFunc func(ctx context.Context, path string, command string) ExecResult

// defaultExec 使用真实 SSH 执行命令
func defaultExec(ctx context.Context, path string, command string) ExecResult {
	result := ExecResult{Session: path}

	s, err := shared.FindSessionAllSources(path)
	if err != nil {
		result.Stderr = "会话未找到"
		result.ExitCode = cli.ExitNotFound
		return result
	}

	if err := resolveSessionPassword(s); err != nil {
		result.Stderr = err.Error()
		result.ExitCode = cli.ExitAuthFailed
		return result
	}

	client, cleanup, err := internalssh.Dial(s)
	if err != nil {
		result.Stderr = err.Error()
		result.ExitCode = cli.ExitConnFailed
		return result
	}
	defer client.Close()
	if cleanup != nil {
		defer cleanup()
	}

	sshSession, err := client.NewSession()
	if err != nil {
		result.Stderr = err.Error()
		result.ExitCode = cli.ExitConnFailed
		return result
	}
	defer sshSession.Close()

	var stdoutBuf, stderrBuf bytes.Buffer
	sshSession.Stdout = &stdoutBuf
	sshSession.Stderr = &stderrBuf

	errCh := make(chan error, 1)
	go func() {
		errCh <- sshSession.Run(command)
	}()

	var runErr error
	select {
	case <-ctx.Done():
		sshSession.Signal(ssh.SIGKILL)
		runErr = ctx.Err()
	case runErr = <-errCh:
	}

	result.Stdout = stdoutBuf.String()
	result.Stderr = stderrBuf.String()

	if runErr != nil {
		if exitErr, ok := runErr.(*ssh.ExitError); ok {
			result.ExitCode = exitErr.ExitStatus()
		} else if ctx.Err() != nil {
			result.ExitCode = cli.ExitTimeout
			if result.Stderr == "" {
				result.Stderr = "命令执行超时"
			}
		} else {
			result.ExitCode = 1
			if result.Stderr == "" {
				result.Stderr = runErr.Error()
			}
		}
	}

	return result
}

// handleExecMulti 批量并发执行远程命令，返回退出码
func handleExecMulti(ctx context.Context, params ExecMultiParams, p *cli.Printer) int {
	return handleExecMultiWithFunc(ctx, params, p, defaultExec)
}

// handleExecMultiWithFunc 内部实现，支持注入 ExecFunc
func handleExecMultiWithFunc(ctx context.Context, params ExecMultiParams, p *cli.Printer, execFn ExecFunc) int {
	if len(params.Paths) == 0 {
		p.PrintErr(cli.NewCLIError(cli.ExitUsage, "未指定会话路径", ""))
		return cli.ExitUsage
	}
	if params.Command == "" {
		p.PrintErr(cli.NewCLIError(cli.ExitUsage, "未指定要执行的命令", ""))
		return cli.ExitUsage
	}

	// 可取消的 context 用于 fail-fast
	execCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	sem := make(chan struct{}, params.Parallel)
	results := make(chan ExecResult, len(params.Paths))

	var wg sync.WaitGroup
	for _, path := range params.Paths {
		wg.Add(1)
		go func(p string) {
			defer wg.Done()

			// 检查是否已取消（fail-fast）
			select {
			case <-execCtx.Done():
				results <- ExecResult{
					Session:  p,
					ExitCode: cli.ExitTimeout,
					Error:    "已取消（fail-fast）",
				}
				return
			case sem <- struct{}{}:
			}
			defer func() { <-sem }()

			cmdCtx, cmdCancel := context.WithTimeout(execCtx, params.Timeout)
			defer cmdCancel()
			results <- execFn(cmdCtx, p, params.Command)
		}(path)
	}

	go func() {
		wg.Wait()
		close(results)
	}()

	hasFailure := false
	for result := range results {
		printExecResult(p, result)
		if result.ExitCode != 0 {
			hasFailure = true
			if params.FailFast {
				cancel()
			}
		}
	}

	if hasFailure && !params.IgnoreErrors {
		return cli.ExitPartial
	}
	return cli.ExitOK
}

// printExecResult 输出单个 exec 结果
func printExecResult(p *cli.Printer, r ExecResult) {
	if p.IsJSON() {
		json.NewEncoder(p.Out).Encode(r)
	} else {
		if r.Error != "" {
			fmt.Fprintf(p.Out, "=== %s (失败: %s) ===\n", r.Session, r.Error)
		} else if r.ExitCode != 0 {
			fmt.Fprintf(p.Out, "=== %s (退出码: %d) ===\n", r.Session, r.ExitCode)
		} else {
			fmt.Fprintf(p.Out, "=== %s ===\n", r.Session)
		}
		if r.Stdout != "" {
			fmt.Fprint(p.Out, r.Stdout)
		}
		if r.Stderr != "" {
			fmt.Fprintf(p.Err, "%s", r.Stderr)
		}
	}
}

// parseExecMultiArgs 解析 exec 批量命令参数
// 格式: <path1,path2,...> [--json] [--timeout 30] [--parallel 5] [--fail-fast] [--ignore-errors] <command...>
func parseExecMultiArgs(sessionPaths string, args []string) ExecMultiParams {
	params := ExecMultiParams{
		Paths:    strings.Split(sessionPaths, ","),
		Timeout:  30 * time.Second,
		Parallel: 5,
	}

	var cmdArgs []string
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--json":
			params.JSON = true
		case "--timeout", "-t":
			if i+1 < len(args) {
				i++
				if n, err := strconv.Atoi(args[i]); err == nil && n > 0 {
					if n > 300 {
						n = 300
					}
					params.Timeout = time.Duration(n) * time.Second
				}
			}
		case "--parallel", "-p":
			if i+1 < len(args) {
				i++
				if n, err := strconv.Atoi(args[i]); err == nil && n > 0 {
					params.Parallel = n
				}
			}
		case "--fail-fast":
			params.FailFast = true
		case "--ignore-errors":
			params.IgnoreErrors = true
		default:
			cmdArgs = append(cmdArgs, args[i])
		}
	}

	params.Command = strings.Join(cmdArgs, " ")
	return params
}

// parsePingArgs 解析 ping 命令参数
func parsePingArgs(args []string) PingParams {
	params := PingParams{
		Timeout:  10 * time.Second,
		Parallel: 5,
	}

	if len(args) == 0 {
		return params
	}

	// 第一个非-开头参数是路径（逗号分隔）
	params.Paths = strings.Split(args[0], ",")

	for i := 1; i < len(args); i++ {
		switch args[i] {
		case "--json":
			params.JSON = true
		case "--timeout", "-t":
			if i+1 < len(args) {
				i++
				if d, err := time.ParseDuration(args[i]); err == nil && d > 0 {
					params.Timeout = d
				}
			}
		case "--parallel", "-p":
			if i+1 < len(args) {
				i++
				if n, err := strconv.Atoi(args[i]); err == nil && n > 0 {
					params.Parallel = n
				}
			}
		}
	}

	return params
}

// EditParams 是 edit 命令的参数
type EditParams struct {
	Path      string
	Host      string
	Port      int // 0 表示不修改
	User      string
	AuthType  string
	Password  string
	KeyPath   string
	ProxyJump string
	JSON      bool
}

// EditResult 是 edit 命令的输出结构
type EditResult struct {
	Path    string `json:"path"`
	Updated bool   `json:"updated"`
}

// handleEdit 修改会话字段，返回退出码
func handleEdit(_ context.Context, params EditParams, p *cli.Printer) int {
	if params.Path == "" {
		p.PrintErr(cli.NewCLIError(cli.ExitUsage, "缺少会话路径", ""))
		return cli.ExitUsage
	}

	s, err := shared.FindSessionAllSources(params.Path)
	if err != nil {
		p.PrintErr(cli.NewCLIError(cli.ExitNotFound, "会话未找到", params.Path))
		return cli.ExitNotFound
	}

	// 检查会话来源：非本地会话无法编辑
	if s.PasswordSource != "" {
		p.PrintErr(cli.NewCLIError(cli.ExitUsage, "无法编辑非本地会话", fmt.Sprintf("会话来源于 %s", s.PasswordSource)))
		return cli.ExitUsage
	}

	// 只更新指定字段（非零值/非空串的字段才覆盖）
	if params.Host != "" {
		s.Host = params.Host
	}
	if params.Port != 0 {
		s.Port = params.Port
	}
	if params.User != "" {
		s.User = params.User
	}
	if params.AuthType != "" {
		s.AuthType = session.AuthType(params.AuthType)
	}
	if params.Password != "" {
		s.Password = params.Password
	}
	if params.KeyPath != "" {
		s.KeyPath = params.KeyPath
	}
	if params.ProxyJump != "" {
		s.ProxyJump = params.ProxyJump
	}

	// 获取会话文件路径
	sessionsDir, err := config.GetSessionsDir()
	if err != nil {
		p.PrintErr(cli.NewCLIError(cli.ExitConfig, "获取会话目录失败", err.Error()))
		return cli.ExitConfig
	}
	filePath := filepath.Join(sessionsDir, params.Path+".yaml")

	// 原子写入
	if err := cli.WriteYAML(filePath, s); err != nil {
		p.PrintErr(cli.NewCLIError(cli.ExitFileOp, "写入会话失败", err.Error()))
		return cli.ExitFileOp
	}

	result := EditResult{Path: params.Path, Updated: true}
	if p.IsJSON() {
		p.Print(result)
	} else {
		p.Print(fmt.Sprintf("✓ 会话已更新: %s", params.Path))
	}
	return cli.ExitOK
}

// DeleteParams 是 delete 命令的参数
type DeleteParams struct {
	Path  string
	Force bool
	JSON  bool
}

// DeleteResult 是 delete 命令的输出结构
type DeleteResult struct {
	Path    string `json:"path"`
	Deleted bool   `json:"deleted"`
}

// handleDelete 删除会话，返回退出码
func handleDelete(_ context.Context, params DeleteParams, p *cli.Printer) int {
	if params.Path == "" {
		p.PrintErr(cli.NewCLIError(cli.ExitUsage, "缺少会话路径", ""))
		return cli.ExitUsage
	}

	// 构建会话文件路径
	sessionsDir, err := config.GetSessionsDir()
	if err != nil {
		p.PrintErr(cli.NewCLIError(cli.ExitConfig, "获取会话目录失败", err.Error()))
		return cli.ExitConfig
	}
	filePath := filepath.Join(sessionsDir, params.Path+".yaml")

	// 检查文件是否存在
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		p.PrintErr(cli.NewCLIError(cli.ExitNotFound, "会话未找到", params.Path))
		return cli.ExitNotFound
	}

	// 删除文件
	if err := os.Remove(filePath); err != nil {
		p.PrintErr(cli.NewCLIError(cli.ExitFileOp, "删除会话失败", err.Error()))
		return cli.ExitFileOp
	}

	result := DeleteResult{Path: params.Path, Deleted: true}
	if p.IsJSON() {
		p.Print(result)
	} else {
		p.Print(fmt.Sprintf("✓ 会话已删除: %s", params.Path))
	}
	return cli.ExitOK
}
