package ssh

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/ketor/xsc/internal/mobaxterm"
	"github.com/ketor/xsc/internal/securecrt"
	"github.com/ketor/xsc/internal/session"
	"github.com/ketor/xsc/internal/xshell"
	"github.com/ketor/xsc/pkg/config"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
	"golang.org/x/crypto/ssh/knownhosts"
	"golang.org/x/term"
)

// Connect 连接到 SSH 会话
func Connect(s *session.Session) error {
	if !s.Valid {
		return fmt.Errorf("invalid session: %v", s.Error)
	}

	// 如果有多种认证方式配置，按顺序尝试
	if len(s.AuthMethods) > 0 {
		return connectWithMultipleAuth(s)
	}

	// 延迟解密密码（SecureCRT 会话）
	if s.AuthType == session.AuthTypePassword && s.Password == "" && s.EncryptedPassword != "" {
		if err := s.ResolvePassword(); err != nil {
			return fmt.Errorf("failed to resolve password: %w", err)
		}
	}

	switch s.AuthType {
	case session.AuthTypePassword, session.AuthTypeKey, session.AuthTypeAgent:
		return connectSingle(s)
	default:
		return fmt.Errorf("unsupported auth type: %s", s.AuthType)
	}
}

// Dial 建立 SSH 客户端连接（不创建交互式会话）
// 返回 *ssh.Client 和可选的 cleanup 函数（用于关闭 SSH Agent 连接）
// 调用方负责关闭 client，并在关闭后调用 cleanup（如果非 nil）
func Dial(s *session.Session) (*ssh.Client, func(), error) {
	if !s.Valid {
		return nil, nil, fmt.Errorf("invalid session: %v", s.Error)
	}

	// 如果有多种认证方式配置，按顺序尝试
	if len(s.AuthMethods) > 0 {
		return dialWithMultipleAuth(s)
	}

	// 延迟解密密码
	if s.AuthType == session.AuthTypePassword && s.Password == "" && s.EncryptedPassword != "" {
		if err := s.ResolvePassword(); err != nil {
			return nil, nil, fmt.Errorf("failed to resolve password: %w", err)
		}
	}

	config, cleanup, err := getSSHConfig(s)
	if err != nil {
		return nil, nil, err
	}

	addr := fmt.Sprintf("%s:%d", s.Host, s.Port)
	conn, err := net.DialTimeout("tcp", addr, 10*time.Second)
	if err != nil {
		if cleanup != nil {
			cleanup()
		}
		return nil, nil, fmt.Errorf("connection timeout: %w", err)
	}

	c, chans, reqs, err := ssh.NewClientConn(conn, addr, config)
	if err != nil {
		conn.Close()
		if cleanup != nil {
			cleanup()
		}
		return nil, nil, fmt.Errorf("failed to create SSH connection: %w", err)
	}

	client := ssh.NewClient(c, chans, reqs)

	// 启动 keepalive goroutine，使用 context 控制生命周期
	keepaliveCtx, keepaliveCancel := context.WithCancel(context.Background())
	go startKeepalive(keepaliveCtx, client)

	// 将 keepalive 取消和原有 cleanup 合并
	combinedCleanup := func() {
		keepaliveCancel()
		if cleanup != nil {
			cleanup()
		}
	}

	return client, combinedCleanup, nil
}

// startKeepalive 启动 keepalive 心跳，监听 context 取消
func startKeepalive(ctx context.Context, client *ssh.Client) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			_, _, err := client.SendRequest("keepalive@openssh.com", true, nil)
			if err != nil {
				// 尽力而为的心跳机制：连接断开时静默退出，由上层处理连接生命周期
				return
			}
		}
	}
}

