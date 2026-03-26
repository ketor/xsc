package main

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"
	"golang.org/x/term"

	"github.com/ketor/xsc/internal/cli"
	"github.com/ketor/xsc/internal/mobaxterm"
	"github.com/ketor/xsc/internal/securecrt"
	"github.com/ketor/xsc/internal/session"
	"github.com/ketor/xsc/internal/shared"
	internalssh "github.com/ketor/xsc/internal/ssh"
	"github.com/ketor/xsc/internal/tui"
	"github.com/ketor/xsc/internal/xshell"
	"github.com/ketor/xsc/pkg/config"
	"github.com/ketor/xsc/pkg/version"
)

func main() {
	if len(os.Args) < 2 {
		// 默认显示帮助信息
		showHelp()
		return
	}

	command := os.Args[1]

	switch command {
	case "tui":
		// TUI 模式
		tui.Run()
	case "list":
		params := ListParams{}
		for _, a := range os.Args[2:] {
			if a == "--json" {
				params.JSON = true
			}
		}
		p := cli.NewPrinter(params.JSON, os.Stdout, os.Stderr)
		os.Exit(handleList(context.Background(), params, p))
	case "add":
		params := parseAddArgs(os.Args[2:])
		p := cli.NewPrinter(params.JSON, os.Stdout, os.Stderr)
		os.Exit(handleAdd(context.Background(), params, p))
	case "show":
		params := parseShowArgs(os.Args[2:])
		p := cli.NewPrinter(params.JSON, os.Stdout, os.Stderr)
		os.Exit(handleShow(context.Background(), params, p))
	case "edit":
		params := parseEditArgs(os.Args[2:])
		p := cli.NewPrinter(params.JSON, os.Stdout, os.Stderr)
		os.Exit(handleEdit(context.Background(), params, p))
	case "delete":
		params := parseDeleteArgs(os.Args[2:])
		p := cli.NewPrinter(params.JSON, os.Stdout, os.Stderr)
		os.Exit(handleDelete(context.Background(), params, p))
	case "connect":
		if len(os.Args) < 3 {
			fmt.Fprintln(os.Stderr, "Usage: xssh connect <session_path>")
			os.Exit(1)
		}
		connectSession(os.Args[2])
	case "ping":
		if len(os.Args) < 3 {
			fmt.Fprintln(os.Stderr, "Usage: xssh ping <path>[,path2,...] [--json] [--timeout 10s] [--parallel 5]")
			os.Exit(cli.ExitUsage)
		}
		params := parsePingArgs(os.Args[2:])
		p := cli.NewPrinter(params.JSON, os.Stdout, os.Stderr)
		os.Exit(handlePing(context.Background(), params, p))
	case "exec":
		if len(os.Args) < 4 {
			fmt.Fprintln(os.Stderr, "Usage: xssh exec <session_path> [options] <command>")
			os.Exit(1)
		}
		// 逗号分隔的多路径 → 批量并发模式
		if strings.Contains(os.Args[2], ",") {
			params := parseExecMultiArgs(os.Args[2], os.Args[3:])
			p := cli.NewPrinter(params.JSON, os.Stdout, os.Stderr)
			os.Exit(handleExecMulti(context.Background(), params, p))
		}
		execCommand(os.Args[2], os.Args[3:])
	case "import-securecrt":
		convertSecureCRT()
	case "import-xshell":
		convertXShell()
	case "import-mobaxterm":
		convertMobaXterm()
	case "version", "--version", "-v":
		fmt.Println(version.String("xssh"))
	case "help", "--help", "-h":
		showHelp()
	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n", command)
		showHelp()
		os.Exit(1)
	}
}

