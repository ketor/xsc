package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

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
		return cli.ExitOK
	}
	command := args[0]
	commandArgs := args[1:]

	switch command {
	case "tui", "connect", "exec", "ping", "import-securecrt", "import-xshell", "import-mobaxterm":
		cfg, err := config.LoadGlobalConfig()
		if err != nil {
			fmt.Fprintf(os.Stderr, "读取配置失败: %v\n", err)
			return cli.ExitConfig
		}
		if err := shared.EnsureMasterPasswords(cfg); err != nil {
			fmt.Fprintf(os.Stderr, "读取主密码失败: %v\n", err)
			return cli.ExitConfig
		}
	}

	switch command {
	case "tui":
		if err := tui.Run(); err != nil {
			fmt.Fprintln(os.Stderr, err)
			return cli.ExitFileOp
		}
		return cli.ExitOK
	case "list":
		params := ListParams{}
		for _, arg := range commandArgs {
			if arg == "--json" {
				params.JSON = true
			} else {
				fmt.Fprintf(os.Stderr, "unknown option: %s\n", arg)
				return cli.ExitUsage
			}
		}
		return handleList(context.Background(), params, cli.NewPrinter(params.JSON, os.Stdout, os.Stderr))
	case "add":
		params := parseAddArgs(commandArgs)
		return handleAdd(context.Background(), params, cli.NewPrinter(params.JSON, os.Stdout, os.Stderr))
	case "show":
		params := parseShowArgs(commandArgs)
		return handleShow(context.Background(), params, cli.NewPrinter(params.JSON, os.Stdout, os.Stderr))
	case "edit":
		params := parseEditArgs(commandArgs)
		return handleEdit(context.Background(), params, cli.NewPrinter(params.JSON, os.Stdout, os.Stderr))
	case "delete":
		params := parseDeleteArgs(commandArgs)
		return handleDelete(context.Background(), params, cli.NewPrinter(params.JSON, os.Stdout, os.Stderr))
	case "connect":
		if len(commandArgs) != 1 {
			fmt.Fprintln(os.Stderr, "Usage: xssh connect <session_path>")
			return cli.ExitUsage
		}
		return connectSession(commandArgs[0])
	case "ping":
		if len(commandArgs) == 0 {
			fmt.Fprintln(os.Stderr, "Usage: xssh ping <path>[,path2,...] [--json] [--timeout 10s] [--parallel 5]")
			return cli.ExitUsage
		}
		params := parsePingArgs(commandArgs)
		return handlePing(context.Background(), params, cli.NewPrinter(params.JSON, os.Stdout, os.Stderr))
	case "exec":
		if len(commandArgs) < 2 {
			fmt.Fprintln(os.Stderr, "Usage: xssh exec <session_path> [options] <command>")
			return cli.ExitUsage
		}
		if strings.Contains(commandArgs[0], ",") {
			params := parseExecMultiArgs(commandArgs[0], commandArgs[1:])
			return handleExecMulti(context.Background(), params, cli.NewPrinter(params.JSON, os.Stdout, os.Stderr))
		}
		return execCommand(commandArgs[0], commandArgs[1:])
	case "import-securecrt":
		return convertSecureCRT(commandArgs)
	case "import-xshell":
		return convertXShell(commandArgs)
	case "import-mobaxterm":
		return convertMobaXterm(commandArgs)
	case "version", "--version", "-v":
		fmt.Println(version.String("xssh"))
		return cli.ExitOK
	case "help", "--help", "-h":
		showHelp()
		return cli.ExitOK
	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n", command)
		showHelp()
		return cli.ExitUsage
	}
}

// resolveSessionPassword 统一处理会话密码解析（委托到 shared 包）
var resolveSessionPassword = shared.ResolveSessionPassword

func connectSession(sessionPath string) int {
	s, err := shared.FindSessionAllSources(sessionPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Session lookup failed: %v\n", err)
		return cli.ExitNotFound
	}
	if err := resolveSessionPassword(s); err != nil {
		fmt.Fprintf(os.Stderr, "认证失败: %v\n", err)
		return cli.ExitAuthFailed
	}
	if err := internalssh.Connect(s); err != nil {
		fmt.Fprintf(os.Stderr, "Connection failed: %v\n", err)
		return cli.ExitConnFailed
	}
	return cli.ExitOK
}