// dialWithMultipleAuth 按顺序尝试多种认证方式建立连接（不创建交互式会话）
func dialWithMultipleAuth(s *session.Session) (*ssh.Client, func(), error) {
	var lastErr error

	for i, authMethod := range s.AuthMethods {
		// 延迟解密密码（如果需要）
		if authMethod.Type == "password" && authMethod.Password == "" && authMethod.EncryptedPassword != "" {
			var decrypted string
			var err error
			switch s.PasswordSource {
			case "xshell":
				decrypted, err = xshell.DecryptPassword(authMethod.EncryptedPassword, s.MasterPassword)
			case "mobaxterm":
				decrypted, err = mobaxterm.DecryptPassword(authMethod.EncryptedPassword, s.MasterPassword)
			default:
				decrypted, err = securecrt.DecryptPassword(authMethod.EncryptedPassword, s.MasterPassword)
			}
			if err != nil {
				lastErr = fmt.Errorf("auth method %d (%s): failed to decrypt password: %w", i+1, authMethod.Type, err)
				continue
			}
			authMethod.Password = decrypted
			s.AuthMethods[i].Password = decrypted
		}

		config, cleanup, err := getSSHConfigForAuthMethod(s, authMethod)
		if err != nil {
			lastErr = fmt.Errorf("auth method %d (%s): %w", i+1, authMethod.Type, err)
			continue
		}

		addr := fmt.Sprintf("%s:%d", s.Host, s.Port)
		conn, err := net.DialTimeout("tcp", addr, 10*time.Second)
		if err != nil {
			if cleanup != nil {
				cleanup()
			}
			lastErr = fmt.Errorf("auth method %d (%s): connection timeout: %w", i+1, authMethod.Type, err)
			continue
		}

		c, chans, reqs, err := ssh.NewClientConn(conn, addr, config)
		if err != nil {
			conn.Close()
			if cleanup != nil {
				cleanup()
			}
			lastErr = fmt.Errorf("auth method %d (%s): %w", i+1, authMethod.Type, err)
			continue
		}

		client := ssh.NewClient(c, chans, reqs)

		// 启动 keepalive goroutine，使用 context 控制生命周期
		keepaliveCtx, keepaliveCancel := context.WithCancel(context.Background())
		go startKeepalive(keepaliveCtx, client)

		// 将 keepalive 取消和原有 cleanup 合并
		combinedCleanup := func() {
			keepaliveCancel()
			if cleanup != nil {
				cleanup()
			}
		}

		return client, combinedCleanup, nil
	}

	if lastErr != nil {
		return nil, nil, fmt.Errorf("all authentication methods failed: %w", lastErr)
	}
	return nil, nil, fmt.Errorf("all authentication methods failed")
}

// connectWithMultipleAuth 按顺序尝试多种认证方式
func connectWithMultipleAuth(s *session.Session) error {
	var lastErr error
	var cleanups []func()
	defer func() {
		for _, cleanup := range cleanups {
			cleanup()
		}
	}()

	for i, authMethod := range s.AuthMethods {
		// 延迟解密密码（如果需要）
		if authMethod.Type == "password" && authMethod.Password == "" && authMethod.EncryptedPassword != "" {
			// 根据密码来源选择解密器
			var decrypted string
			var err error
			switch s.PasswordSource {
			case "xshell":
				decrypted, err = xshell.DecryptPassword(authMethod.EncryptedPassword, s.MasterPassword)
			case "mobaxterm":
				decrypted, err = mobaxterm.DecryptPassword(authMethod.EncryptedPassword, s.MasterPassword)
			default:
				decrypted, err = securecrt.DecryptPassword(authMethod.EncryptedPassword, s.MasterPassword)
			}
			if err != nil {
				lastErr = fmt.Errorf("auth method %d (%s): failed to decrypt password: %w", i+1, authMethod.Type, err)
				continue
			}
			authMethod.Password = decrypted
			s.AuthMethods[i].Password = decrypted
		}

		config, cleanup, err := getSSHConfigForAuthMethod(s, authMethod)
		if err != nil {
			lastErr = fmt.Errorf("auth method %d (%s): %w", i+1, authMethod.Type, err)
			continue
		}

		if cleanup != nil {
			cleanups = append(cleanups, cleanup)
		}

		// 尝试连接
		err = connectInteractive(s, config)
		if err == nil {
			// 连接成功
			return nil
		}

		// 记录错误，继续尝试下一个认证方法
		lastErr = fmt.Errorf("auth method %d (%s): %w", i+1, authMethod.Type, err)
	}

	if lastErr != nil {
		return fmt.Errorf("all authentication methods failed: %w", lastErr)
	}
	return fmt.Errorf("all authentication methods failed")
}

// resolveTerminalType 返回发送给远端的终端类型。
// 优先读取 ~/.xsc/config.yaml 中的 ssh.terminal_type；
// 未配置时若本地 $TERM 不在远端兼容列表中，则 fallback 到 xterm-256color。
func resolveTerminalType() string {
	cfg, err := config.LoadGlobalConfig()
	if err == nil {
		return cfg.SSH.GetTerminalType()
	}
	return "xterm-256color"
}