// resolveSessionPassword 统一处理会话密码解析：
// 1. ResolvePassword() 延迟解密（支持 XSC_MASTER_PASSWORD 环境变量）
// 2. XSC_PASSWORD 环境变量注入
// 3. 非 TTY 环境密码缺失检测
func resolveSessionPassword(s *session.Session) error {
	if err := s.ResolvePassword(); err != nil {
		return fmt.Errorf("密码解密失败: %w", err)
	}
	// XSC_PASSWORD 环境变量注入
	if s.AuthType == session.AuthTypePassword && s.Password == "" {
		if envPwd := os.Getenv("XSC_PASSWORD"); envPwd != "" {
			s.Password = envPwd
		}
	}
	// 非 TTY 环境下密码缺失，立即失败而非卡死
	if s.AuthType == session.AuthTypePassword && s.Password == "" {
		if !term.IsTerminal(int(os.Stdin.Fd())) {
			return fmt.Errorf("密码未设置且非 TTY 环境（设置 XSC_PASSWORD 环境变量）")
		}
	}
	return nil
}

func connectSession(sessionPath string) {
	s, err := shared.FindSessionAllSources(sessionPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Session not found: %s\n", sessionPath)
		os.Exit(cli.ExitNotFound)
	}

	if err := resolveSessionPassword(s); err != nil {
		fmt.Fprintf(os.Stderr, "认证失败: %v\n", err)
		os.Exit(cli.ExitAuthFailed)
	}

	if err := internalssh.Connect(s); err != nil {
		fmt.Fprintf(os.Stderr, "Connection failed: %v\n", err)
		os.Exit(cli.ExitConnFailed)
	}
}

// execCommand 在远程主机上执行命令
func execCommand(sessionPath string, args []string) {
	timeout := 30 // 默认超时 30 秒
	jsonOutput := false
	var cmdArgs []string

	// 解析选项
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "-t", "--timeout":
			if i+1 >= len(args) {
				fmt.Fprintln(os.Stderr, "错误: -t/--timeout 需要指定超时秒数")
				os.Exit(1)
			}
			i++
			t, err := strconv.Atoi(args[i])
			if err != nil || t <= 0 {
				fmt.Fprintln(os.Stderr, "错误: 超时秒数必须为正整数")
				os.Exit(1)
			}
			if t > 300 {
				t = 300
			}
			timeout = t
		case "--json":
			jsonOutput = true
		default:
			cmdArgs = append(cmdArgs, args[i])
		}
	}

	p := cli.NewPrinter(jsonOutput, os.Stdout, os.Stderr)

	if len(cmdArgs) == 0 {
		p.PrintErr(cli.NewCLIError(cli.ExitUsage, "未指定要执行的命令", ""))
		os.Exit(cli.ExitUsage)
	}
	command := strings.Join(cmdArgs, " ")

	// 查找会话（支持本地 YAML + SecureCRT/XShell/MobaXterm）
	s, err := shared.FindSessionAllSources(sessionPath)
	if err != nil {
		p.PrintErr(cli.NewCLIError(cli.ExitNotFound, "会话未找到", sessionPath))
		os.Exit(cli.ExitNotFound)
	}

	if err := resolveSessionPassword(s); err != nil {
		p.PrintErr(cli.NewCLIError(cli.ExitAuthFailed, "认证失败", err.Error()))
		os.Exit(cli.ExitAuthFailed)
	}

	// 建立 SSH 连接
	client, cleanup, err := internalssh.Dial(s)
	if err != nil {
		p.PrintErr(cli.NewCLIError(cli.ExitConnFailed, "SSH 连接失败", err.Error()))
		os.Exit(cli.ExitConnFailed)
	}
	defer client.Close()
	if cleanup != nil {
		defer cleanup()
	}

	// 创建会话
	sshSession, err := client.NewSession()
	if err != nil {
		p.PrintErr(cli.NewCLIError(cli.ExitConnFailed, "创建 SSH 会话失败", err.Error()))
		os.Exit(cli.ExitConnFailed)
	}
	defer sshSession.Close()

	var stdoutBuf, stderrBuf bytes.Buffer
	sshSession.Stdout = &stdoutBuf
	sshSession.Stderr = &stderrBuf

	// 使用 context 实现超时
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeout)*time.Second)
	defer cancel()

	// 在 goroutine 中执行命令
	errCh := make(chan error, 1)
	go func() {
		errCh <- sshSession.Run(command)
	}()

	var runErr error
	select {
	case <-ctx.Done():
		sshSession.Signal(ssh.SIGKILL)
		runErr = fmt.Errorf("命令执行超时（%d秒）", timeout)
	case runErr = <-errCh:
	}

	exitCode := 0
	if runErr != nil {
		if exitErr, ok := runErr.(*ssh.ExitError); ok {
			exitCode = exitErr.ExitStatus()
		} else if ctx.Err() != nil {
			exitCode = cli.ExitTimeout
		} else {
			exitCode = 1
		}
	}

	if p.IsJSON() {
		result := map[string]interface{}{
			"stdout":    stdoutBuf.String(),
			"stderr":    stderrBuf.String(),
			"exit_code": exitCode,
		}
		if runErr != nil && ctx.Err() != nil {
			result["error"] = runErr.Error()
		}
		p.Print(result)
	} else {
		if stdoutBuf.Len() > 0 {
			fmt.Print(stdoutBuf.String())
		}
		if stderrBuf.Len() > 0 {
			fmt.Fprint(os.Stderr, stderrBuf.String())
		}
	}

	os.Exit(exitCode)
}

