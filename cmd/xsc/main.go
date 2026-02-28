package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/user/xsc/internal/securecrt"
	"github.com/user/xsc/internal/session"
	"github.com/user/xsc/internal/ssh"
	"github.com/user/xsc/internal/tui"
	"github.com/user/xsc/pkg/config"
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
			fmt.Fprintln(os.Stderr, "Usage: xsc connect <session_path>")
			os.Exit(1)
		}
		connectSession(os.Args[2])
	case "import-securecrt":
		convertSecureCRT()
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

	// 尝试精确匹配
	s, err := session.LoadSession(filepath.Join(sessionsDir, sessionPath+".yaml"))
	if err != nil {
		// 尝试模糊匹配
		sessions, _ := session.LoadAllSessions(sessionsDir)
		for _, sess := range sessions {
			relPath, _ := filepath.Rel(sessionsDir, sess.FilePath)
			relPath = strings.TrimSuffix(relPath, ".yaml")
			if strings.Contains(relPath, sessionPath) || sessionPath == filepath.Base(relPath) {
				s = sess
				break
			}
		}
	}

	if s == nil {
		fmt.Fprintf(os.Stderr, "Session not found: %s\n", sessionPath)
		os.Exit(1)
	}

	if err := ssh.Connect(s); err != nil {
		fmt.Fprintf(os.Stderr, "Connection failed: %v\n", err)
		os.Exit(1)
	}
}

func convertSecureCRT() {
	// 加载全局配置
	globalConfig, err := config.LoadGlobalConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading global config: %v\n", err)
		os.Exit(1)
	}

	if !globalConfig.SecureCRT.Enabled {
		fmt.Fprintln(os.Stderr, "SecureCRT is not enabled in config")
		os.Exit(1)
	}

	// 加载所有 SecureCRT 会话
	scConfig := securecrt.Config{
		SessionPath: globalConfig.SecureCRT.SessionPath,
		Password:    globalConfig.SecureCRT.Password,
	}

	scSessions, err := securecrt.LoadSessions(scConfig)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading SecureCRT sessions: %v\n", err)
		os.Exit(1)
	}

	if len(scSessions) == 0 {
		fmt.Println("No SecureCRT sessions found")
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
	targetDir := filepath.Join(sessionsDir, "securecrt-converted", timestamp)

	fmt.Printf("Converting %d SecureCRT sessions...\n", len(scSessions))
	fmt.Printf("Target directory: %s\n\n", targetDir)

	converted := 0
	errors := 0

	for _, scSession := range scSessions {
		// 立即解密密码（如果有）
		if scSession.EncryptedPassword != "" && globalConfig.SecureCRT.Password != "" {
			decryptedPwd, err := securecrt.DecryptPassword(scSession.EncryptedPassword, globalConfig.SecureCRT.Password)
			if err == nil {
				scSession.Password = decryptedPwd
			}
		}

		// 转换为 xsc 会话
		sessionData := scSession.ConvertToXSCSession()

		// 创建 xsc Session
		xscSession := &session.Session{
			Host:     sessionData["host"].(string),
			Port:     sessionData["port"].(int),
			User:     sessionData["user"].(string),
			AuthType: session.AuthType(sessionData["auth_type"].(string)),
		}

		// 处理密码
		if pwd, ok := sessionData["password"].(string); ok && pwd != "" {
			xscSession.Password = pwd
		} else if scSession.Password != "" {
			xscSession.Password = scSession.Password
		}

		// 构建目标路径（保持目录层次结构）
		var targetPath string
		if scSession.Folder != "" {
			targetPath = filepath.Join(targetDir, scSession.Folder, scSession.Name+".yaml")
		} else {
			targetPath = filepath.Join(targetDir, scSession.Name+".yaml")
		}

		// 保存会话
		if err := session.SaveSession(xscSession, targetPath); err != nil {
			fmt.Fprintf(os.Stderr, "  ✗ %s: %v\n", scSession.Name, err)
			errors++
			continue
		}

		fmt.Printf("  ✓ %s\n", scSession.Name)
		converted++
	}

	fmt.Printf("\n✓ Converted: %d | ✗ Errors: %d\n", converted, errors)
	fmt.Printf("\nConverted sessions are saved in: %s\n", targetDir)
	fmt.Println("\nYou can now use 'xsc tui' to browse and connect to these sessions.")
}

func showHelp() {
	fmt.Println("xsc - XShell CLI - SSH Session Manager")
	fmt.Println()
	fmt.Println("Usage:")
	fmt.Println("  xsc                          Show this help message")
	fmt.Println("  xsc tui                      Launch TUI mode")
	fmt.Println("  xsc list                     List all sessions")
	fmt.Println("  xsc connect <path>           Connect to a session")
	fmt.Println("  xsc import-securecrt         Import SecureCRT sessions to local format")
	fmt.Println("  xsc help                     Show this help message")
	fmt.Println()
	fmt.Println("Examples:")
	fmt.Println("  xsc tui")
	fmt.Println("  xsc connect prod/db/master")
	fmt.Println("  xsc connect web-server")
	fmt.Println("  xsc import-securecrt")
	fmt.Println()
	fmt.Println("Session files are stored in: ~/.xsc/sessions/")
}

