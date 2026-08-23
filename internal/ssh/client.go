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
	"sync"
	"syscall"
	"time"

	"github.com/ketor/xsc/internal/session"
	"github.com/ketor/xsc/pkg/config"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
	"golang.org/x/crypto/ssh/knownhosts"
	"golang.org/x/term"
)

// Connect 连接到 SSH 会话并接管当前终端。
func Connect(s *session.Session) error {
	client, cleanup, err := Dial(s)
	if err != nil {
		return err
	}
	defer client.Close()
	defer runCleanup(cleanup)
	return connectInteractive(s, client)
}

// Dial 建立 SSH 客户端连接。交互式调用可在缺少密码时读取终端。
func Dial(s *session.Session) (*ssh.Client, func(), error) {
	if err := prepareDialSession(s, true); err != nil {
		return nil, nil, err
	}
	return dialSession(context.Background(), s, make(map[string]bool))
}

// DialContext 建立受 context 控制的非交互式 SSH 连接。
// TCP 连接、代理拨号和 SSH 握手均受取消与超时约束。
func DialContext(ctx context.Context, s *session.Session) (*ssh.Client, func(), error) {
	if ctx == nil {
		return nil, nil, fmt.Errorf("dial context is required")
	}
	if err := prepareDialSession(s, false); err != nil {
		return nil, nil, err
	}
	return dialSession(ctx, s, make(map[string]bool))
}

func prepareDialSession(s *session.Session, interactive bool) error {
	if s == nil {
		return fmt.Errorf("session is required")
	}
	if !s.Valid {
		return fmt.Errorf("invalid session: %v", s.Error)
	}
	if s.AuthType == session.AuthTypePassword && s.Password == "" && s.EncryptedPassword != "" {
		if err := s.ResolvePassword(); err != nil {
			return fmt.Errorf("failed to resolve password: %w", err)
		}
	}
	if s.AuthType == session.AuthTypePassword && s.Password == "" {
		if !interactive {
			return fmt.Errorf("password is required for non-interactive SSH connection")
		}
		fmt.Printf("Password for %s@%s: ", s.User, s.Host)
		pw, err := term.ReadPassword(int(syscall.Stdin))
		fmt.Println()
		if err != nil {
			return fmt.Errorf("failed to read password: %w", err)
		}
		s.Password = string(pw)
	}
	return nil
}

func dialSession(ctx context.Context, s *session.Session, visited map[string]bool) (*ssh.Client, func(), error) {
	identity := s.FilePath
	if identity == "" {
		identity = fmt.Sprintf("%s@%s:%d", s.User, s.Host, s.Port)
	}
	if visited[identity] {
		return nil, nil, fmt.Errorf("proxy jump cycle detected at %s", identity)
	}
	visited[identity] = true
	defer delete(visited, identity)

	if len(s.AuthMethods) > 0 {
		return dialWithMultipleAuth(ctx, s, visited)
	}
	sshConfig, cleanup, err := getSSHConfig(s)
	if err != nil {
		return nil, nil, err
	}
	if s.ProxyJump != "" {
		return dialViaProxy(ctx, s, sshConfig, cleanup, visited)
	}
	addr := net.JoinHostPort(s.Host, fmt.Sprintf("%d", s.Port))
	return dialSSHClientWithKeepalive(ctx, addr, sshConfig, cleanup)
}

