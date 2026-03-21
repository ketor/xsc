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
	"time"

	"golang.org/x/crypto/ssh"

	"github.com/ketor/xsc/internal/mobaxterm"
	"github.com/ketor/xsc/internal/securecrt"
	"github.com/ketor/xsc/internal/session"
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
		listSessions()
	case "connect":
		if len(os.Args) < 3 {
			fmt.Fprintln(os.Stderr, "Usage: xssh connect <session_path>")
			os.Exit(1)
		}
		connectSession(os.Args[2])
	case "exec":
		if len(os.Args) < 4 {
			fmt.Fprintln(os.Stderr, "Usage: xssh exec <session_path> [options] <command>")
			os.Exit(1)
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

func listSessions() {
	sessionsDir, err := config.GetSessionsDir()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error getting sessions directory: %v\n", err)
		os.Exit(1)
	}

	sessions, err := session.LoadAllSessions(sessionsDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading sessions: %v\n", err)
		os.Exit(1)
	}

	for _, s := range sessions {
		relPath, _ := filepath.Rel(sessionsDir, s.FilePath)
		relPath = strings.TrimSuffix(relPath, ".yaml")
		fmt.Println(relPath)
	}
}

func connectSession(sessionPath string) {
	sessionsDir, err := config.GetSessionsDir()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error getting sessions directory: %v\n", err)
		os.Exit(1)
	}

	s, err := session.FindSession(sessionsDir, sessionPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Session not found: %s\n", sessionPath)
		os.Exit(1)
	}

	if err := internalssh.Connect(s); err != nil {
		fmt.Fprintf(os.Stderr, "Connection failed: %v\n", err)
		os.Exit(1)
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

	if len(cmdArgs) == 0 {
		fmt.Fprintln(os.Stderr, "错误: 未指定要执行的命令")
		os.Exit(1)
	}
	command := strings.Join(cmdArgs, " ")

	// 查找会话
	sessionsDir, err := config.GetSessionsDir()
	if err != nil {
		fmt.Fprintf(os.Stderr, "获取会话目录失败: %v\n", err)
		os.Exit(1)
	}

	s, err := session.FindSession(sessionsDir, sessionPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "会话未找到: %s\n", sessionPath)
		os.Exit(1)
	}

	// 解密密码
	if err := s.ResolvePassword(); err != nil {
		fmt.Fprintf(os.Stderr, "密码解密失败: %v\n", err)
		os.Exit(1)
	}

	// 建立 SSH 连接
	client, cleanup, err := internalssh.Dial(s)
	if err != nil {
		fmt.Fprintf(os.Stderr, "SSH 连接失败: %v\n", err)
		os.Exit(1)
	}
	defer client.Close()
	if cleanup != nil {
		defer cleanup()
	}

	// 创建会话
	sshSession, err := client.NewSession()
	if err != nil {
		fmt.Fprintf(os.Stderr, "创建 SSH 会话失败: %v\n", err)
		os.Exit(1)
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
			exitCode = 124 // 超时退出码
		} else {
			exitCode = 1
		}
	}

	if jsonOutput {
		result := map[string]interface{}{
			"stdout":    stdoutBuf.String(),
			"stderr":    stderrBuf.String(),
			"exit_code": exitCode,
		}
		if runErr != nil && ctx.Err() != nil {
			result["error"] = runErr.Error()
		}
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		enc.Encode(result)
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
}

// importSource 描述一个导入源的配置
type importSource struct {
	name           string // 来源名称，如 "SecureCRT"
	dirPrefix      string // 转换目录前缀，如 "securecrt-converted"
	enabled        bool
	loadAndConvert func() ([]importSession, error) // 加载并转换会话
}

func convertSessions(src importSource) {
	if !src.enabled {
		fmt.Fprintf(os.Stderr, "%s is not enabled in config\n", src.name)
		os.Exit(1)
	}

	sessions, err := src.loadAndConvert()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading %s sessions: %v\n", src.name, err)
		os.Exit(1)
	}

	if len(sessions) == 0 {
		fmt.Printf("No %s sessions found\n", src.name)
		return
	}

	// 获取 sessions 目录
	sessionsDir, err := config.GetSessionsDir()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error getting sessions directory: %v\n", err)
		os.Exit(1)
	}

	// 创建新的目录（年月日-时分秒格式）
	timestamp := time.Now().Format("20060102-150405")
	targetDir := filepath.Join(sessionsDir, src.dirPrefix, timestamp)

	fmt.Printf("Converting %d %s sessions...\n", len(sessions), src.name)
	fmt.Printf("Target directory: %s\n\n", targetDir)

	converted := 0
	errors := 0

	for _, s := range sessions {
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

		fmt.Printf("  ✓ %s\n", s.Name)
		converted++
	}

	fmt.Printf("\n✓ Converted: %d | ✗ Errors: %d\n", converted, errors)
	fmt.Printf("\nConverted sessions are saved in: %s\n", targetDir)
	fmt.Println("\nYou can now use 'xssh tui' to browse and connect to these sessions.")
}