// getHostKeyCallback 获取主机密钥验证回调
// 默认跳过验证（便捷优先），仅当配置中显式设 strict_host_key: true 时才启用 known_hosts 验证
func getHostKeyCallback() ssh.HostKeyCallback {
	cfg, err := config.LoadGlobalConfig()
	// 配置加载失败时，按默认行为（忽略验证）处理，避免因配置异常触发 known_hosts 检查
	if err != nil || !cfg.SSH.IsStrictHostKey() {
		return ssh.InsecureIgnoreHostKey()
	}

	// 仅当显式配置 strict_host_key: true 时，才使用 known_hosts 验证
	knownHostsPath, err := config.GetKnownHostsPath()
	if err != nil || knownHostsPath == "" {
		// 无法获取 known_hosts 路径，回退到忽略
		return ssh.InsecureIgnoreHostKey()
	}

	if _, statErr := os.Stat(knownHostsPath); statErr != nil {
		// known_hosts 文件不存在，回退到忽略
		return ssh.InsecureIgnoreHostKey()
	}

	hostKeyCallback, err := knownhosts.New(knownHostsPath)
	if err != nil {
		// 解析 known_hosts 失败，回退到忽略
		return ssh.InsecureIgnoreHostKey()
	}

	// TOFU: Trust on First Use
	// 未知主机（不在 known_hosts 中）→ 接受并写入
	// 已知主机但密钥变更 → 拒绝（可能 MITM 攻击）
	return func(hostname string, remote net.Addr, key ssh.PublicKey) error {
		err := hostKeyCallback(hostname, remote, key)
		if err == nil {
			return nil // 主机密钥匹配
		}

		var keyErr *knownhosts.KeyError
		if errors.As(err, &keyErr) {
			if len(keyErr.Want) == 0 {
				// 未知主机 - TOFU: 首次连接自动信任
				appendHostKey(knownHostsPath, remote, key)
				return nil
			}
			// 密钥变更 - 可能是中间人攻击，拒绝连接
			return fmt.Errorf("WARNING: host key for %s has changed! This could indicate a MITM attack: %w", hostname, err)
		}

		return err
	}
}

// appendHostKey 将主机密钥追加到 known_hosts 文件（尽力而为）
func appendHostKey(knownHostsPath string, remote net.Addr, key ssh.PublicKey) {
	f, err := os.OpenFile(knownHostsPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		fmt.Fprintf(os.Stderr, "警告: 无法打开 known_hosts 文件 %s: %v\n", knownHostsPath, err)
		return
	}
	defer f.Close()

	line := knownhosts.Line([]string{knownhosts.Normalize(remote.String())}, key)
	if _, err := fmt.Fprintf(f, "%s\n", line); err != nil {
		fmt.Fprintf(os.Stderr, "警告: 无法写入 known_hosts 文件 %s: %v\n", knownHostsPath, err)
	}
}

// getSSHConfig 根据认证类型获取 SSH 客户端配置
// 返回的 cleanup 函数用于关闭 SSH Agent 连接（非 agent 模式时为 nil）
func getSSHConfig(s *session.Session) (*ssh.ClientConfig, func(), error) {
	sshConfig := &ssh.ClientConfig{
		User:            s.User,
		HostKeyCallback: getHostKeyCallback(),
	}

	var cleanup func()

	switch s.AuthType {
	case session.AuthTypePassword:
		sshConfig.Auth = []ssh.AuthMethod{
			ssh.Password(s.Password),
		}
	case session.AuthTypeKey:
		key, err := os.ReadFile(s.KeyPath)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to read key file: %w", err)
		}
		signer, err := ssh.ParsePrivateKey(key)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to parse private key: %w", err)
		}
		sshConfig.Auth = []ssh.AuthMethod{
			ssh.PublicKeys(signer),
		}
	case session.AuthTypeAgent:
		authMethod, agentConn, err := getSSHAgentAuth()
		if err != nil {
			return nil, nil, fmt.Errorf("failed to get SSH agent auth: %w", err)
		}
		sshConfig.Auth = []ssh.AuthMethod{authMethod}
		cleanup = func() { agentConn.Close() }
	default:
		return nil, nil, fmt.Errorf("unsupported auth type: %s", s.AuthType)
	}

	return sshConfig, cleanup, nil
}