// execCommand 在远程主机上执行命令。
func execCommand(sessionPath string, args []string) int {
	timeout := 30
	jsonOutput := false
	var cmdArgs []string
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "-t", "--timeout":
			if i+1 >= len(args) {
				fmt.Fprintln(os.Stderr, "错误: -t/--timeout 需要指定超时秒数")
				return cli.ExitUsage
			}
			i++
			value, err := strconv.Atoi(args[i])
			if err != nil || value <= 0 {
				fmt.Fprintln(os.Stderr, "错误: 超时秒数必须为正整数")
				return cli.ExitUsage
			}
			if value > 300 {
				value = 300
			}
			timeout = value
		case "--json":
			jsonOutput = true
		default:
			cmdArgs = append(cmdArgs, args[i])
		}
	}

	p := cli.NewPrinter(jsonOutput, os.Stdout, os.Stderr)
	if len(cmdArgs) == 0 {
		p.PrintErr(cli.NewCLIError(cli.ExitUsage, "未指定要执行的命令", ""))
		return cli.ExitUsage
	}
	s, err := shared.FindSessionAllSources(sessionPath)
	if err != nil {
		p.PrintErr(cli.NewCLIError(cli.ExitNotFound, "会话查找失败", err.Error()))
		return cli.ExitNotFound
	}
	if err := resolveSessionPassword(s); err != nil {
		p.PrintErr(cli.NewCLIError(cli.ExitAuthFailed, "认证失败", err.Error()))
		return cli.ExitAuthFailed
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeout)*time.Second)
	defer cancel()
	result, runErr := internalssh.RunCommand(ctx, s, strings.Join(cmdArgs, " "), internalssh.DefaultMaxOutputBytes)
	exitCode := result.ExitCode
	if runErr != nil {
		if ctx.Err() != nil {
			exitCode = cli.ExitTimeout
		} else {
			exitCode = cli.ExitConnFailed
		}
	}

	if p.IsJSON() {
		output := map[string]interface{}{
			"stdout":           result.Stdout,
			"stderr":           result.Stderr,
			"exit_code":        exitCode,
			"stdout_truncated": result.StdoutTruncated,
			"stderr_truncated": result.StderrTruncated,
		}
		if runErr != nil {
			output["error"] = runErr.Error()
		}
		p.Print(output)
	} else {
		fmt.Fprint(os.Stdout, result.Stdout)
		fmt.Fprint(os.Stderr, result.Stderr)
		if result.StdoutTruncated {
			fmt.Fprintln(os.Stderr, "xsc: stdout 已截断")
		}
		if result.StderrTruncated {
			fmt.Fprintln(os.Stderr, "xsc: stderr 已截断")
		}
		if runErr != nil && result.Stderr == "" {
			fmt.Fprintln(os.Stderr, runErr)
		}
	}
	return exitCode
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

