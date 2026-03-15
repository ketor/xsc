package main

import (
	"fmt"
	"os"

	"github.com/ketor/xsc/internal/session"
	"github.com/ketor/xsc/internal/xftp"
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
		// TUI 模式：启动会话选择器
		if err := xftp.Run(nil); err != nil {
			fmt.Fprintf(os.Stderr, "TUI 启动失败: %v\n", err)
			os.Exit(1)
		}
	case "connect":
		if len(os.Args) < 3 {
			fmt.Fprintln(os.Stderr, "Usage: xftp connect <session_path>")
			os.Exit(1)
		}
		connectAndRun(os.Args[2])
	case "version", "--version", "-v":
		fmt.Println(version.String("xftp"))
	case "help", "--help", "-h":
		showHelp()
	default:
		// 默认当作 session path 直连
		connectAndRun(command)
	}
}

// connectAndRun 查找 session 并启动 SFTP 文件管理器
func connectAndRun(sessionPath string) {
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

	if err := xftp.Run(s); err != nil {
		fmt.Fprintf(os.Stderr, "SFTP 会话失败: %v\n", err)
		os.Exit(1)
	}
}

func showHelp() {
	fmt.Println("xftp - TUI SFTP 文件管理器")
	fmt.Println()
	fmt.Println("Usage:")
	fmt.Println("  xftp                          显示帮助信息")
	fmt.Println("  xftp tui                      启动 TUI 模式")
	fmt.Println("  xftp <session-path>           连接到指定会话并打开 SFTP")
	fmt.Println("  xftp connect <session-path>   连接到指定会话并打开 SFTP")
	fmt.Println("  xftp version                  显示版本信息")
	fmt.Println("  xftp help                     显示帮助信息")
	fmt.Println()
	fmt.Println("Examples:")
	fmt.Println("  xftp tui")
	fmt.Println("  xftp prod/db/master")
	fmt.Println("  xftp connect web-server")
	fmt.Println()
	fmt.Println("会话文件存储在: ~/.xsc/sessions/")
}