// importSession 描述可导入的会话来源
type importSession struct {
	Name        string
	Folder      string
	Password    string
	SessionData map[string]interface{}
	DecryptErr  error // 解密失败时记录错误
}

// importSource 描述一个导入源的配置
type importSource struct {
	name              string // 来源名称，如 "SecureCRT"
	dirPrefix         string // 转换目录前缀，如 "securecrt-converted"
	enabled           bool
	skipDecryptErrors bool                            // --skip-decrypt-errors
	loadAndConvert    func() ([]importSession, error) // 加载并转换会话
}

func convertSessions(src importSource) {
	if !src.enabled {
		fmt.Fprintf(os.Stderr, "%s is not enabled in config\n", src.name)
		os.Exit(cli.ExitConfig)
	}

	sessions, err := src.loadAndConvert()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading %s sessions: %v\n", src.name, err)
		os.Exit(cli.ExitConfig)
	}

	if len(sessions) == 0 {
		fmt.Fprintf(os.Stderr, "No %s sessions found\n", src.name)
		return
	}

	// 获取 sessions 目录
	sessionsDir, err := config.GetSessionsDir()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error getting sessions directory: %v\n", err)
		os.Exit(cli.ExitConfig)
	}

	// 创建新的目录（年月日-时分秒格式）
	timestamp := time.Now().Format("20060102-150405")
	targetDir := filepath.Join(sessionsDir, src.dirPrefix, timestamp)

	fmt.Fprintf(os.Stderr, "Converting %d %s sessions...\n", len(sessions), src.name)
	fmt.Fprintf(os.Stderr, "Target directory: %s\n\n", targetDir)

	converted := 0
	errors := 0

	for _, s := range sessions {
		// 处理解密错误
		if s.DecryptErr != nil {
			if src.skipDecryptErrors {
				fmt.Fprintf(os.Stderr, "  ⚠ %s: 解密跳过: %v\n", s.Name, s.DecryptErr)
			} else {
				fmt.Fprintf(os.Stderr, "  ✗ %s: 解密失败: %v\n", s.Name, s.DecryptErr)
				errors++
				continue
			}
		}

		sessionData := s.SessionData

		// 创建 xssh Session（使用安全类型断言）
		host, _ := sessionData["host"].(string)
		port, _ := sessionData["port"].(int)
		user, _ := sessionData["user"].(string)
		authType, _ := sessionData["auth_type"].(string)

		xsshSession := &session.Session{
			Host:     host,
			Port:     port,
			User:     user,
			AuthType: session.AuthType(authType),
		}

		// 处理密码
		if pwd, ok := sessionData["password"].(string); ok && pwd != "" {
			xsshSession.Password = pwd
		} else if s.Password != "" {
			xsshSession.Password = s.Password
		}

		// 构建目标路径（保持目录层次结构）
		var targetPath string
		if s.Folder != "" {
			targetPath = filepath.Join(targetDir, s.Folder, s.Name+".yaml")
		} else {
			targetPath = filepath.Join(targetDir, s.Name+".yaml")
		}

		// 保存会话
		if err := session.SaveSession(xsshSession, targetPath); err != nil {
			fmt.Fprintf(os.Stderr, "  ✗ %s: %v\n", s.Name, err)
			errors++
			continue
		}

		fmt.Fprintf(os.Stderr, "  ✓ %s\n", s.Name)
		converted++
	}

	fmt.Fprintf(os.Stderr, "\n✓ Converted: %d | ✗ Errors: %d\n", converted, errors)
	fmt.Fprintf(os.Stderr, "\nConverted sessions are saved in: %s\n", targetDir)
	fmt.Fprintf(os.Stderr, "\nYou can now use 'xssh tui' to browse and connect to these sessions.\n")

	if errors > 0 {
		if converted == 0 {
			os.Exit(cli.ExitConfig)
		}
		os.Exit(cli.ExitPartial)
	}
}