func convertSessions(src importSource) int {
	if !src.enabled {
		fmt.Fprintf(os.Stderr, "%s is not enabled in config\n", src.name)
		return cli.ExitConfig
	}
	sessions, err := src.loadAndConvert()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading %s sessions: %v\n", src.name, err)
		return cli.ExitConfig
	}
	if len(sessions) == 0 {
		fmt.Fprintf(os.Stderr, "No %s sessions found\n", src.name)
		return cli.ExitOK
	}
	sessionsDir, err := config.GetSessionsDir()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error getting sessions directory: %v\n", err)
		return cli.ExitConfig
	}
	timestamp := time.Now().Format("20060102-150405")
	targetDir := filepath.Join(sessionsDir, src.dirPrefix, timestamp)
	if err := os.MkdirAll(targetDir, 0700); err != nil {
		fmt.Fprintf(os.Stderr, "Error creating target directory: %v\n", err)
		return cli.ExitFileOp
	}

	fmt.Fprintf(os.Stderr, "Converting %d %s sessions...\n", len(sessions), src.name)
	fmt.Fprintf(os.Stderr, "Target directory: %s\n\n", targetDir)
	converted := 0
	errorCount := 0
	for _, imported := range sessions {
		if imported.DecryptErr != nil {
			if src.skipDecryptErrors {
				fmt.Fprintf(os.Stderr, "  ⚠ %s: 解密跳过: %v\n", imported.Name, imported.DecryptErr)
			} else {
				fmt.Fprintf(os.Stderr, "  ✗ %s: 解密失败: %v\n", imported.Name, imported.DecryptErr)
				errorCount++
				continue
			}
		}

		xsshSession := buildXSSHSessionFromImport(imported)
		relativePath := imported.Name
		if imported.Folder != "" {
			relativePath = filepath.Join(imported.Folder, imported.Name)
		}
		targetPath, err := session.ResolveSessionFile(targetDir, relativePath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "  ✗ %s: unsafe import path: %v\n", imported.Name, err)
			errorCount++
			continue
		}
		if err := session.SaveSession(xsshSession, targetPath); err != nil {
			fmt.Fprintf(os.Stderr, "  ✗ %s: %v\n", imported.Name, err)
			errorCount++
			continue
		}
		fmt.Fprintf(os.Stderr, "  ✓ %s\n", imported.Name)
		converted++
	}
	fmt.Fprintf(os.Stderr, "\n✓ Converted: %d | ✗ Errors: %d\n", converted, errorCount)
	fmt.Fprintf(os.Stderr, "\nConverted sessions are saved in: %s\n", targetDir)
	fmt.Fprintln(os.Stderr, "\nYou can now use 'xssh tui' to browse and connect to these sessions.")
	if errorCount == 0 {
		return cli.ExitOK
	}
	if converted == 0 {
		return cli.ExitConfig
	}
	return cli.ExitPartial
}

func parseImportArgs(args []string) bool {
	for _, arg := range args {
		if arg == "--skip-decrypt-errors" {
			return true
		}
	}
	return false
}

// sessionFlags 解析 --host/--port/--user/--auth-type/--password/--key/--json 等通用 flag
type sessionFlags struct {
	Path, Host, User, AuthType, Password, KeyPath, ProxyJump string
	Port                                                     int
	Force                                                    bool
	JSON                                                     bool
	Err                                                      error
}

func parseSessionFlags(args []string) sessionFlags {
	var result sessionFlags
	for index := 0; index < len(args); index++ {
		arg := args[index]
		nextValue := func() (string, bool) {
			if index+1 >= len(args) {
				result.Err = fmt.Errorf("%s requires a value", arg)
				return "", false
			}
			index++
			return args[index], true
		}
		switch arg {
		case "--host", "--user", "--auth-type", "--password", "--key", "--proxy-jump":
			value, ok := nextValue()
			if !ok {
				return result
			}
			switch arg {
			case "--host":
				result.Host = value
			case "--user":
				result.User = value
			case "--auth-type":
				result.AuthType = value
			case "--password":
				result.Password = value
			case "--key":
				result.KeyPath = value
			case "--proxy-jump":
				result.ProxyJump = value
			}
		case "--port":
			value, ok := nextValue()
			if !ok {
				return result
			}
			port, err := strconv.Atoi(value)
			if err != nil || port < 1 || port > 65535 {
				result.Err = fmt.Errorf("--port must be between 1 and 65535")
				return result
			}
			result.Port = port
		case "--force":
			result.Force = true
		case "--json":
			result.JSON = true
		default:
			if strings.HasPrefix(arg, "-") {
				result.Err = fmt.Errorf("unknown option: %s", arg)
				return result
			}
			if result.Path != "" {
				result.Err = fmt.Errorf("unexpected argument: %s", arg)
				return result
			}
			result.Path = arg
		}
	}
	return result
}

// parseAddArgs 解析 add 命令参数
func parseAddArgs(args []string) AddParams {
	f := parseSessionFlags(args)
	return AddParams{
		Path: f.Path, Host: f.Host, Port: f.Port, User: f.User,
		AuthType: f.AuthType, Password: f.Password, KeyPath: f.KeyPath,
		ProxyJump: f.ProxyJump, Force: f.Force, JSON: f.JSON, ParseErr: f.Err,
	}
}

