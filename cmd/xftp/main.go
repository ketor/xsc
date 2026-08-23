package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path"
	"time"

	"github.com/pkg/sftp"

	"github.com/ketor/xsc/internal/mobaxterm"
	"github.com/ketor/xsc/internal/securecrt"
	"github.com/ketor/xsc/internal/session"
	"github.com/ketor/xsc/internal/shared"
	internalssh "github.com/ketor/xsc/internal/ssh"
	"github.com/ketor/xsc/internal/xftp"
	"github.com/ketor/xsc/internal/xshell"
	"github.com/ketor/xsc/pkg/config"
	"github.com/ketor/xsc/pkg/version"
)

func init() {
	// 注册密码解密器
	session.RegisterDecrypter("securecrt", session.DecrypterFunc(securecrt.DecryptPassword))
	session.RegisterDecrypter("xshell", session.DecrypterFunc(xshell.DecryptPassword))
	session.RegisterDecrypter("mobaxterm", session.DecrypterFunc(mobaxterm.DecryptPassword))
}

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(args []string) int {
	if len(args) == 0 {
		showHelp()
		return 0
	}
	command := args[0]
	commandArgs := args[1:]

	switch command {
	case "tui", "connect", "ls", "cat", "get", "put", "mkdir", "rm", "stat", "cp", "mv", "rename":
		cfg, err := config.LoadGlobalConfig()
		if err != nil {
			fmt.Fprintf(os.Stderr, "读取配置失败: %v\n", err)
			return 1
		}
		if err := shared.EnsureMasterPasswords(cfg); err != nil {
			fmt.Fprintf(os.Stderr, "读取主密码失败: %v\n", err)
			return 1
		}
	}

	requireArgs := func(minimum, maximum int, usage string) bool {
		_, positional := hasJSONFlag(commandArgs)
		if len(positional) < minimum || (maximum >= 0 && len(positional) > maximum) {
			fmt.Fprintln(os.Stderr, usage)
			return false
		}
		return true
	}

	switch command {
	case "tui":
		if len(commandArgs) != 0 {
			fmt.Fprintln(os.Stderr, "Usage: xftp tui")
			return 1
		}
		if err := xftp.Run(nil); err != nil {
			fmt.Fprintf(os.Stderr, "TUI 启动失败: %v\n", err)
			return 1
		}
		return 0
	case "connect":
		if !requireArgs(1, 1, "Usage: xftp connect <session_path>") {
			return 1
		}
		return connectAndRun(commandArgs[0])
	case "ls":
		if !requireArgs(1, 2, "Usage: xftp ls <session_path> [remote_path]") {
			return 1
		}
		return lsCommand(commandArgs)
	case "cat":
		if !requireArgs(2, 2, "Usage: xftp cat <session_path> <remote_path>") {
			return 1
		}
		return catCommand(commandArgs)
	case "get":
		if !requireArgs(3, 3, "Usage: xftp get <session_path> <remote_path> <local_path>") {
			return 1
		}
		return getCommand(commandArgs)
	case "put":
		if !requireArgs(3, 3, "Usage: xftp put <session_path> <local_path> <remote_path>") {
			return 1
		}
		return putCommand(commandArgs)
	case "mkdir":
		if !requireArgs(2, 2, "Usage: xftp mkdir <session_path> <remote_path>") {
			return 1
		}
		return mkdirCommand(commandArgs)
	case "rm":
		if !requireArgs(2, 2, "Usage: xftp rm <session_path> <remote_path>") {
			return 1
		}
		return rmCommand(commandArgs)
	case "stat":
		if !requireArgs(2, 2, "Usage: xftp stat <session_path> <remote_path>") {
			return 1
		}
		return statCommand(commandArgs)
	case "cp":
		if !requireArgs(3, 3, "Usage: xftp cp <session_path> <src_path> <dest_path>") {
			return 1
		}
		return cpCommand(commandArgs)
	case "mv", "rename":
		if !requireArgs(3, 3, "Usage: xftp mv <session_path> <src_path> <dest_path>") {
			return 1
		}
		return mvCommand(commandArgs)
	case "version", "--version", "-v":
		fmt.Println(version.String("xftp"))
		return 0
	case "help", "--help", "-h":
		showHelp()
		return 0
	default:
		if len(commandArgs) != 0 {
			fmt.Fprintf(os.Stderr, "Unknown command: %s\n", command)
			return 1
		}
		return connectAndRun(command)
	}
}