// parseImportArgs 解析 import 命令的 --skip-decrypt-errors 参数
func parseImportArgs() bool {
	for _, arg := range os.Args[2:] {
		if arg == "--skip-decrypt-errors" {
			return true
		}
	}
	return false
}

// parseAddArgs 解析 add 命令参数
func parseAddArgs(args []string) AddParams {
	params := AddParams{}
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--host":
			if i+1 < len(args) {
				i++
				params.Host = args[i]
			}
		case "--port":
			if i+1 < len(args) {
				i++
				if p, err := strconv.Atoi(args[i]); err == nil {
					params.Port = p
				}
			}
		case "--user":
			if i+1 < len(args) {
				i++
				params.User = args[i]
			}
		case "--auth-type":
			if i+1 < len(args) {
				i++
				params.AuthType = args[i]
			}
		case "--password":
			if i+1 < len(args) {
				i++
				params.Password = args[i]
			}
		case "--key":
			if i+1 < len(args) {
				i++
				params.KeyPath = args[i]
			}
		case "--json":
			params.JSON = true
		default:
			// 第一个非 flag 参数是路径
			if !strings.HasPrefix(args[i], "-") && params.Path == "" {
				params.Path = args[i]
			}
		}
	}
	return params
}

// parseShowArgs 解析 show 命令参数
func parseShowArgs(args []string) ShowParams {
	params := ShowParams{}
	for _, arg := range args {
		if arg == "--json" {
			params.JSON = true
		} else if !strings.HasPrefix(arg, "-") && params.Path == "" {
			params.Path = arg
		}
	}
	return params
}

// parseEditArgs 解析 edit 命令参数
func parseEditArgs(args []string) EditParams {
	params := EditParams{}
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--host":
			if i+1 < len(args) {
				i++
				params.Host = args[i]
			}
		case "--port":
			if i+1 < len(args) {
				i++
				if p, err := strconv.Atoi(args[i]); err == nil {
					params.Port = p
				}
			}
		case "--user":
			if i+1 < len(args) {
				i++
				params.User = args[i]
			}
		case "--auth-type":
			if i+1 < len(args) {
				i++
				params.AuthType = args[i]
			}
		case "--password":
			if i+1 < len(args) {
				i++
				params.Password = args[i]
			}
		case "--key":
			if i+1 < len(args) {
				i++
				params.KeyPath = args[i]
			}
		case "--json":
			params.JSON = true
		default:
			// 第一个非 flag 参数是路径
			if !strings.HasPrefix(args[i], "-") && params.Path == "" {
				params.Path = args[i]
			}
		}
	}
	return params
}