// parseShowArgs 解析 show 命令参数
func parseShowArgs(args []string) ShowParams {
	var params ShowParams
	for _, arg := range args {
		switch {
		case arg == "--json":
			params.JSON = true
		case strings.HasPrefix(arg, "-"):
			params.ParseErr = fmt.Errorf("unknown option: %s", arg)
			return params
		case params.Path == "":
			params.Path = arg
		default:
			params.ParseErr = fmt.Errorf("unexpected argument: %s", arg)
			return params
		}
	}
	return params
}

// parseEditArgs 解析 edit 命令参数
func parseEditArgs(args []string) EditParams {
	f := parseSessionFlags(args)
	return EditParams{
		Path: f.Path, Host: f.Host, Port: f.Port, User: f.User,
		AuthType: f.AuthType, Password: f.Password, KeyPath: f.KeyPath,
		ProxyJump: f.ProxyJump, JSON: f.JSON, ParseErr: f.Err,
	}
}

// parseDeleteArgs 解析 delete 命令参数
func parseDeleteArgs(args []string) DeleteParams {
	var params DeleteParams
	for _, arg := range args {
		switch {
		case arg == "--force":
			params.Force = true
		case arg == "--json":
			params.JSON = true
		case strings.HasPrefix(arg, "-"):
			params.ParseErr = fmt.Errorf("unknown option: %s", arg)
			return params
		case params.Path == "":
			params.Path = arg
		default:
			params.ParseErr = fmt.Errorf("unexpected argument: %s", arg)
			return params
		}
	}
	return params
}

func doImport(args []string, buildSource func(gc *config.GlobalConfig, skipDecrypt bool) importSource) int {
	skipDecrypt := parseImportArgs(args)
	globalConfig, err := config.LoadGlobalConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading global config: %v\n", err)
		return cli.ExitConfig
	}
	return convertSessions(buildSource(globalConfig, skipDecrypt))
}

func convertSecureCRT(args []string) int {
	return doImport(args, func(gc *config.GlobalConfig, skipDecrypt bool) importSource {
		return importSource{
			name:              "SecureCRT",
			dirPrefix:         "securecrt-converted",
			enabled:           gc.SecureCRT.Enabled,
			skipDecryptErrors: skipDecrypt,
			loadAndConvert: func() ([]importSession, error) {
				scConfig := securecrt.Config{
					SessionPath: gc.SecureCRT.SessionPath,
					Password:    gc.SecureCRT.Password,
				}
				scSessions, err := securecrt.LoadSessions(scConfig)
				if err != nil {
					return nil, err
				}
				raw := make([]rawImportSession, len(scSessions))
				for i, s := range scSessions {
					raw[i] = rawImportSession{Name: s.Name, Folder: s.Folder, Password: s.Password, EncryptedPassword: s.EncryptedPassword, SessionData: s.ConvertToXSSHSession()}
				}
				return buildImportSessions(raw, gc.SecureCRT.Password, func(enc, master string) (string, error) { return securecrt.DecryptPassword(enc, master) }), nil
			},
		}
	})
}

func convertXShell(args []string) int {
	return doImport(args, func(gc *config.GlobalConfig, skipDecrypt bool) importSource {
		return importSource{
			name:              "Xshell",
			dirPrefix:         "xshell-converted",
			enabled:           gc.XShell.Enabled,
			skipDecryptErrors: skipDecrypt,
			loadAndConvert: func() ([]importSession, error) {
				xsConfig := xshell.Config{
					SessionPath: gc.XShell.SessionPath,
					Password:    gc.XShell.Password,
				}
				xsSessions, err := xshell.LoadSessions(xsConfig)
				if err != nil {
					return nil, err
				}
				raw := make([]rawImportSession, len(xsSessions))
				for i, s := range xsSessions {
					raw[i] = rawImportSession{Name: s.Name, Folder: s.Folder, Password: s.Password, EncryptedPassword: s.EncryptedPassword, SessionData: s.ConvertToXSSHSession()}
				}
				return buildImportSessions(raw, gc.XShell.Password, func(enc, master string) (string, error) { return xshell.DecryptPassword(enc, master) }), nil
			},
		}
	})
}