// connectAndRun 查找 session 并启动 SFTP 文件管理器
func connectAndRun(sessionPath string) int {
	s, err := shared.FindSessionAllSources(sessionPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "会话未找到: %s\n", sessionPath)
		return 1
	}

	if err := xftp.Run(s); err != nil {
		fmt.Fprintf(os.Stderr, "SFTP 会话失败: %v\n", err)
		return 1
	}
	return 0
}

// hasJSONFlag 检查参数列表中是否包含 --json 标志，并返回过滤后的参数
func hasJSONFlag(args []string) (bool, []string) {
	jsonOutput := false
	var filtered []string
	for _, a := range args {
		if a == "--json" {
			jsonOutput = true
		} else {
			filtered = append(filtered, a)
		}
	}
	return jsonOutput, filtered
}

// resolveSessionPassword 统一处理会话密码解析（委托到 shared 包）
var resolveSessionPassword = shared.ResolveSessionPassword

// connectSFTP 封装会话查找、密码解密、SSH 连接和 SFTP 客户端创建
func connectSFTP(sessionPath string) (*sftp.Client, func(), error) {
	s, err := shared.FindSessionAllSources(sessionPath)
	if err != nil {
		return nil, nil, fmt.Errorf("会话未找到: %s", sessionPath)
	}

	if err := resolveSessionPassword(s); err != nil {
		return nil, nil, fmt.Errorf("认证失败: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	sshClient, sshCleanup, err := internalssh.DialContext(ctx, s)
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

// lsCommand 列出远程目录内容
func lsCommand(args []string) int {
	jsonOutput, args := hasJSONFlag(args)
	sessionPath := args[0]
	remotePath := "."
	if len(args) > 1 {
		remotePath = args[1]
	}

	sftpClient, cleanup, err := connectSFTP(sessionPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		return 1
	}
	defer cleanup()

	entries, err := sftpClient.ReadDir(remotePath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "读取目录失败: %v\n", err)
		return 1
	}

	if jsonOutput {
		type fileInfo struct {
			Name    string `json:"name"`
			Size    int64  `json:"size"`
			Mode    string `json:"mode"`
			ModTime string `json:"mod_time"`
			IsDir   bool   `json:"is_dir"`
		}
		var files []fileInfo
		for _, e := range entries {
			files = append(files, fileInfo{
				Name:    e.Name(),
				Size:    e.Size(),
				Mode:    e.Mode().String(),
				ModTime: e.ModTime().Format(time.RFC3339),
				IsDir:   e.IsDir(),
			})
		}
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		enc.Encode(files)
	} else {
		for _, e := range entries {
			fmt.Printf("%s %10d %s %s\n",
				e.Mode().String(),
				e.Size(),
				e.ModTime().Format("2006-01-02 15:04"),
				e.Name(),
			)
		}
	}
	return 0
}

// catCommand 读取远程文件内容到标准输出
func catCommand(args []string) int {
	jsonOutput, args := hasJSONFlag(args)
	sessionPath := args[0]
	remotePath := args[1]

	sftpClient, cleanup, err := connectSFTP(sessionPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		return 1
	}
	defer cleanup()

	f, err := sftpClient.Open(remotePath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "打开文件失败: %v\n", err)
		return 1
	}
	defer f.Close()

	// 限制读取 1MB
	const maxSize = 1 << 20
	lr := io.LimitReader(f, maxSize+1)
	data, err := io.ReadAll(lr)
	if err != nil {
		fmt.Fprintf(os.Stderr, "读取文件失败: %v\n", err)
		return 1
	}

	if len(data) > maxSize {
		fmt.Fprintf(os.Stderr, "警告: 文件超过 1MB，仅显示前 1MB 内容\n")
		data = data[:maxSize]
	}

	if jsonOutput {
		result := map[string]interface{}{
			"path":    remotePath,
			"content": string(data),
			"size":    len(data),
		}
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		enc.Encode(result)
	} else {
		os.Stdout.Write(data)
	}
	return 0
}

// getCommand 下载远程文件到本地
func getCommand(args []string) int {
	jsonOutput, args := hasJSONFlag(args)
	sessionPath := args[0]
	remotePath := args[1]
	localPath := args[2]

	sftpClient, cleanup, err := connectSFTP(sessionPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		return 1
	}
	defer cleanup()

	remoteFile, err := sftpClient.Open(remotePath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "打开远程文件失败: %v\n", err)
		return 1
	}
	defer remoteFile.Close()

	localFile, err := xftp.NewAtomicLocalFile(localPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "创建本地临时文件失败: %v\n", err)
		return 1
	}
	defer localFile.Abort()

	n, err := io.Copy(localFile, remoteFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "下载失败: %v\n", err)
		return 1
	}
	mode := os.FileMode(0600)
	if info, statErr := remoteFile.Stat(); statErr == nil {
		mode = info.Mode().Perm()
	}
	if err := localFile.Commit(mode); err != nil {
		fmt.Fprintf(os.Stderr, "提交下载文件失败: %v\n", err)
		return 1
	}

	if jsonOutput {
		result := map[string]interface{}{
			"remote": remotePath,
			"local":  localPath,
			"size":   n,
		}
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		enc.Encode(result)
	} else {
		fmt.Printf("已下载: %s → %s (%d 字节)\n", remotePath, localPath, n)
	}
	return 0
}