// dialViaProxy 通过跳板机建立 SSH 连接。
func dialViaProxy(ctx context.Context, s *session.Session, targetConfig *ssh.ClientConfig, targetCleanup func(), visited map[string]bool) (*ssh.Client, func(), error) {
	proxySession, err := loadProxySession(s.ProxyJump)
	if err != nil {
		runCleanup(targetCleanup)
		return nil, nil, fmt.Errorf("failed to load proxy session: %w", err)
	}
	if err := prepareDialSession(proxySession, false); err != nil {
		runCleanup(targetCleanup)
		return nil, nil, fmt.Errorf("invalid proxy session %s: %w", s.ProxyJump, err)
	}

	proxyClient, proxyCleanup, err := dialSession(ctx, proxySession, visited)
	if err != nil {
		runCleanup(targetCleanup)
		return nil, nil, fmt.Errorf("failed to connect to proxy %s: %w", s.ProxyJump, err)
	}

	targetAddr := net.JoinHostPort(s.Host, fmt.Sprintf("%d", s.Port))
	proxyConn, err := proxyClient.DialContext(ctx, "tcp", targetAddr)
	if err != nil {
		proxyClient.Close()
		runCleanup(proxyCleanup)
		runCleanup(targetCleanup)
		return nil, nil, fmt.Errorf("failed to dial target via proxy: %w", err)
	}

	if err := setHandshakeDeadline(ctx, proxyConn); err != nil {
		proxyConn.Close()
		proxyClient.Close()
		runCleanup(proxyCleanup)
		runCleanup(targetCleanup)
		return nil, nil, err
	}
	c, chans, reqs, err := ssh.NewClientConn(proxyConn, targetAddr, targetConfig)
	if err != nil {
		proxyConn.Close()
		proxyClient.Close()
		runCleanup(proxyCleanup)
		runCleanup(targetCleanup)
		return nil, nil, fmt.Errorf("failed to create SSH connection via proxy: %w", err)
	}
	_ = proxyConn.SetDeadline(time.Time{})

	client := ssh.NewClient(c, chans, reqs)
	keepaliveCtx, keepaliveCancel := context.WithCancel(context.Background())
	go startKeepalive(keepaliveCtx, client)

	combinedCleanup := func() {
		keepaliveCancel()
		proxyClient.Close()
		runCleanup(proxyCleanup)
		runCleanup(targetCleanup)
	}
	return client, combinedCleanup, nil
}

func runCleanup(cleanup func()) {
	if cleanup != nil {
		cleanup()
	}
}

// loadProxySession 加载跳板机会话
func loadProxySession(proxyJump string) (*session.Session, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get home dir: %w", err)
	}
	sessionsDir := filepath.Join(home, ".xsc", "sessions")
	return session.FindSession(sessionsDir, proxyJump)
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

// decryptAuthMethodPassword 根据密码来源解密认证方法中的加密密码
func decryptAuthMethodPassword(encryptedPwd, masterPwd, source string) (string, error) {
	d, ok := session.GetDecrypter(source)
	if !ok {
		return "", fmt.Errorf("unknown password source: %q (no decrypter registered)", source)
	}
	return d.Decrypt(encryptedPwd, masterPwd)
}

// dialSSHClientWithKeepalive 建立 TCP 连接、SSH 握手和 keepalive。
func dialSSHClientWithKeepalive(ctx context.Context, addr string, sshConfig *ssh.ClientConfig, authCleanup func()) (*ssh.Client, func(), error) {
	dialer := net.Dialer{Timeout: 10 * time.Second}
	conn, err := dialer.DialContext(ctx, "tcp", addr)
	if err != nil {
		runCleanup(authCleanup)
		return nil, nil, fmt.Errorf("connect SSH target: %w", err)
	}
	if err := setHandshakeDeadline(ctx, conn); err != nil {
		conn.Close()
		runCleanup(authCleanup)
		return nil, nil, err
	}

	c, chans, reqs, err := ssh.NewClientConn(conn, addr, sshConfig)
	if err != nil {
		conn.Close()
		runCleanup(authCleanup)
		return nil, nil, fmt.Errorf("SSH handshake failed: %w", err)
	}
	_ = conn.SetDeadline(time.Time{})

	client := ssh.NewClient(c, chans, reqs)
	keepaliveCtx, keepaliveCancel := context.WithCancel(context.Background())
	go startKeepalive(keepaliveCtx, client)
	combinedCleanup := func() {
		keepaliveCancel()
		runCleanup(authCleanup)
	}
	return client, combinedCleanup, nil
}

func setHandshakeDeadline(ctx context.Context, conn net.Conn) error {
	deadline := time.Now().Add(10 * time.Second)
	if contextDeadline, ok := ctx.Deadline(); ok && contextDeadline.Before(deadline) {
		deadline = contextDeadline
	}
	if err := conn.SetDeadline(deadline); err != nil {
		return fmt.Errorf("set SSH handshake deadline: %w", err)
	}
	return nil
}