// getSSHConfigForAuthMethod 为特定的认证方法创建 SSH 配置
func getSSHConfigForAuthMethod(s *session.Session, authMethod session.AuthMethod) (*ssh.ClientConfig, func(), error) {
	sshConfig := &ssh.ClientConfig{
		User:            s.User,
		HostKeyCallback: getHostKeyCallback(),
	}

	var cleanup func()

	switch authMethod.Type {
	case "password":
		sshConfig.Auth = []ssh.AuthMethod{
			ssh.Password(authMethod.Password),
		}
	case "key", "publickey":
		keyPath := authMethod.KeyPath
		var signers []ssh.Signer

		if keyPath != "" {
			// 使用指定的密钥文件
			key, err := os.ReadFile(keyPath)
			if err != nil {
				return nil, nil, fmt.Errorf("failed to read key file: %w", err)
			}
			signer, err := ssh.ParsePrivateKey(key)
			if err != nil {
				return nil, nil, fmt.Errorf("failed to parse private key: %w", err)
			}
			signers = append(signers, signer)
		} else {
			// 如果没有指定密钥文件，自动查找 ~/.ssh/ 下的默认密钥
			defaultKeys := findDefaultSSHKeys()
			if len(defaultKeys) == 0 {
				return nil, nil, fmt.Errorf("no key path specified and no default SSH keys found in ~/.ssh")
			}
			for _, keyFile := range defaultKeys {
				key, err := os.ReadFile(keyFile)
				if err != nil {
					continue // 跳过无法读取的密钥
				}
				signer, err := ssh.ParsePrivateKey(key)
				if err != nil {
					continue // 跳过无法解析的密钥
				}
				signers = append(signers, signer)
			}
			if len(signers) == 0 {
				return nil, nil, fmt.Errorf("found default SSH keys in ~/.ssh but failed to load any of them")
			}
		}

		sshConfig.Auth = []ssh.AuthMethod{
			ssh.PublicKeys(signers...),
		}
	case "agent":
		authMethod, agentConn, err := getSSHAgentAuth()
		if err != nil {
			return nil, nil, fmt.Errorf("failed to get SSH agent auth: %w", err)
		}
		sshConfig.Auth = []ssh.AuthMethod{authMethod}
		cleanup = func() { agentConn.Close() }
	case "keyboard-interactive":
		// 键盘交互式认证 - 使用标准的键盘交互回调
		sshConfig.Auth = []ssh.AuthMethod{
			ssh.KeyboardInteractive(func(user, instruction string, questions []string, echos []bool) ([]string, error) {
				// 对于 SecureCRT 导入的会话，我们假设密码已经提供
				// 如果有密码，则使用密码回答
				if authMethod.Password != "" {
					answers := make([]string, len(questions))
					for i := range questions {
						answers[i] = authMethod.Password
					}
					return answers, nil
				}
				// 如果没有密码，返回空（让连接失败，然后尝试下一个认证方式）
				return nil, fmt.Errorf("keyboard-interactive requires password but none provided")
			}),
		}
	default:
		return nil, nil, fmt.Errorf("unsupported auth type: %s", authMethod.Type)
	}

	return sshConfig, cleanup, nil
}

// findDefaultSSHKeys 查找 ~/.ssh/ 目录下的默认 SSH 密钥文件
// 按照标准 SSH 客户端的顺序返回存在的密钥文件路径
func findDefaultSSHKeys() []string {
	// 标准 SSH 客户端默认密钥文件名（按优先级排序）
	defaultKeyNames := []string{
		"id_ed25519",
		"id_ecdsa",
		"id_ecdsa_sk",
		"id_ed25519_sk",
		"id_rsa",
		"id_dsa",
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil
	}

	sshDir := filepath.Join(homeDir, ".ssh")
	var foundKeys []string

	for _, keyName := range defaultKeyNames {
		keyPath := filepath.Join(sshDir, keyName)
		if _, err := os.Stat(keyPath); err == nil {
			foundKeys = append(foundKeys, keyPath)
		}
	}

	return foundKeys
}

// AgentKeyInfo 描述 SSH Agent 中的一个密钥
type AgentKeyInfo struct {
	Type    string
	Bits    int
	Comment string
}

// ListAgentKeys 列出 SSH Agent 中的所有密钥
func ListAgentKeys() ([]AgentKeyInfo, error) {
	authSock := os.Getenv("SSH_AUTH_SOCK")
	if authSock == "" {
		return nil, fmt.Errorf("SSH_AUTH_SOCK not set")
	}

	conn, err := net.Dial("unix", authSock)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to ssh-agent: %w", err)
	}
	defer conn.Close()

	agentClient := agent.NewClient(conn)
	keys, err := agentClient.List()
	if err != nil {
		return nil, fmt.Errorf("failed to list keys: %w", err)
	}

	var result []AgentKeyInfo
	for _, k := range keys {
		info := AgentKeyInfo{
			Type:    k.Type(),
			Comment: k.Comment,
		}
		result = append(result, info)
	}
	return result, nil
}