// putCommand 上传本地文件到远程
func putCommand(args []string) int {
	jsonOutput, args := hasJSONFlag(args)
	sessionPath := args[0]
	localPath := args[1]
	remotePath := args[2]

	sftpClient, cleanup, err := connectSFTP(sessionPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		return 1
	}
	defer cleanup()

	localFile, err := os.Open(localPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "打开本地文件失败: %v\n", err)
		return 1
	}
	defer localFile.Close()
	mode := os.FileMode(0644)
	if info, statErr := localFile.Stat(); statErr == nil {
		mode = info.Mode().Perm()
	}

	remoteFile, err := xftp.NewAtomicRemoteFile(sftpClient, remotePath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "创建远程临时文件失败: %v\n", err)
		return 1
	}
	defer remoteFile.Abort()

	n, err := io.Copy(remoteFile, localFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "上传失败: %v\n", err)
		return 1
	}
	if err := remoteFile.Commit(mode); err != nil {
		fmt.Fprintf(os.Stderr, "提交上传文件失败: %v\n", err)
		return 1
	}

	if jsonOutput {
		result := map[string]interface{}{
			"local":  localPath,
			"remote": remotePath,
			"size":   n,
		}
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		enc.Encode(result)
	} else {
		fmt.Printf("已上传: %s → %s (%d 字节)\n", localPath, remotePath, n)
	}
	return 0
}

// mkdirCommand 创建远程目录
func mkdirCommand(args []string) int {
	jsonOutput, args := hasJSONFlag(args)
	sessionPath := args[0]
	remotePath := args[1]

	sftpClient, cleanup, err := connectSFTP(sessionPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		return 1
	}
	defer cleanup()

	if err := sftpClient.MkdirAll(remotePath); err != nil {
		fmt.Fprintf(os.Stderr, "创建目录失败: %v\n", err)
		return 1
	}

	if jsonOutput {
		result := map[string]interface{}{
			"path":    remotePath,
			"created": true,
		}
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		enc.Encode(result)
	} else {
		fmt.Printf("已创建目录: %s\n", remotePath)
	}
	return 0
}