func convertSecureCRT() {
	globalConfig, err := config.LoadGlobalConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading global config: %v\n", err)
		os.Exit(1)
	}

	convertSessions(importSource{
		name:      "SecureCRT",
		dirPrefix: "securecrt-converted",
		enabled:   globalConfig.SecureCRT.Enabled,
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
				if s.EncryptedPassword != "" && globalConfig.SecureCRT.Password != "" {
					if pwd, err := securecrt.DecryptPassword(s.EncryptedPassword, globalConfig.SecureCRT.Password); err == nil {
						s.Password = pwd
					}
				}
				result = append(result, importSession{
					Name:        s.Name,
					Folder:      s.Folder,
					Password:    s.Password,
					SessionData: s.ConvertToXSSHSession(),
				})
			}
			return result, nil
		},
	})
}

func convertXShell() {
	globalConfig, err := config.LoadGlobalConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading global config: %v\n", err)
		os.Exit(1)
	}

	convertSessions(importSource{
		name:      "Xshell",
		dirPrefix: "xshell-converted",
		enabled:   globalConfig.XShell.Enabled,
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
				if s.EncryptedPassword != "" && globalConfig.XShell.Password != "" {
					if pwd, err := xshell.DecryptPassword(s.EncryptedPassword, globalConfig.XShell.Password); err == nil {
						s.Password = pwd
					}
				}
				result = append(result, importSession{
					Name:        s.Name,
					Folder:      s.Folder,
					Password:    s.Password,
					SessionData: s.ConvertToXSSHSession(),
				})
			}
			return result, nil
		},
	})
}

func convertMobaXterm() {
	globalConfig, err := config.LoadGlobalConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading global config: %v\n", err)
		os.Exit(1)
	}

	convertSessions(importSource{
		name:      "MobaXterm",
		dirPrefix: "mobaxterm-converted",
		enabled:   globalConfig.MobaXterm.Enabled,
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
				if s.EncryptedPassword != "" && globalConfig.MobaXterm.Password != "" {
					if pwd, err := mobaxterm.DecryptPassword(s.EncryptedPassword, globalConfig.MobaXterm.Password); err == nil {
						s.Password = pwd
					}
				}
				result = append(result, importSession{
					Name:        s.Name,
					Folder:      s.Folder,
					Password:    s.Password,
					SessionData: s.ConvertToXSSHSession(),
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
	fmt.Println("  xssh connect <path>          Connect to a session")
	fmt.Println("  xssh exec <path> <cmd>       Execute a remote command")
	fmt.Println("  xssh exec <path> -t N <cmd>  Execute with timeout (default 30s, max 300s)")
	fmt.Println("  xssh exec <path> --json <cmd> JSON formatted output")
	fmt.Println("  xssh import-securecrt        Import SecureCRT sessions to local format")
	fmt.Println("  xssh import-xshell           Import Xshell sessions to local format")
	fmt.Println("  xssh import-mobaxterm        Import MobaXterm sessions to local format")
	fmt.Println("  xssh version                 Show version information")
	fmt.Println("  xssh help                    Show this help message")
	fmt.Println()
	fmt.Println("Examples:")
	fmt.Println("  xssh tui")
	fmt.Println("  xssh connect prod/db/master")
	fmt.Println("  xssh connect web-server")
	fmt.Println("  xssh exec prod/db/master uptime")
	fmt.Println("  xssh exec prod/db/master --json df -h")
	fmt.Println("  xssh import-securecrt")
	fmt.Println("  xssh import-xshell")
	fmt.Println("  xssh import-mobaxterm")
	fmt.Println()
	fmt.Println("Session files are stored in: ~/.xsc/sessions/")
}