// getSSHAgentAuth 获取 SSH Agent 认证方法
// 返回的 net.Conn 需要调用方在 SSH 连接结束后关闭
func getSSHAgentAuth() (ssh.AuthMethod, net.Conn, error) {
	authSock := os.Getenv("SSH_AUTH_SOCK")
	if authSock == "" {
		return nil, nil, fmt.Errorf("SSH_AUTH_SOCK not set, is ssh-agent running?")
	}

	conn, err := net.Dial("unix", authSock)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to connect to ssh-agent: %w", err)
	}

	agentClient := agent.NewClient(conn)
	return ssh.PublicKeysCallback(agentClient.Signers), conn, nil
}

// connectInteractive 建立交互式 SSH 连接
func connectInteractive(s *session.Session, config *ssh.ClientConfig) error {
	addr := fmt.Sprintf("%s:%d", s.Host, s.Port)

	// 设置连接超时为10秒（业界标准）
	conn, err := net.DialTimeout("tcp", addr, 10*time.Second)
	if err != nil {
		return fmt.Errorf("connection timeout: %w", err)
	}

	// 使用已建立的连接创建 SSH 客户端
	c, chans, reqs, err := ssh.NewClientConn(conn, addr, config)
	if err != nil {
		conn.Close()
		return fmt.Errorf("failed to create SSH connection: %w", err)
	}
	client := ssh.NewClient(c, chans, reqs)
	defer client.Close()

	sess, err := client.NewSession()
	if err != nil {
		return fmt.Errorf("failed to create session: %w", err)
	}
	defer sess.Close()

	// 获取终端尺寸
	width, height, err := term.GetSize(int(os.Stdin.Fd()))
	if err != nil {
		width, height = 80, 24
	}

	// 配置终端模式
	// ONLCR: 将输出中的 \n 转换为 \r\n，解决换行问题
	modes := ssh.TerminalModes{
		ssh.ECHO:          1,
		ssh.ONLCR:         1,
		ssh.OPOST:         1,
		ssh.TTY_OP_ISPEED: 14400,
		ssh.TTY_OP_OSPEED: 14400,
	}

	termType := resolveTerminalType()

	// 请求伪终端
	if err := sess.RequestPty(termType, height, width, modes); err != nil {
		return fmt.Errorf("failed to request pty: %w", err)
	}

	// 将本地终端设为 raw 模式（必须在启动 shell 之前）
	oldState, err := term.MakeRaw(int(os.Stdin.Fd()))
	if err != nil {
		return fmt.Errorf("failed to make terminal raw: %w", err)
	}
	defer term.Restore(int(os.Stdin.Fd()), oldState)

	// 获取 stdin/stdout/stderr pipes
	stdinPipe, err := sess.StdinPipe()
	if err != nil {
		return fmt.Errorf("failed to get stdin pipe: %w", err)
	}
	defer stdinPipe.Close()

	stdoutPipe, err := sess.StdoutPipe()
	if err != nil {
		return fmt.Errorf("failed to get stdout pipe: %w", err)
	}

	stderrPipe, err := sess.StderrPipe()
	if err != nil {
		return fmt.Errorf("failed to get stderr pipe: %w", err)
	}

	// 启动 shell
	if err := sess.Shell(); err != nil {
		return fmt.Errorf("failed to start shell: %w", err)
	}

	// 设置窗口大小调整处理
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go handleWindowResize(ctx, sess)

	// 使用 goroutines 在本地终端和 SSH session 之间传输数据
	errChan := make(chan error, 3)

	// 本地 stdin -> 远程 stdin
	go func() {
		_, err := io.Copy(stdinPipe, os.Stdin)
		if err != nil {
			errChan <- fmt.Errorf("stdin copy error: %w", err)
		}
	}()

	// 远程 stdout -> 本地 stdout
	go func() {
		_, err := io.Copy(os.Stdout, stdoutPipe)
		if err != nil {
			errChan <- fmt.Errorf("stdout copy error: %w", err)
		}
	}()

	// 远程 stderr -> 本地 stderr
	go func() {
		_, err := io.Copy(os.Stderr, stderrPipe)
		if err != nil {
			errChan <- fmt.Errorf("stderr copy error: %w", err)
		}
	}()

	// 等待会话结束
	err = sess.Wait()
	if err != nil {
		// ExitError 表示远程命令以非零状态退出，属于正常退出
		if _, ok := err.(*ssh.ExitError); ok {
			return nil
		}
		return err
	}

	return nil
}