func convertMobaXterm(args []string) int {
	return doImport(args, func(gc *config.GlobalConfig, skipDecrypt bool) importSource {
		return importSource{
			name:              "MobaXterm",
			dirPrefix:         "mobaxterm-converted",
			enabled:           gc.MobaXterm.Enabled,
			skipDecryptErrors: skipDecrypt,
			loadAndConvert: func() ([]importSession, error) {
				mxConfig := mobaxterm.Config{
					SessionPath: gc.MobaXterm.SessionPath,
					Password:    gc.MobaXterm.Password,
				}
				mxSessions, err := mobaxterm.LoadSessions(mxConfig)
				if err != nil {
					return nil, err
				}
				raw := make([]rawImportSession, len(mxSessions))
				for i, s := range mxSessions {
					raw[i] = rawImportSession{Name: s.Name, Folder: s.Folder, Password: s.Password, EncryptedPassword: s.EncryptedPassword, SessionData: s.ConvertToXSSHSession()}
				}
				return buildImportSessions(raw, gc.MobaXterm.Password, func(enc, master string) (string, error) { return mobaxterm.DecryptPassword(enc, master) }), nil
			},
		}
	})
}

// rawImportSession 是三种导入源的公共中间表示
type rawImportSession struct {
	Name, Folder, Password, EncryptedPassword string
	SessionData                               map[string]interface{}
}

// buildXSSHSessionFromImport 从外部源（SecureCRT/XShell/MobaXterm）的导入数据构造完整的
// xssh Session 对象。需要保留：基础字段、密码（解密后明文）、key_path、完整 AuthMethods 列表。
//
// 历史回归（commit ceeb45b, 2026-03）只搬了 Host/Port/User/AuthType/Password，丢掉了
// AuthMethods 和 KeyPath，导致导入后的 session 在 TUI 里：
//   - 多种认证方式只显示一种（无 AuthMethods）
//   - 无法显示 publickey 路径
//   - 在某些场景下密码也丢失
func buildXSSHSessionFromImport(s importSession) *session.Session {
	sessionData := s.SessionData

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

	if pwd, ok := sessionData["password"].(string); ok && pwd != "" {
		xsshSession.Password = pwd
	} else if s.Password != "" {
		xsshSession.Password = s.Password
	}

	if kp, ok := sessionData["key_path"].(string); ok && kp != "" {
		xsshSession.KeyPath = kp
	}

	// AuthMethods 多认证列表 — SecureCRT 必需，否则 TUI 只显示一种认证方式
	if authMethods, ok := sessionData["auth_methods"]; ok {
		if amList, ok := authMethods.([]securecrt.AuthMethod); ok {
			for _, am := range amList {
				method := session.AuthMethod{
					Type:     am.Type,
					Priority: am.Priority,
					KeyPath:  am.KeyFile,
				}
				// SecureCRT.AuthMethod.Password 字段存的是加密密文（命名混淆，见
				// securecrt/parser.go ConvertToXSSHSession）。导入产物落本地 YAML 后没有
				// master password 可用，因此把 buildImportSessions 已解密的明文写到
				// AuthMethod.Password，让 TUI 能正常显示。
				if am.Type == "password" && s.Password != "" && am.Password != "" {
					method.Password = s.Password
				}
				xsshSession.AuthMethods = append(xsshSession.AuthMethods, method)
			}
		}
	}

	return xsshSession
}

// buildImportSessions 将原始导入会话列表转换为 importSession 列表，统一处理解密逻辑
func buildImportSessions(sessions []rawImportSession, masterPassword string, decryptFn func(string, string) (string, error)) []importSession {
	var result []importSession
	for _, s := range sessions {
		var decryptErr error
		if s.EncryptedPassword != "" && masterPassword != "" {
			pwd, err := decryptFn(s.EncryptedPassword, masterPassword)
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
			SessionData: s.SessionData,
			DecryptErr:  decryptErr,
		})
	}
	return result
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