// dialWithMultipleAuth 按顺序尝试多种认证方式。
func dialWithMultipleAuth(ctx context.Context, s *session.Session, visited map[string]bool) (*ssh.Client, func(), error) {
	var lastErr error
	for i, authMethod := range s.AuthMethods {
		if err := ctx.Err(); err != nil {
			return nil, nil, err
		}
		if authMethod.Type == "password" && authMethod.Password == "" && authMethod.EncryptedPassword != "" {
			decrypted, err := decryptAuthMethodPassword(authMethod.EncryptedPassword, s.MasterPassword, s.PasswordSource)
			if err != nil {
				lastErr = fmt.Errorf("auth method %d (%s): failed to decrypt password: %w", i+1, authMethod.Type, err)
				continue
			}
			authMethod.Password = decrypted
			s.AuthMethods[i].Password = decrypted
		}

		sshConfig, cleanup, err := getSSHConfigForAuthMethod(s, authMethod)
		if err != nil {
			lastErr = fmt.Errorf("auth method %d (%s): %w", i+1, authMethod.Type, err)
			continue
		}

		var client *ssh.Client
		var combinedCleanup func()
		if s.ProxyJump != "" {
			client, combinedCleanup, err = dialViaProxy(ctx, s, sshConfig, cleanup, visited)
		} else {
			addr := net.JoinHostPort(s.Host, fmt.Sprintf("%d", s.Port))
			client, combinedCleanup, err = dialSSHClientWithKeepalive(ctx, addr, sshConfig, cleanup)
		}
		if err != nil {
			lastErr = fmt.Errorf("auth method %d (%s): %w", i+1, authMethod.Type, err)
			continue
		}
		return client, combinedCleanup, nil
	}
	if lastErr != nil {
		return nil, nil, fmt.Errorf("all authentication methods failed: %w", lastErr)
	}
	return nil, nil, fmt.Errorf("all authentication methods failed")
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

var knownHostsMu sync.Mutex

// getHostKeyCallback 获取主机密钥验证回调。
// 默认保持兼容模式；显式 strict_host_key=true 后任何配置或持久化错误都失败关闭。
func getHostKeyCallback() ssh.HostKeyCallback {
	cfg, err := config.LoadGlobalConfig()
	if err != nil {
		return rejectingHostKeyCallback(fmt.Errorf("load SSH host-key configuration: %w", err))
	}
	if !cfg.SSH.IsStrictHostKey() {
		return ssh.InsecureIgnoreHostKey()
	}

	knownHostsPath, err := config.GetKnownHostsPath()
	if err != nil {
		return rejectingHostKeyCallback(fmt.Errorf("resolve known_hosts path: %w", err))
	}
	if err := os.MkdirAll(filepath.Dir(knownHostsPath), 0700); err != nil {
		return rejectingHostKeyCallback(fmt.Errorf("create known_hosts directory: %w", err))
	}
	f, err := os.OpenFile(knownHostsPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return rejectingHostKeyCallback(fmt.Errorf("open known_hosts: %w", err))
	}
	if err := f.Close(); err != nil {
		return rejectingHostKeyCallback(fmt.Errorf("close known_hosts: %w", err))
	}

	hostKeyCallback, err := knownhosts.New(knownHostsPath)
	if err != nil {
		return rejectingHostKeyCallback(fmt.Errorf("parse known_hosts: %w", err))
	}

	return func(hostname string, remote net.Addr, key ssh.PublicKey) error {
		err := hostKeyCallback(hostname, remote, key)
		if err == nil {
			return nil
		}

		var keyErr *knownhosts.KeyError
		if !errors.As(err, &keyErr) {
			return err
		}
		if len(keyErr.Want) != 0 {
			return fmt.Errorf("WARNING: host key for %s has changed; possible MITM attack: %w", hostname, err)
		}
		if err := appendHostKey(knownHostsPath, hostname, key); err != nil {
			return fmt.Errorf("trust-on-first-use failed for %s: %w", hostname, err)
		}
		return nil
	}
}

func rejectingHostKeyCallback(reason error) ssh.HostKeyCallback {
	return func(string, net.Addr, ssh.PublicKey) error {
		return reason
	}
}

// appendHostKey 将主机密钥安全追加到 known_hosts。
func appendHostKey(knownHostsPath, hostname string, key ssh.PublicKey) error {
	if hostname == "" || key == nil {
		return fmt.Errorf("hostname and public key are required")
	}
	knownHostsMu.Lock()
	defer knownHostsMu.Unlock()

	f, err := os.OpenFile(knownHostsPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	line := knownhosts.Line([]string{knownhosts.Normalize(hostname)}, key)
	if _, err := fmt.Fprintf(f, "%s\n", line); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
		return err
	}
	return f.Close()
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
		signers, err := loadPrivateKeySigners(s.KeyPath)
		if err != nil {
			return nil, nil, err
		}
		sshConfig.Auth = []ssh.AuthMethod{ssh.PublicKeys(signers...)}
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
		signers, err := loadPrivateKeySigners(authMethod.KeyPath)
		if err != nil {
			return nil, nil, err
		}
		sshConfig.Auth = []ssh.AuthMethod{ssh.PublicKeys(signers...)}
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

func loadPrivateKeySigners(keyPath string) ([]ssh.Signer, error) {
	keyPaths := []string{keyPath}
	if keyPath == "" {
		keyPaths = findDefaultSSHKeys()
		if len(keyPaths) == 0 {
			return nil, fmt.Errorf("no key path specified and no default SSH keys found in ~/.ssh")
		}
	}

	signers := make([]ssh.Signer, 0, len(keyPaths))
	var lastErr error
	for _, candidate := range keyPaths {
		key, err := os.ReadFile(candidate)
		if err != nil {
			lastErr = fmt.Errorf("failed to read key file %s: %w", candidate, err)
			continue
		}
		signer, err := ssh.ParsePrivateKey(key)
		if err != nil {
			lastErr = fmt.Errorf("failed to parse private key %s: %w", candidate, err)
			continue
		}
		signers = append(signers, signer)
	}
	if len(signers) == 0 {
		return nil, lastErr
	}
	return signers, nil
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

// connectInteractive 在已建立的 SSH client 上启动交互式终端。
func connectInteractive(s *session.Session, client *ssh.Client) error {
	sess, err := client.NewSession()
	if err != nil {
		return fmt.Errorf("failed to create session: %w", err)
	}
	defer sess.Close()

	width, height, err := term.GetSize(int(os.Stdin.Fd()))
	if err != nil {
		width, height = 80, 24
	}
	modes := ssh.TerminalModes{
		ssh.ECHO:          1,
		ssh.ONLCR:         1,
		ssh.OPOST:         1,
		ssh.TTY_OP_ISPEED: 14400,
		ssh.TTY_OP_OSPEED: 14400,
	}
	if err := sess.RequestPty(resolveTerminalType(), height, width, modes); err != nil {
		return fmt.Errorf("failed to request pty: %w", err)
	}

	oldState, err := term.MakeRaw(int(os.Stdin.Fd()))
	if err != nil {
		return fmt.Errorf("failed to make terminal raw: %w", err)
	}
	defer term.Restore(int(os.Stdin.Fd()), oldState)

	sess.Stdin = os.Stdin
	sess.Stdout = os.Stdout
	sess.Stderr = os.Stderr
	if err := sess.Shell(); err != nil {
		return fmt.Errorf("failed to start shell: %w", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go handleWindowResize(ctx, sess)

	if err := sess.Wait(); err != nil {
		if _, ok := err.(*ssh.ExitError); ok {
			return nil
		}
		return err
	}
	return nil
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

// ConnectWithIO 使用自定义输入输出流建立 SSH 终端。
func ConnectWithIO(s *session.Session, stdin io.Reader, stdout, stderr io.Writer) error {
	client, cleanup, err := DialContext(context.Background(), s)
	if err != nil {
		return err
	}
	defer client.Close()
	defer runCleanup(cleanup)

	sess, err := client.NewSession()
	if err != nil {
		return fmt.Errorf("failed to create session: %w", err)
	}
	defer sess.Close()

	modes := ssh.TerminalModes{
		ssh.ECHO:          1,
		ssh.ONLCR:         1,
		ssh.OPOST:         1,
		ssh.TTY_OP_ISPEED: 14400,
		ssh.TTY_OP_OSPEED: 14400,
	}
	if err := sess.RequestPty(resolveTerminalType(), 24, 80, modes); err != nil {
		return fmt.Errorf("failed to request pty: %w", err)
	}
	sess.Stdin = stdin
	sess.Stdout = stdout
	sess.Stderr = stderr
	if err := sess.Shell(); err != nil {
		return fmt.Errorf("failed to start shell: %w", err)
	}
	if err := sess.Wait(); err != nil {
		if exitErr, ok := err.(*ssh.ExitError); ok {
			return fmt.Errorf("ssh session exited with code %d", exitErr.ExitStatus())
		}
		return err
	}
	return nil
}