// parseDeleteArgs 解析 delete 命令参数
func parseDeleteArgs(args []string) DeleteParams {
	params := DeleteParams{}
	for _, arg := range args {
		if arg == "--force" {
			params.Force = true
		} else if arg == "--json" {
			params.JSON = true
		} else if !strings.HasPrefix(arg, "-") && params.Path == "" {
			params.Path = arg
		}
	}
	return params
}

func convertSecureCRT() {
	skipDecrypt := parseImportArgs()
	globalConfig, err := config.LoadGlobalConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading global config: %v\n", err)
		os.Exit(cli.ExitConfig)
	}

	convertSessions(importSource{
		name:              "SecureCRT",
		dirPrefix:         "securecrt-converted",
		enabled:           globalConfig.SecureCRT.Enabled,
		skipDecryptErrors: skipDecrypt,
		loadAndConvert: func() ([]importSession, error) {
			scConfig := securecrt.Config{
				SessionPath: globalConfig.SecureCRT.SessionPath,
				Password:    globalConfig.SecureCRT.Password,
			}
			scSessions, err := securecrt.LoadSessions(scConfig)
			if err != nil {
				return nil, err
			}
			var result []importSession
			for _, s := range scSessions {
				var decryptErr error
				if s.EncryptedPassword != "" && globalConfig.SecureCRT.Password != "" {
					pwd, err := securecrt.DecryptPassword(s.EncryptedPassword, globalConfig.SecureCRT.Password)
					if err != nil {
						decryptErr = err
					} else {
						s.Password = pwd
					}
				}
				result = append(result, importSession{
					Name:        s.Name,
					Folder:      s.Folder,
					Password:    s.Password,
					SessionData: s.ConvertToXSSHSession(),
					DecryptErr:  decryptErr,
				})
			}
			return result, nil
		},
	})
}

func convertXShell() {
	skipDecrypt := parseImportArgs()
	globalConfig, err := config.LoadGlobalConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading global config: %v\n", err)
		os.Exit(cli.ExitConfig)
	}

	convertSessions(importSource{
		name:              "Xshell",
		dirPrefix:         "xshell-converted",
		enabled:           globalConfig.XShell.Enabled,
		skipDecryptErrors: skipDecrypt,
		loadAndConvert: func() ([]importSession, error) {
			xsConfig := xshell.Config{
				SessionPath: globalConfig.XShell.SessionPath,
				Password:    globalConfig.XShell.Password,
			}
			xsSessions, err := xshell.LoadSessions(xsConfig)
			if err != nil {
				return nil, err
			}
			var result []importSession
			for _, s := range xsSessions {
				var decryptErr error
				if s.EncryptedPassword != "" && globalConfig.XShell.Password != "" {
					pwd, err := xshell.DecryptPassword(s.EncryptedPassword, globalConfig.XShell.Password)
					if err != nil {
						decryptErr = err
					} else {
						s.Password = pwd
					}
				}
				result = append(result, importSession{
					Name:        s.Name,
					Folder:      s.Folder,
					Password:    s.Password,
					SessionData: s.ConvertToXSSHSession(),
					DecryptErr:  decryptErr,
				})
			}
			return result, nil
		},
	})
}