// rmCommand 删除远程文件或目录（递归）
func rmCommand(args []string) int {
	jsonOutput, args := hasJSONFlag(args)
	sessionPath := args[0]
	remotePath := args[1]

	sftpClient, cleanup, err := connectSFTP(sessionPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		return 1
	}
	defer cleanup()

	// 检查是否为目录
	info, err := sftpClient.Stat(remotePath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "获取文件信息失败: %v\n", err)
		return 1
	}

	if info.IsDir() {
		if err := removeDir(sftpClient, remotePath); err != nil {
			fmt.Fprintf(os.Stderr, "删除目录失败: %v\n", err)
			return 1
		}
	} else {
		if err := sftpClient.Remove(remotePath); err != nil {
			fmt.Fprintf(os.Stderr, "删除文件失败: %v\n", err)
			return 1
		}
	}

	if jsonOutput {
		result := map[string]interface{}{
			"path":    remotePath,
			"deleted": true,
		}
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		enc.Encode(result)
	} else {
		fmt.Printf("已删除: %s\n", remotePath)
	}
	return 0
}

// statCommand 获取远程文件/目录详细信息
func statCommand(args []string) int {
	jsonOutput, args := hasJSONFlag(args)
	sessionPath := args[0]
	remotePath := args[1]

	sftpClient, cleanup, err := connectSFTP(sessionPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		return 1
	}
	defer cleanup()

	info, err := sftpClient.Stat(remotePath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "获取文件信息失败: %v\n", err)
		return 1
	}

	if jsonOutput {
		type StatResult struct {
			Path  string `json:"path"`
			Size  int64  `json:"size"`
			Mode  string `json:"mode"`
			IsDir bool   `json:"is_dir"`
			MTime string `json:"mtime"`
		}
		result := StatResult{
			Path:  remotePath,
			Size:  info.Size(),
			Mode:  fmt.Sprintf("%04o", info.Mode().Perm()),
			IsDir: info.IsDir(),
			MTime: info.ModTime().Format(time.RFC3339),
		}
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		enc.Encode(result)
	} else {
		fmt.Printf("路径:     %s\n", remotePath)
		fmt.Printf("大小:     %d 字节\n", info.Size())
		fmt.Printf("权限:     %04o\n", info.Mode().Perm())
		fmt.Printf("类型:     %s\n", map[bool]string{true: "目录", false: "文件"}[info.IsDir()])
		fmt.Printf("修改时间: %s\n", info.ModTime().Format(time.RFC3339))
	}
	return 0
}

// cpCommand 复制远程文件
func cpCommand(args []string) int {
	jsonOutput, args := hasJSONFlag(args)
	sessionPath := args[0]
	srcPath := args[1]
	dstPath := args[2]

	sftpClient, cleanup, err := connectSFTP(sessionPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		return 1
	}
	defer cleanup()

	// 打开源文件
	srcFile, err := sftpClient.Open(srcPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "打开源文件失败: %v\n", err)
		return 1
	}
	defer srcFile.Close()

	mode := os.FileMode(0644)
	if info, statErr := srcFile.Stat(); statErr == nil {
		mode = info.Mode().Perm()
	}
	dstFile, err := xftp.NewAtomicRemoteFile(sftpClient, dstPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "创建目标临时文件失败: %v\n", err)
		return 1
	}
	defer dstFile.Abort()

	n, err := io.Copy(dstFile, srcFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "复制失败: %v\n", err)
		return 1
	}
	if err := dstFile.Commit(mode); err != nil {
		fmt.Fprintf(os.Stderr, "提交目标文件失败: %v\n", err)
		return 1
	}

	if jsonOutput {
		type FileOpResult struct {
			Src    string `json:"src"`
			Dst    string `json:"dst"`
			Size   int64  `json:"size"`
			Status string `json:"status"`
		}
		result := FileOpResult{
			Src:    srcPath,
			Dst:    dstPath,
			Size:   n,
			Status: "copied",
		}
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		enc.Encode(result)
	} else {
		fmt.Printf("已复制: %s → %s (%d 字节)\n", srcPath, dstPath, n)
	}
	return 0
}