// connectSingle 使用单一认证方式建立交互式 SSH 连接
func connectSingle(s *session.Session) error {
	config, cleanup, err := getSSHConfig(s)
	if err != nil {
		return err
	}
	if cleanup != nil {
		defer cleanup()
	}
	return connectInteractive(s, config)
}

// handleWindowResize 处理终端窗口大小调整
func handleWindowResize(ctx context.Context, sess *ssh.Session) {
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGWINCH)
	defer signal.Stop(sigChan)

	for {
		select {
		case <-sigChan:
			width, height, err := term.GetSize(int(os.Stdout.Fd()))
			if err != nil {
				continue
			}
			sess.WindowChange(height, width)
		case <-ctx.Done():
			return
		}
	}
}

// ConnectWithIO 使用自定义输入输出流连接
func ConnectWithIO(s *session.Session, stdin io.Reader, stdout, stderr io.Writer) error {
	if !s.Valid {
		return fmt.Errorf("invalid session: %v", s.Error)
	}

	switch s.AuthType {
	case session.AuthTypePassword, session.AuthTypeKey, session.AuthTypeAgent:
		return connectSingleIO(s, stdin, stdout, stderr)
	default:
		return fmt.Errorf("unsupported auth type: %s", s.AuthType)
	}
}

// connectWithIO 建立非交互式 SSH 连接（支持自定义 IO）
func connectWithIO(s *session.Session, stdin io.Reader, stdout, stderr io.Writer, config *ssh.ClientConfig) error {
	addr := fmt.Sprintf("%s:%d", s.Host, s.Port)
	client, err := ssh.Dial("tcp", addr, config)
	if err != nil {
		return fmt.Errorf("failed to dial: %w", err)
	}
	defer client.Close()

	sess, err := client.NewSession()
	if err != nil {
		return fmt.Errorf("failed to create session: %w", err)
	}
	defer sess.Close()

	// 请求伪终端
	modes := ssh.TerminalModes{
		ssh.ECHO:          1,
		ssh.ONLCR:         1,
		ssh.OPOST:         1,
		ssh.TTY_OP_ISPEED: 14400,
		ssh.TTY_OP_OSPEED: 14400,
	}

	termType := resolveTerminalType()

	if err := sess.RequestPty(termType, 24, 80, modes); err != nil {
		return fmt.Errorf("failed to request pty: %w", err)
	}

	// 获取 pipes
	stdinPipe, err := sess.StdinPipe()
	if err != nil {
		return fmt.Errorf("failed to get stdin pipe: %w", err)
	}
	defer stdinPipe.Close()

	stdoutPipe, err := sess.StdoutPipe()
	if err != nil {
		return fmt.Errorf("failed to get stdout pipe: %w", err)
	}

	stderrPipe, err := sess.StderrPipe()
	if err != nil {
		return fmt.Errorf("failed to get stderr pipe: %w", err)
	}

	if err := sess.Shell(); err != nil {
		return fmt.Errorf("failed to start shell: %w", err)
	}

	// 使用 goroutines 传输数据
	errChan := make(chan error, 3)

	go func() {
		_, err := io.Copy(stdinPipe, stdin)
		if err != nil {
			errChan <- fmt.Errorf("stdin copy error: %w", err)
		}
	}()

	go func() {
		_, err := io.Copy(stdout, stdoutPipe)
		if err != nil {
			errChan <- fmt.Errorf("stdout copy error: %w", err)
		}
	}()

	go func() {
		_, err := io.Copy(stderr, stderrPipe)
		if err != nil {
			errChan <- fmt.Errorf("stderr copy error: %w", err)
		}
	}()

	if err := sess.Wait(); err != nil {
		if exitErr, ok := err.(*ssh.ExitError); ok {
			return fmt.Errorf("ssh session exited with code %d", exitErr.ExitStatus())
		}
		return err
	}

	// 检查传输错误
	select {
	case err := <-errChan:
		if err != nil {
			return err
		}
	default:
	}

	return nil
}

// connectSingleIO 使用单一认证方式建立 SSH 连接（支持自定义 IO）
func connectSingleIO(s *session.Session, stdin io.Reader, stdout, stderr io.Writer) error {
	config, cleanup, err := getSSHConfig(s)
	if err != nil {
		return err
	}
	if cleanup != nil {
		defer cleanup()
	}
	return connectWithIO(s, stdin, stdout, stderr, config)
}