func convertMobaXterm() {
	skipDecrypt := parseImportArgs()
	globalConfig, err := config.LoadGlobalConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading global config: %v\n", err)
		os.Exit(cli.ExitConfig)
	}

	convertSessions(importSource{
		name:              "MobaXterm",
		dirPrefix:         "mobaxterm-converted",
		enabled:           globalConfig.MobaXterm.Enabled,
		skipDecryptErrors: skipDecrypt,
		loadAndConvert: func() ([]importSession, error) {
			mxConfig := mobaxterm.Config{
				SessionPath: globalConfig.MobaXterm.SessionPath,
				Password:    globalConfig.MobaXterm.Password,
			}
			mxSessions, err := mobaxterm.LoadSessions(mxConfig)
			if err != nil {
				return nil, err
			}
			var result []importSession
			for _, s := range mxSessions {
				var decryptErr error
				if s.EncryptedPassword != "" && globalConfig.MobaXterm.Password != "" {
					pwd, err := mobaxterm.DecryptPassword(s.EncryptedPassword, globalConfig.MobaXterm.Password)
					if err != nil {
						decryptErr = err
					} else {
						s.Password = pwd
					}
				}
				result = append(result, importSession{
					Name:        s.Name,
					Folder:      s.Folder,
					Password:    s.Password,
					SessionData: s.ConvertToXSSHSession(),
					DecryptErr:  decryptErr,
				})
			}
			return result, nil
		},
	})
}

func showHelp() {
	fmt.Println("xssh - XShell CLI - SSH Session Manager")
	fmt.Println()
	fmt.Println("Usage:")
	fmt.Println("  xssh                         Show this help message")
	fmt.Println("  xssh tui                     Launch TUI mode")
	fmt.Println("  xssh list                    List all sessions")
	fmt.Println("  xssh list --json             List all sessions in JSON format")
	fmt.Println("  xssh add <path> --host H     Create a new session")
	fmt.Println("  xssh show <path>             Show session details")
	fmt.Println("  xssh edit <path> [options]   Edit session fields")
	fmt.Println("  xssh delete <path>           Delete a session")
	fmt.Println("  xssh ping <path>             Test SSH connectivity")
	fmt.Println("  xssh ping <p1,p2,...> [--parallel 5]  Batch ping")
	fmt.Println("  xssh connect <path>          Connect to a session")
	fmt.Println("  xssh exec <path> <cmd>       Execute a remote command")
	fmt.Println("  xssh exec <path> -t N <cmd>  Execute with timeout (default 30s, max 300s)")
	fmt.Println("  xssh exec <path> --json <cmd> JSON formatted output")
	fmt.Println("  xssh exec <p1,p2,...> [--parallel 5] [--fail-fast] <cmd>  Batch exec")
	fmt.Println("  xssh import-securecrt [--skip-decrypt-errors]  Import SecureCRT sessions")
	fmt.Println("  xssh import-xshell [--skip-decrypt-errors]     Import Xshell sessions")
	fmt.Println("  xssh import-mobaxterm [--skip-decrypt-errors]   Import MobaXterm sessions")
	fmt.Println("  xssh version                 Show version information")
	fmt.Println("  xssh help                    Show this help message")
	fmt.Println()
	fmt.Println("Edit options:")
	fmt.Println("  --host <host>         Set new host")
	fmt.Println("  --port <port>         Set new port")
	fmt.Println("  --user <user>         Set new user")
	fmt.Println("  --auth-type <type>    Set auth type (password|key|agent)")
	fmt.Println("  --password <pass>     Set new password")
	fmt.Println("  --key <keypath>       Set new key path")
	fmt.Println("  --json                JSON output")
	fmt.Println()
	fmt.Println("Examples:")
	fmt.Println("  xssh tui")
	fmt.Println("  xssh connect prod/db/master")
	fmt.Println("  xssh connect web-server")
	fmt.Println("  xssh exec prod/db/master uptime")
	fmt.Println("  xssh exec prod/db/master --json df -h")
	fmt.Println("  xssh edit prod/web1 --host 10.0.0.2 --port 2222")
	fmt.Println("  xssh delete old-server")
	fmt.Println("  xssh import-securecrt")
	fmt.Println("  xssh import-xshell")
	fmt.Println("  xssh import-mobaxterm")
	fmt.Println()
	fmt.Println("Session files are stored in: ~/.xsc/sessions/")
}