// mvCommand 移动/重命名远程文件
func mvCommand(args []string) int {
	jsonOutput, args := hasJSONFlag(args)
	sessionPath := args[0]
	srcPath := args[1]
	dstPath := args[2]

	sftpClient, cleanup, err := connectSFTP(sessionPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		return 1
	}
	defer cleanup()

	// 执行重命名
	if err := sftpClient.Rename(srcPath, dstPath); err != nil {
		fmt.Fprintf(os.Stderr, "移动/重命名失败: %v\n", err)
		return 1
	}

	if jsonOutput {
		type FileOpResult struct {
			Src    string `json:"src"`
			Dst    string `json:"dst"`
			Status string `json:"status"`
		}
		result := FileOpResult{
			Src:    srcPath,
			Dst:    dstPath,
			Status: "moved",
		}
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		enc.Encode(result)
	} else {
		action := "移动"
		if path.Dir(srcPath) == path.Dir(dstPath) {
			action = "重命名"
		}
		fmt.Printf("已%s: %s → %s\n", action, srcPath, dstPath)
	}
	return 0
}

// removeDir 递归删除远程目录
func removeDir(client *sftp.Client, dirPath string) error {
	entries, err := client.ReadDir(dirPath)
	if err != nil {
		return fmt.Errorf("读取目录失败: %w", err)
	}

	for _, entry := range entries {
		fullPath := path.Join(dirPath, entry.Name())
		if entry.IsDir() {
			if err := removeDir(client, fullPath); err != nil {
				return err
			}
		} else {
			if err := client.Remove(fullPath); err != nil {
				return fmt.Errorf("删除文件 %s 失败: %w", fullPath, err)
			}
		}
	}

	return client.RemoveDirectory(dirPath)
}

func showHelp() {
	fmt.Println("xftp - TUI SFTP 文件管理器")
	fmt.Println()
	fmt.Println("Usage:")
	fmt.Println("  xftp                          显示帮助信息")
	fmt.Println("  xftp tui                      启动 TUI 模式")
	fmt.Println("  xftp <session-path>           连接到指定会话并打开 SFTP")
	fmt.Println("  xftp connect <session-path>   连接到指定会话并打开 SFTP")
	fmt.Println()
	fmt.Println("  xftp ls <path> [remote_path]  列出远程目录")
	fmt.Println("  xftp cat <path> <remote>      读取远程文件内容")
	fmt.Println("  xftp stat <path> <remote>     获取文件/目录详细信息")
	fmt.Println("  xftp get <path> <r> <l>       下载远程文件到本地")
	fmt.Println("  xftp put <path> <l> <r>       上传本地文件到远程")
	fmt.Println("  xftp cp <path> <src> <dst>    复制远程文件")
	fmt.Println("  xftp mv <path> <src> <dst>    移动远程文件")
	fmt.Println("  xftp rename <path> <old> <new> 重命名远程文件")
	fmt.Println("  xftp mkdir <path> <remote>    创建远程目录")
	fmt.Println("  xftp rm <path> <remote>       删除远程文件/目录")
	fmt.Println()
	fmt.Println("  xftp version                  显示版本信息")
	fmt.Println("  xftp help                     显示帮助信息")
	fmt.Println()
	fmt.Println("所有命令支持 --json 参数输出 JSON 格式")
	fmt.Println()
	fmt.Println("Examples:")
	fmt.Println("  xftp tui")
	fmt.Println("  xftp prod/db/master")
	fmt.Println("  xftp connect web-server")
	fmt.Println("  xftp ls prod/db/master /var/log")
	fmt.Println("  xftp stat prod/db/master /etc/hosts")
	fmt.Println("  xftp get prod/db/master /etc/hosts ./hosts")
	fmt.Println("  xftp put prod/db/master ./config.yaml /etc/app/config.yaml")
	fmt.Println("  xftp cp prod/db/master /etc/hosts /etc/hosts.bak")
	fmt.Println("  xftp mv prod/db/master /tmp/old.log /var/log/new.log")
	fmt.Println("  xftp rename prod/db/master /tmp/file.txt /tmp/newfile.txt")
	fmt.Println()
	fmt.Println("会话文件存储在: ~/.xsc/sessions/")
}
