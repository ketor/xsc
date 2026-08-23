package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ketor/xsc/internal/cli"
	"github.com/ketor/xsc/internal/session"
)

func TestHandleList_TextMode(t *testing.T) {
	var out, errOut bytes.Buffer
	p := cli.NewPrinter(false, &out, &errOut)

	code := handleList(context.Background(), ListParams{}, p)

	if code != cli.ExitOK && code != cli.ExitConfig && code != cli.ExitPartial {
		t.Errorf("handleList() = %d, want OK, Config, or Partial", code)
	}

	if code == cli.ExitOK && errOut.Len() > 0 {
		t.Errorf("unexpected stderr output: %s", errOut.String())
	}

	// text 模式输出不应包含 JSON
	if code == cli.ExitOK && out.Len() > 0 {
		if strings.HasPrefix(strings.TrimSpace(out.String()), "[") {
			t.Error("text mode should not output JSON array")
		}
	}
}

func TestHandleList_JSONMode(t *testing.T) {
	var out, errOut bytes.Buffer
	p := cli.NewPrinter(true, &out, &errOut)

	code := handleList(context.Background(), ListParams{JSON: true}, p)

	if code != cli.ExitOK && code != cli.ExitConfig && code != cli.ExitPartial {
		t.Errorf("handleList() = %d, want OK, Config, or Partial", code)
	}

	// JSON 模式有完整或部分结果时，stdout 都必须是有效 JSON 数组。
	if code == cli.ExitOK || code == cli.ExitPartial {
		var entries []SessionInfo
		if err := json.Unmarshal(out.Bytes(), &entries); err != nil {
			t.Errorf("JSON 解析失败: %v, output: %s", err, out.String())
		}
		if entries == nil {
			t.Error("JSON output should be [] not null for empty list")
		}
		for i, e := range entries {
			if e.Path == "" {
				t.Errorf("entries[%d].Path is empty", i)
			}
		}
	}
}

// mockDial 创建用于测试的 DialFunc。
func mockDial(latency time.Duration, err error) DialFunc {
	return func(ctx context.Context, s *session.Session) (func(), error) {
		timer := time.NewTimer(latency)
		defer timer.Stop()
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-timer.C:
		}
		if err != nil {
			return nil, err
		}
		return func() {}, nil
	}
}

func TestHandlePing_SingleSuccess(t *testing.T) {
	var out, errOut bytes.Buffer
	p := cli.NewPrinter(true, &out, &errOut)

	params := PingParams{
		Paths:    []string{"test/session"},
		JSON:     true,
		Timeout:  5 * time.Second,
		Parallel: 5,
	}

	// 注意：这里使用 handlePingWithDial + mockDial
	// 但 pingOne 内部会先调用 shared.FindSessionAllSources，找不到会返回 error
	code := handlePingWithDial(context.Background(), params, p, mockDial(10*time.Millisecond, nil))

	// 会话找不到也算失败
	if code != cli.ExitConnFailed {
		// 如果恰好有这个 session 则可能成功，但一般测试环境不会有
		if code != cli.ExitOK {
			t.Errorf("handlePing() = %d, want %d or %d", code, cli.ExitConnFailed, cli.ExitOK)
		}
	}

	// 验证 JSON 输出
	var result PingResult
	if err := json.Unmarshal(out.Bytes(), &result); err != nil {
		t.Fatalf("JSON 解析失败: %v, output: %s", err, out.String())
	}
	if result.Session != "test/session" {
		t.Errorf("session = %q, want %q", result.Session, "test/session")
	}
}

func TestHandlePing_BatchPartial(t *testing.T) {
	var out, errOut bytes.Buffer
	p := cli.NewPrinter(true, &out, &errOut)

	params := PingParams{
		Paths:    []string{"session1", "session2", "session3"},
		JSON:     true,
		Timeout:  5 * time.Second,
		Parallel: 2,
	}

	code := handlePingWithDial(context.Background(), params, p, mockDial(5*time.Millisecond, nil))

	// 会话都找不到，应为 ExitPartial
	if code != cli.ExitPartial {
		t.Errorf("handlePing batch = %d, want %d", code, cli.ExitPartial)
	}

	// 验证 NDJSON 输出（每行一个 JSON）
	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	if len(lines) != 3 {
		t.Fatalf("expected 3 NDJSON lines, got %d: %s", len(lines), out.String())
	}

	sessions := make(map[string]bool)
	for _, line := range lines {
		var r PingResult
		if err := json.Unmarshal([]byte(line), &r); err != nil {
			t.Errorf("NDJSON line parse failed: %v, line: %s", err, line)
			continue
		}
		sessions[r.Session] = true
		if r.OK {
			t.Errorf("session %q should not be OK (not found)", r.Session)
		}
	}
	for _, path := range params.Paths {
		if !sessions[path] {
			t.Errorf("missing result for session %q", path)
		}
	}
}

func TestHandlePing_TextOutput(t *testing.T) {
	var out, errOut bytes.Buffer
	p := cli.NewPrinter(false, &out, &errOut)

	params := PingParams{
		Paths:    []string{"test/session"},
		JSON:     false,
		Timeout:  5 * time.Second,
		Parallel: 5,
	}

	handlePingWithDial(context.Background(), params, p, mockDial(10*time.Millisecond, nil))

	// text 模式应含 ✗（会话找不到）
	output := out.String()
	if !strings.Contains(output, "✗") {
		t.Errorf("expected ✗ in text output, got: %s", output)
	}
	if !strings.Contains(output, "test/session") {
		t.Errorf("expected session path in output, got: %s", output)
	}
}

func TestHandlePing_Timeout(t *testing.T) {
	var out, errOut bytes.Buffer
	p := cli.NewPrinter(true, &out, &errOut)

	// 模拟受 context 控制的慢连接。
	slowDial := func(ctx context.Context, s *session.Session) (func(), error) {
		<-ctx.Done()
		return nil, ctx.Err()
	}

	params := PingParams{
		Paths:    []string{"test/slow"},
		JSON:     true,
		Timeout:  100 * time.Millisecond,
		Parallel: 5,
	}

	code := handlePingWithDial(context.Background(), params, p, slowDial)

	if code != cli.ExitConnFailed {
		t.Errorf("handlePing timeout = %d, want %d", code, cli.ExitConnFailed)
	}

	var result PingResult
	if err := json.Unmarshal(out.Bytes(), &result); err != nil {
		t.Fatalf("JSON 解析失败: %v", err)
	}
	if result.OK {
		t.Error("timed out ping should not be OK")
	}
}

func TestHandlePing_EmptyPaths(t *testing.T) {
	var out, errOut bytes.Buffer
	p := cli.NewPrinter(false, &out, &errOut)

	params := PingParams{
		Paths:    nil,
		Timeout:  5 * time.Second,
		Parallel: 5,
	}

	code := handlePingWithDial(context.Background(), params, p, mockDial(0, nil))
	if code != cli.ExitUsage {
		t.Errorf("handlePing empty = %d, want %d", code, cli.ExitUsage)
	}
}

func TestParsePingArgs(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		wantJSON bool
		wantN    int
		wantPar  int
		wantTO   time.Duration
	}{
		{
			name:     "single path",
			args:     []string{"prod/web1"},
			wantJSON: false,
			wantN:    1,
			wantPar:  5,
			wantTO:   10 * time.Second,
		},
		{
			name:     "batch with json",
			args:     []string{"a,b,c", "--json"},
			wantJSON: true,
			wantN:    3,
			wantPar:  5,
			wantTO:   10 * time.Second,
		},
		{
			name:     "with timeout and parallel",
			args:     []string{"a,b", "--timeout", "5s", "--parallel", "3"},
			wantJSON: false,
			wantN:    2,
			wantPar:  3,
			wantTO:   5 * time.Second,
		},
		{
			name:     "short flags",
			args:     []string{"x", "-t", "2s", "-p", "10", "--json"},
			wantJSON: true,
			wantN:    1,
			wantPar:  10,
			wantTO:   2 * time.Second,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := parsePingArgs(tt.args)
			if p.JSON != tt.wantJSON {
				t.Errorf("JSON = %v, want %v", p.JSON, tt.wantJSON)
			}
			if len(p.Paths) != tt.wantN {
				t.Errorf("paths count = %d, want %d", len(p.Paths), tt.wantN)
			}
			if p.Parallel != tt.wantPar {
				t.Errorf("parallel = %d, want %d", p.Parallel, tt.wantPar)
			}
			if p.Timeout != tt.wantTO {
				t.Errorf("timeout = %v, want %v", p.Timeout, tt.wantTO)
			}
		})
	}
}

func TestPrintPingResult_TextSuccess(t *testing.T) {
	var out bytes.Buffer
	p := cli.NewPrinter(false, &out, &bytes.Buffer{})

	printPingResult(p, PingResult{Session: "web1", OK: true, LatencyMS: 42})

	if !strings.Contains(out.String(), "✓ web1 (42ms)") {
		t.Errorf("unexpected output: %s", out.String())
	}
}

func TestPrintPingResult_TextFailure(t *testing.T) {
	var out bytes.Buffer
	p := cli.NewPrinter(false, &out, &bytes.Buffer{})

	printPingResult(p, PingResult{Session: "web2", OK: false, Error: "connection refused"})

	if !strings.Contains(out.String(), "✗ web2 (connection refused)") {
		t.Errorf("unexpected output: %s", out.String())
	}
}

func TestPrintPingResult_JSON(t *testing.T) {
	var out bytes.Buffer
	p := cli.NewPrinter(true, &out, &bytes.Buffer{})

	printPingResult(p, PingResult{Session: "web1", OK: true, LatencyMS: 42})

	var r PingResult
	if err := json.Unmarshal(out.Bytes(), &r); err != nil {
		t.Fatalf("JSON parse failed: %v", err)
	}
	if !r.OK || r.LatencyMS != 42 || r.Session != "web1" {
		t.Errorf("unexpected result: %+v", r)
	}
}

// --- add/show tests ---

func TestHandleAdd_Success_JSON(t *testing.T) {
	tmpDir := t.TempDir()
	// 设置 XDG/HOME 让 config.GetSessionsDir 返回 tmpDir 下路径
	// 直接用 handleAdd 内部逻辑需要 config.GetSessionsDir，但我们可以通过
	// 在 tmpDir 创建目录结构并设置 HOME 来隔离
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", origHome)

	// 创建 sessions 目录
	sessionsDir := filepath.Join(tmpDir, ".xsc", "sessions")
	os.MkdirAll(sessionsDir, 0755)

	var out, errOut bytes.Buffer
	p := cli.NewPrinter(true, &out, &errOut)

	params := AddParams{
		Path:     "test/server1",
		Host:     "192.168.1.1",
		Port:     2222,
		User:     "admin",
		AuthType: "password",
		Password: "secret",
		JSON:     true,
	}

	code := handleAdd(context.Background(), params, p)
	if code != cli.ExitOK {
		t.Fatalf("handleAdd() = %d, want %d, stderr: %s", code, cli.ExitOK, errOut.String())
	}

	// 验证 JSON 输出
	var result AddResult
	if err := json.Unmarshal(out.Bytes(), &result); err != nil {
		t.Fatalf("JSON parse failed: %v, output: %s", err, out.String())
	}
	if !result.Created {
		t.Error("expected created=true")
	}
	if result.Path != "test/server1" {
		t.Errorf("path = %q, want %q", result.Path, "test/server1")
	}

	// 验证文件已创建
	filePath := filepath.Join(sessionsDir, "test", "server1.yaml")
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		t.Errorf("session file not created: %s", filePath)
	}

	// 验证文件内容可加载
	s, err := session.LoadSession(filePath)
	if err != nil {
		t.Fatalf("LoadSession failed: %v", err)
	}
	if s.Host != "192.168.1.1" {
		t.Errorf("host = %q, want %q", s.Host, "192.168.1.1")
	}
	if s.Port != 2222 {
		t.Errorf("port = %d, want %d", s.Port, 2222)
	}
	if s.User != "admin" {
		t.Errorf("user = %q, want %q", s.User, "admin")
	}
}

func TestHandleAdd_Success_Text(t *testing.T) {
	tmpDir := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", origHome)
	os.MkdirAll(filepath.Join(tmpDir, ".xsc", "sessions"), 0755)

	var out, errOut bytes.Buffer
	p := cli.NewPrinter(false, &out, &errOut)

	params := AddParams{
		Path: "myserver",
		Host: "10.0.0.1",
	}

	code := handleAdd(context.Background(), params, p)
	if code != cli.ExitOK {
		t.Fatalf("handleAdd() = %d, want %d, stderr: %s", code, cli.ExitOK, errOut.String())
	}

	if !strings.Contains(out.String(), "✓ 会话已创建: myserver") {
		t.Errorf("unexpected text output: %s", out.String())
	}
}

func TestHandleAdd_Defaults(t *testing.T) {
	tmpDir := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", origHome)
	os.MkdirAll(filepath.Join(tmpDir, ".xsc", "sessions"), 0755)

	var out, errOut bytes.Buffer
	p := cli.NewPrinter(true, &out, &errOut)

	// 只指定 path 和 host，其余用默认值
	params := AddParams{
		Path: "default-test",
		Host: "1.2.3.4",
		JSON: true,
	}

	code := handleAdd(context.Background(), params, p)
	if code != cli.ExitOK {
		t.Fatalf("handleAdd() = %d, want %d", code, cli.ExitOK)
	}

	// 验证默认值
	filePath := filepath.Join(tmpDir, ".xsc", "sessions", "default-test.yaml")
	s, err := session.LoadSession(filePath)
	if err != nil {
		t.Fatalf("LoadSession failed: %v", err)
	}
	if s.Port != 22 {
		t.Errorf("default port = %d, want 22", s.Port)
	}
	if s.User != "root" {
		t.Errorf("default user = %q, want 'root'", s.User)
	}
	if s.AuthType != "password" {
		t.Errorf("default auth_type = %q, want 'password'", s.AuthType)
	}
}

func TestHandleAdd_MissingHost(t *testing.T) {
	var out, errOut bytes.Buffer
	p := cli.NewPrinter(true, &out, &errOut)

	params := AddParams{Path: "test/no-host", JSON: true}
	code := handleAdd(context.Background(), params, p)
	if code != cli.ExitUsage {
		t.Errorf("handleAdd() = %d, want %d", code, cli.ExitUsage)
	}
}

func TestHandleAdd_MissingPath(t *testing.T) {
	var out, errOut bytes.Buffer
	p := cli.NewPrinter(true, &out, &errOut)

	params := AddParams{Host: "1.2.3.4", JSON: true}
	code := handleAdd(context.Background(), params, p)
	if code != cli.ExitUsage {
		t.Errorf("handleAdd() = %d, want %d", code, cli.ExitUsage)
	}
}

func TestHandleAdd_XSCPasswordEnv(t *testing.T) {
	tmpDir := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", origHome)
	os.MkdirAll(filepath.Join(tmpDir, ".xsc", "sessions"), 0755)

	// 设置 XSC_PASSWORD 环境变量
	os.Setenv("XSC_PASSWORD", "env-password")
	defer os.Unsetenv("XSC_PASSWORD")

	var out, errOut bytes.Buffer
	p := cli.NewPrinter(true, &out, &errOut)

	params := AddParams{
		Path:     "env-pwd-test",
		Host:     "10.0.0.5",
		AuthType: "password",
		JSON:     true,
	}

	code := handleAdd(context.Background(), params, p)
	if code != cli.ExitOK {
		t.Fatalf("handleAdd() = %d, want %d", code, cli.ExitOK)
	}

	// 验证密码已从环境变量获取
	filePath := filepath.Join(tmpDir, ".xsc", "sessions", "env-pwd-test.yaml")
	s, err := session.LoadSession(filePath)
	if err != nil {
		t.Fatalf("LoadSession failed: %v", err)
	}
	if s.Password != "env-password" {
		t.Errorf("password = %q, want 'env-password'", s.Password)
	}
}

func TestHandleShow_NotFound(t *testing.T) {
	var out, errOut bytes.Buffer
	p := cli.NewPrinter(true, &out, &errOut)

	params := ShowParams{Path: "nonexistent/session", JSON: true}
	code := handleShow(context.Background(), params, p)
	if code != cli.ExitNotFound {
		t.Errorf("handleShow() = %d, want %d", code, cli.ExitNotFound)
	}
}

func TestHandleShow_MissingPath(t *testing.T) {
	var out, errOut bytes.Buffer
	p := cli.NewPrinter(true, &out, &errOut)

	params := ShowParams{JSON: true}
	code := handleShow(context.Background(), params, p)
	if code != cli.ExitUsage {
		t.Errorf("handleShow() = %d, want %d", code, cli.ExitUsage)
	}
}

func TestHandleShow_Found(t *testing.T) {
	tmpDir := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", origHome)
	sessionsDir := filepath.Join(tmpDir, ".xsc", "sessions")
	os.MkdirAll(sessionsDir, 0755)

	keyPath := filepath.Join(tmpDir, "id_show")
	if err := os.WriteFile(keyPath, []byte("test key placeholder"), 0600); err != nil {
		t.Fatalf("write key fixture: %v", err)
	}

	// 先创建一个会话文件
	s := &session.Session{
		Host:     "10.0.0.99",
		Port:     22,
		User:     "deploy",
		AuthType: "key",
		KeyPath:  keyPath,
	}
	session.SaveSession(s, filepath.Join(sessionsDir, "prod", "web1.yaml"))

	var out, errOut bytes.Buffer
	p := cli.NewPrinter(true, &out, &errOut)

	params := ShowParams{Path: "prod/web1", JSON: true}
	code := handleShow(context.Background(), params, p)
	if code != cli.ExitOK {
		t.Fatalf("handleShow() = %d, want %d, stderr: %s", code, cli.ExitOK, errOut.String())
	}

	var result ShowResult
	if err := json.Unmarshal(out.Bytes(), &result); err != nil {
		t.Fatalf("JSON parse failed: %v, output: %s", err, out.String())
	}
	if result.Host != "10.0.0.99" {
		t.Errorf("host = %q, want %q", result.Host, "10.0.0.99")
	}
	if result.User != "deploy" {
		t.Errorf("user = %q, want %q", result.User, "deploy")
	}
	if result.AuthType != "key" {
		t.Errorf("auth_type = %q, want %q", result.AuthType, "key")
	}
}

func TestHandleShow_TextOutput(t *testing.T) {
	tmpDir := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", origHome)
	sessionsDir := filepath.Join(tmpDir, ".xsc", "sessions")
	os.MkdirAll(sessionsDir, 0755)

	s := &session.Session{
		Host:     "10.0.0.99",
		Port:     22,
		User:     "root",
		AuthType: "password",
		Password: "secret",
	}
	session.SaveSession(s, filepath.Join(sessionsDir, "myhost.yaml"))

	var out, errOut bytes.Buffer
	p := cli.NewPrinter(false, &out, &errOut)

	params := ShowParams{Path: "myhost"}
	code := handleShow(context.Background(), params, p)
	if code != cli.ExitOK {
		t.Fatalf("handleShow() = %d, want %d, stderr: %s", code, cli.ExitOK, errOut.String())
	}

	output := out.String()
	if !strings.Contains(output, "Host:") || !strings.Contains(output, "10.0.0.99") {
		t.Errorf("text output missing host info: %s", output)
	}
	// Password should NOT appear in show output
	if strings.Contains(output, "secret") {
		t.Error("show output should not contain password")
	}
}

func TestParseAddArgs(t *testing.T) {
	args := []string{"prod/db/master", "--host", "db.example.com", "--port", "3306", "--user", "dba", "--auth-type", "key", "--key", "/home/dba/.ssh/id_rsa", "--json"}
	params := parseAddArgs(args)

	if params.Path != "prod/db/master" {
		t.Errorf("path = %q, want %q", params.Path, "prod/db/master")
	}
	if params.Host != "db.example.com" {
		t.Errorf("host = %q, want %q", params.Host, "db.example.com")
	}
	if params.Port != 3306 {
		t.Errorf("port = %d, want %d", params.Port, 3306)
	}
	if params.User != "dba" {
		t.Errorf("user = %q, want %q", params.User, "dba")
	}
	if params.AuthType != "key" {
		t.Errorf("auth_type = %q, want %q", params.AuthType, "key")
	}
	if params.KeyPath != "/home/dba/.ssh/id_rsa" {
		t.Errorf("key = %q, want %q", params.KeyPath, "/home/dba/.ssh/id_rsa")
	}
	if !params.JSON {
		t.Error("json should be true")
	}
}

func TestParseShowArgs(t *testing.T) {
	args := []string{"prod/web1", "--json"}
	params := parseShowArgs(args)

	if params.Path != "prod/web1" {
		t.Errorf("path = %q, want %q", params.Path, "prod/web1")
	}
	if !params.JSON {
		t.Error("json should be true")
	}
}

// 确保 DialFunc 不能访问测试环境外（编译验证 mockDial 签名兼容性）
func TestDialFunc_Compatibility(t *testing.T) {
	var df DialFunc = mockDial(0, nil)
	cleanup, err := df(context.Background(), &session.Session{Valid: true, Host: "127.0.0.1", Port: 22})
	if err != nil {
		t.Fatalf("mockDial error: %v", err)
	}
	if cleanup == nil {
		t.Fatal("mockDial should return non-nil cleanup")
	}
	cleanup()
}

func TestDialFunc_Error(t *testing.T) {
	var df DialFunc = mockDial(0, fmt.Errorf("mock error"))
	_, err := df(context.Background(), &session.Session{Valid: true, Host: "127.0.0.1", Port: 22})
	if err == nil || err.Error() != "mock error" {
		t.Errorf("expected mock error, got: %v", err)
	}
}

// --- exec-multi tests ---

// mockExec 创建用于测试的 ExecFunc
func mockExec(exitCode int, stdout, stderr, errMsg string, delay time.Duration) ExecFunc {
	return func(ctx context.Context, path string, command string) ExecResult {
		select {
		case <-ctx.Done():
			return ExecResult{Session: path, ExitCode: cli.ExitTimeout, Error: "命令执行超时"}
		case <-time.After(delay):
		}
		return ExecResult{
			Session:  path,
			Stdout:   stdout,
			Stderr:   stderr,
			ExitCode: exitCode,
			Error:    errMsg,
		}
	}
}

func TestHandleExecMulti_SingleSuccess(t *testing.T) {
	var out, errOut bytes.Buffer
	p := cli.NewPrinter(true, &out, &errOut)

	params := ExecMultiParams{
		Paths:    []string{"server1"},
		Command:  "uptime",
		JSON:     true,
		Timeout:  5 * time.Second,
		Parallel: 5,
	}

	code := handleExecMultiWithFunc(context.Background(), params, p,
		mockExec(0, "up 42 days\n", "", "", 0))

	if code != cli.ExitOK {
		t.Errorf("handleExecMulti() = %d, want %d", code, cli.ExitOK)
	}

	var result ExecResult
	if err := json.Unmarshal(out.Bytes(), &result); err != nil {
		t.Fatalf("JSON parse failed: %v, output: %s", err, out.String())
	}
	if result.Session != "server1" {
		t.Errorf("session = %q, want %q", result.Session, "server1")
	}
	if result.Stdout != "up 42 days\n" {
		t.Errorf("stdout = %q, want %q", result.Stdout, "up 42 days\n")
	}
	if result.ExitCode != 0 {
		t.Errorf("exit_code = %d, want 0", result.ExitCode)
	}
}

func TestHandleExecMulti_BatchNDJSON(t *testing.T) {
	var out, errOut bytes.Buffer
	p := cli.NewPrinter(true, &out, &errOut)

	params := ExecMultiParams{
		Paths:    []string{"s1", "s2", "s3"},
		Command:  "hostname",
		JSON:     true,
		Timeout:  5 * time.Second,
		Parallel: 3,
	}

	code := handleExecMultiWithFunc(context.Background(), params, p,
		mockExec(0, "host\n", "", "", 5*time.Millisecond))

	if code != cli.ExitOK {
		t.Errorf("handleExecMulti batch = %d, want %d", code, cli.ExitOK)
	}

	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	if len(lines) != 3 {
		t.Fatalf("expected 3 NDJSON lines, got %d: %s", len(lines), out.String())
	}

	sessions := make(map[string]bool)
	for _, line := range lines {
		var r ExecResult
		if err := json.Unmarshal([]byte(line), &r); err != nil {
			t.Errorf("NDJSON parse failed: %v, line: %s", err, line)
			continue
		}
		sessions[r.Session] = true
	}
	for _, path := range params.Paths {
		if !sessions[path] {
			t.Errorf("missing result for session %q", path)
		}
	}
}

func TestHandleExecMulti_PartialFailure(t *testing.T) {
	var out, errOut bytes.Buffer
	p := cli.NewPrinter(true, &out, &errOut)

	callCount := 0
	var mu sync.Mutex
	mixedExec := func(ctx context.Context, path string, command string) ExecResult {
		mu.Lock()
		callCount++
		n := callCount
		mu.Unlock()
		if n%2 == 0 {
			return ExecResult{Session: path, ExitCode: 1, Error: "command failed"}
		}
		return ExecResult{Session: path, Stdout: "ok\n", ExitCode: 0}
	}

	params := ExecMultiParams{
		Paths:    []string{"s1", "s2", "s3", "s4"},
		Command:  "test",
		JSON:     true,
		Timeout:  5 * time.Second,
		Parallel: 4,
	}

	code := handleExecMultiWithFunc(context.Background(), params, p, mixedExec)

	if code != cli.ExitPartial {
		t.Errorf("handleExecMulti partial = %d, want %d", code, cli.ExitPartial)
	}
}

func TestHandleExecMulti_FailFast(t *testing.T) {
	var out, errOut bytes.Buffer
	p := cli.NewPrinter(true, &out, &errOut)

	// 第一个立即失败，其余需要 1 秒
	failFastExec := func(ctx context.Context, path string, command string) ExecResult {
		if path == "fail" {
			return ExecResult{Session: path, ExitCode: 1, Error: "immediate failure"}
		}
		select {
		case <-ctx.Done():
			return ExecResult{Session: path, ExitCode: cli.ExitTimeout, Error: "已取消（fail-fast）"}
		case <-time.After(1 * time.Second):
			return ExecResult{Session: path, Stdout: "ok\n", ExitCode: 0}
		}
	}

	params := ExecMultiParams{
		Paths:    []string{"fail", "slow1", "slow2"},
		Command:  "test",
		JSON:     true,
		Timeout:  5 * time.Second,
		Parallel: 3,
		FailFast: true,
	}

	start := time.Now()
	code := handleExecMultiWithFunc(context.Background(), params, p, failFastExec)
	elapsed := time.Since(start)

	if code != cli.ExitPartial {
		t.Errorf("handleExecMulti fail-fast = %d, want %d", code, cli.ExitPartial)
	}

	// fail-fast 应在远小于 1 秒内完成
	if elapsed > 500*time.Millisecond {
		t.Errorf("fail-fast took %v, expected < 500ms", elapsed)
	}
}

func TestHandleExecMulti_IgnoreErrors(t *testing.T) {
	var out, errOut bytes.Buffer
	p := cli.NewPrinter(true, &out, &errOut)

	params := ExecMultiParams{
		Paths:        []string{"s1"},
		Command:      "test",
		JSON:         true,
		Timeout:      5 * time.Second,
		Parallel:     5,
		IgnoreErrors: true,
	}

	code := handleExecMultiWithFunc(context.Background(), params, p,
		mockExec(1, "", "", "failed", 0))

	if code != cli.ExitOK {
		t.Errorf("handleExecMulti ignore-errors = %d, want %d", code, cli.ExitOK)
	}
}

func TestHandleExecMulti_EmptyPaths(t *testing.T) {
	var out, errOut bytes.Buffer
	p := cli.NewPrinter(false, &out, &errOut)

	params := ExecMultiParams{Command: "test", Timeout: 5 * time.Second, Parallel: 5}
	code := handleExecMultiWithFunc(context.Background(), params, p, mockExec(0, "", "", "", 0))
	if code != cli.ExitUsage {
		t.Errorf("empty paths = %d, want %d", code, cli.ExitUsage)
	}
}

func TestHandleExecMulti_EmptyCommand(t *testing.T) {
	var out, errOut bytes.Buffer
	p := cli.NewPrinter(false, &out, &errOut)

	params := ExecMultiParams{Paths: []string{"s1"}, Timeout: 5 * time.Second, Parallel: 5}
	code := handleExecMultiWithFunc(context.Background(), params, p, mockExec(0, "", "", "", 0))
	if code != cli.ExitUsage {
		t.Errorf("empty command = %d, want %d", code, cli.ExitUsage)
	}
}

func TestHandleExecMulti_TextOutput(t *testing.T) {
	var out, errOut bytes.Buffer
	p := cli.NewPrinter(false, &out, &errOut)

	params := ExecMultiParams{
		Paths:    []string{"web1"},
		Command:  "uptime",
		Timeout:  5 * time.Second,
		Parallel: 5,
	}

	handleExecMultiWithFunc(context.Background(), params, p,
		mockExec(0, "up 10 days\n", "", "", 0))

	output := out.String()
	if !strings.Contains(output, "=== web1 ===") {
		t.Errorf("expected header in text output, got: %s", output)
	}
	if !strings.Contains(output, "up 10 days") {
		t.Errorf("expected stdout in text output, got: %s", output)
	}
}

func TestHandleExecMulti_TextErrorOutput(t *testing.T) {
	var out, errOut bytes.Buffer
	p := cli.NewPrinter(false, &out, &errOut)

	params := ExecMultiParams{
		Paths:    []string{"broken"},
		Command:  "test",
		Timeout:  5 * time.Second,
		Parallel: 5,
	}

	handleExecMultiWithFunc(context.Background(), params, p,
		mockExec(1, "", "error output\n", "cmd failed", 0))

	if !strings.Contains(out.String(), "失败: cmd failed") {
		t.Errorf("expected failure text, got: %s", out.String())
	}
	if !strings.Contains(errOut.String(), "error output") {
		t.Errorf("expected stderr output, got: %s", errOut.String())
	}
}

func TestParseExecMultiArgs(t *testing.T) {
	tests := []struct {
		name     string
		paths    string
		args     []string
		wantCmd  string
		wantJSON bool
		wantPar  int
		wantTO   time.Duration
		wantFF   bool
		wantIE   bool
	}{
		{
			name:    "basic",
			paths:   "s1,s2",
			args:    []string{"uptime"},
			wantCmd: "uptime",
			wantPar: 5,
			wantTO:  30 * time.Second,
		},
		{
			name:     "all flags",
			paths:    "a,b,c",
			args:     []string{"--json", "--timeout", "10", "--parallel", "3", "--fail-fast", "--ignore-errors", "df", "-h"},
			wantCmd:  "df -h",
			wantJSON: true,
			wantPar:  3,
			wantTO:   10 * time.Second,
			wantFF:   true,
			wantIE:   true,
		},
		{
			name:    "timeout cap",
			paths:   "s1",
			args:    []string{"-t", "999", "test"},
			wantCmd: "test",
			wantPar: 5,
			wantTO:  300 * time.Second,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := parseExecMultiArgs(tt.paths, tt.args)
			if p.Command != tt.wantCmd {
				t.Errorf("command = %q, want %q", p.Command, tt.wantCmd)
			}
			if p.JSON != tt.wantJSON {
				t.Errorf("JSON = %v, want %v", p.JSON, tt.wantJSON)
			}
			if p.Parallel != tt.wantPar {
				t.Errorf("parallel = %d, want %d", p.Parallel, tt.wantPar)
			}
			if p.Timeout != tt.wantTO {
				t.Errorf("timeout = %v, want %v", p.Timeout, tt.wantTO)
			}
			if p.FailFast != tt.wantFF {
				t.Errorf("fail-fast = %v, want %v", p.FailFast, tt.wantFF)
			}
			if p.IgnoreErrors != tt.wantIE {
				t.Errorf("ignore-errors = %v, want %v", p.IgnoreErrors, tt.wantIE)
			}
		})
	}
}

func TestPrintExecResult_JSON(t *testing.T) {
	var out bytes.Buffer
	p := cli.NewPrinter(true, &out, &bytes.Buffer{})

	printExecResult(p, ExecResult{Session: "web1", Stdout: "ok\n", ExitCode: 0})

	var r ExecResult
	if err := json.Unmarshal(out.Bytes(), &r); err != nil {
		t.Fatalf("JSON parse failed: %v", err)
	}
	if r.Session != "web1" || r.Stdout != "ok\n" || r.ExitCode != 0 {
		t.Errorf("unexpected result: %+v", r)
	}
}

func TestPrintExecResult_TextSuccess(t *testing.T) {
	var out bytes.Buffer
	p := cli.NewPrinter(false, &out, &bytes.Buffer{})

	printExecResult(p, ExecResult{Session: "web1", Stdout: "hello\n", ExitCode: 0})

	if !strings.Contains(out.String(), "=== web1 ===") {
		t.Errorf("unexpected output: %s", out.String())
	}
	if !strings.Contains(out.String(), "hello") {
		t.Errorf("expected stdout, got: %s", out.String())
	}
}

func TestPrintExecResult_TextError(t *testing.T) {
	var out bytes.Buffer
	p := cli.NewPrinter(false, &out, &bytes.Buffer{})

	printExecResult(p, ExecResult{Session: "web2", ExitCode: 1, Error: "conn refused"})

	if !strings.Contains(out.String(), "失败: conn refused") {
		t.Errorf("unexpected output: %s", out.String())
	}
}

// --- edit/delete tests ---

func TestHandleEdit_Success_JSON(t *testing.T) {
	tmpDir := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", origHome)
	sessionsDir := filepath.Join(tmpDir, ".xsc", "sessions")
	os.MkdirAll(sessionsDir, 0755)

	// 先创建一个会话文件
	s := &session.Session{
		Host:     "10.0.0.1",
		Port:     22,
		User:     "root",
		AuthType: "password",
	}
	session.SaveSession(s, filepath.Join(sessionsDir, "edit-test.yaml"))

	keyPath := filepath.Join(tmpDir, "id_test")
	if err := os.WriteFile(keyPath, []byte("test key placeholder"), 0600); err != nil {
		t.Fatalf("write key fixture: %v", err)
	}

	var out, errOut bytes.Buffer
	p := cli.NewPrinter(true, &out, &errOut)

	params := EditParams{
		Path:     "edit-test",
		Host:     "10.0.0.99",
		Port:     2222,
		User:     "admin",
		AuthType: "key",
		KeyPath:  keyPath,
		JSON:     true,
	}

	code := handleEdit(context.Background(), params, p)
	if code != cli.ExitOK {
		t.Fatalf("handleEdit() = %d, want %d, stderr: %s", code, cli.ExitOK, errOut.String())
	}

	// 验证 JSON 输出
	var result EditResult
	if err := json.Unmarshal(out.Bytes(), &result); err != nil {
		t.Fatalf("JSON parse failed: %v, output: %s", err, out.String())
	}
	if !result.Updated {
		t.Error("expected updated=true")
	}
	if result.Path != "edit-test" {
		t.Errorf("path = %q, want %q", result.Path, "edit-test")
	}

	// 验证文件已更新
	updated, err := session.LoadSession(filepath.Join(sessionsDir, "edit-test.yaml"))
	if err != nil {
		t.Fatalf("LoadSession failed: %v", err)
	}
	if updated.Host != "10.0.0.99" {
		t.Errorf("host = %q, want %q", updated.Host, "10.0.0.99")
	}
	if updated.Port != 2222 {
		t.Errorf("port = %d, want %d", updated.Port, 2222)
	}
	if updated.User != "admin" {
		t.Errorf("user = %q, want %q", updated.User, "admin")
	}
}

func TestHandleEdit_Success_Text(t *testing.T) {
	tmpDir := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", origHome)
	sessionsDir := filepath.Join(tmpDir, ".xsc", "sessions")
	os.MkdirAll(sessionsDir, 0755)

	s := &session.Session{Host: "10.0.0.1", Port: 22, User: "root", AuthType: "password"}
	session.SaveSession(s, filepath.Join(sessionsDir, "text-edit.yaml"))

	var out, errOut bytes.Buffer
	p := cli.NewPrinter(false, &out, &errOut)

	params := EditParams{Path: "text-edit", Host: "10.0.0.2"}

	code := handleEdit(context.Background(), params, p)
	if code != cli.ExitOK {
		t.Fatalf("handleEdit() = %d, want %d", code, cli.ExitOK)
	}

	if !strings.Contains(out.String(), "✓ 会话已更新: text-edit") {
		t.Errorf("unexpected text output: %s", out.String())
	}
}

func TestHandleEdit_NotFound(t *testing.T) {
	var out, errOut bytes.Buffer
	p := cli.NewPrinter(true, &out, &errOut)

	params := EditParams{Path: "nonexistent/session", Host: "10.0.0.1", JSON: true}
	code := handleEdit(context.Background(), params, p)
	if code != cli.ExitNotFound {
		t.Errorf("handleEdit() = %d, want %d", code, cli.ExitNotFound)
	}
}

func TestHandleEdit_MissingPath(t *testing.T) {
	var out, errOut bytes.Buffer
	p := cli.NewPrinter(true, &out, &errOut)

	params := EditParams{Host: "10.0.0.1", JSON: true}
	code := handleEdit(context.Background(), params, p)
	if code != cli.ExitUsage {
		t.Errorf("handleEdit() = %d, want %d", code, cli.ExitUsage)
	}
}

func TestHandleEdit_NonLocalSession(t *testing.T) {
	// 注意：PasswordSource 字段使用 yaml:"-" 标签，不会被序列化。
	// 非本地会话（SecureCRT/Xshell/MobaXterm）是通过 FindSessionAllSources
	// 从外部配置加载的，PasswordSource 会在加载时设置。
	// 本测试验证：如果 PasswordSource 非空，编辑应被拒绝。
	//
	// 由于无法通过文件模拟非本地会话，此测试仅验证本地会话可编辑。
	// 非本地会话的集成测试需要在有实际外部配置的环境中进行。

	tmpDir := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", origHome)
	sessionsDir := filepath.Join(tmpDir, ".xsc", "sessions")
	os.MkdirAll(sessionsDir, 0755)

	// 创建本地会话
	s := &session.Session{
		Host:     "10.0.0.1",
		Port:     22,
		User:     "root",
		AuthType: "password",
	}
	session.SaveSession(s, filepath.Join(sessionsDir, "local.yaml"))

	var out, errOut bytes.Buffer
	p := cli.NewPrinter(true, &out, &errOut)

	params := EditParams{Path: "local", Host: "10.0.0.2", JSON: true}
	code := handleEdit(context.Background(), params, p)
	if code != cli.ExitOK {
		t.Errorf("handleEdit() = %d, want %d (local session should be editable)", code, cli.ExitOK)
	}
}

func TestHandleDelete_Success_JSON(t *testing.T) {
	tmpDir := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", origHome)
	sessionsDir := filepath.Join(tmpDir, ".xsc", "sessions")
	os.MkdirAll(sessionsDir, 0755)

	// 先创建一个会话文件
	s := &session.Session{Host: "10.0.0.1", Port: 22, User: "root", AuthType: "password"}
	session.SaveSession(s, filepath.Join(sessionsDir, "delete-test.yaml"))

	var out, errOut bytes.Buffer
	p := cli.NewPrinter(true, &out, &errOut)

	params := DeleteParams{Path: "delete-test", JSON: true}

	code := handleDelete(context.Background(), params, p)
	if code != cli.ExitOK {
		t.Fatalf("handleDelete() = %d, want %d, stderr: %s", code, cli.ExitOK, errOut.String())
	}

	// 验证 JSON 输出
	var result DeleteResult
	if err := json.Unmarshal(out.Bytes(), &result); err != nil {
		t.Fatalf("JSON parse failed: %v, output: %s", err, out.String())
	}
	if !result.Deleted {
		t.Error("expected deleted=true")
	}

	// 验证文件已删除
	filePath := filepath.Join(sessionsDir, "delete-test.yaml")
	if _, err := os.Stat(filePath); !os.IsNotExist(err) {
		t.Errorf("session file should be deleted: %s", filePath)
	}
}

func TestHandleDelete_Success_Text(t *testing.T) {
	tmpDir := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", origHome)
	sessionsDir := filepath.Join(tmpDir, ".xsc", "sessions")
	os.MkdirAll(sessionsDir, 0755)

	s := &session.Session{Host: "10.0.0.1", Port: 22, User: "root", AuthType: "password"}
	session.SaveSession(s, filepath.Join(sessionsDir, "text-delete.yaml"))

	var out, errOut bytes.Buffer
	p := cli.NewPrinter(false, &out, &errOut)

	params := DeleteParams{Path: "text-delete"}

	code := handleDelete(context.Background(), params, p)
	if code != cli.ExitOK {
		t.Fatalf("handleDelete() = %d, want %d", code, cli.ExitOK)
	}

	if !strings.Contains(out.String(), "✓ 会话已删除: text-delete") {
		t.Errorf("unexpected text output: %s", out.String())
	}
}

func TestHandleDelete_NotFound(t *testing.T) {
	var out, errOut bytes.Buffer
	p := cli.NewPrinter(true, &out, &errOut)

	params := DeleteParams{Path: "nonexistent/session", JSON: true}
	code := handleDelete(context.Background(), params, p)
	if code != cli.ExitNotFound {
		t.Errorf("handleDelete() = %d, want %d", code, cli.ExitNotFound)
	}
}

func TestHandleDelete_MissingPath(t *testing.T) {
	var out, errOut bytes.Buffer
	p := cli.NewPrinter(true, &out, &errOut)

	params := DeleteParams{JSON: true}
	code := handleDelete(context.Background(), params, p)
	if code != cli.ExitUsage {
		t.Errorf("handleDelete() = %d, want %d", code, cli.ExitUsage)
	}
}

func TestParseEditArgs(t *testing.T) {
	args := []string{"prod/web1", "--host", "10.0.0.2", "--port", "2222", "--user", "admin", "--auth-type", "key", "--key", "/home/admin/.ssh/id_rsa", "--json"}
	params := parseEditArgs(args)

	if params.Path != "prod/web1" {
		t.Errorf("path = %q, want %q", params.Path, "prod/web1")
	}
	if params.Host != "10.0.0.2" {
		t.Errorf("host = %q, want %q", params.Host, "10.0.0.2")
	}
	if params.Port != 2222 {
		t.Errorf("port = %d, want %d", params.Port, 2222)
	}
	if params.User != "admin" {
		t.Errorf("user = %q, want %q", params.User, "admin")
	}
	if params.AuthType != "key" {
		t.Errorf("auth_type = %q, want %q", params.AuthType, "key")
	}
	if params.KeyPath != "/home/admin/.ssh/id_rsa" {
		t.Errorf("key = %q, want %q", params.KeyPath, "/home/admin/.ssh/id_rsa")
	}
	if !params.JSON {
		t.Error("json should be true")
	}
}

func TestParseDeleteArgs(t *testing.T) {
	args := []string{"prod/web1", "--force", "--json"}
	params := parseDeleteArgs(args)

	if params.Path != "prod/web1" {
		t.Errorf("path = %q, want %q", params.Path, "prod/web1")
	}
	if !params.Force {
		t.Error("force should be true")
	}
	if !params.JSON {
		t.Error("json should be true")
	}
}

func TestHandleAddRejectsOverwriteWithoutForce(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	printer := cli.NewPrinter(true, &bytes.Buffer{}, &bytes.Buffer{})

	first := AddParams{Path: "prod/web", Host: "10.0.0.1", AuthType: "agent", JSON: true}
	if code := handleAdd(context.Background(), first, printer); code != cli.ExitOK {
		t.Fatalf("first add returned %d", code)
	}
	second := first
	second.Host = "10.0.0.2"
	if code := handleAdd(context.Background(), second, printer); code != cli.ExitFileOp {
		t.Fatalf("overwrite add returned %d, want %d", code, cli.ExitFileOp)
	}

	loaded, err := session.LoadSession(filepath.Join(home, ".xsc", "sessions", "prod", "web.yaml"))
	if err != nil {
		t.Fatalf("LoadSession: %v", err)
	}
	if loaded.Host != "10.0.0.1" {
		t.Fatalf("existing session was overwritten: %s", loaded.Host)
	}
}

func TestHandleEditUsesCanonicalMatchedPath(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	sessionsDir := filepath.Join(home, ".xsc", "sessions")
	originalPath := filepath.Join(sessionsDir, "prod", "web.yaml")
	if err := session.SaveSession(&session.Session{
		Host: "10.0.0.1", Port: 22, User: "root", AuthType: session.AuthTypeAgent,
	}, originalPath); err != nil {
		t.Fatalf("SaveSession: %v", err)
	}

	printer := cli.NewPrinter(true, &bytes.Buffer{}, &bytes.Buffer{})
	if code := handleEdit(context.Background(), EditParams{Path: "web", Host: "10.0.0.2"}, printer); code != cli.ExitOK {
		t.Fatalf("handleEdit returned %d", code)
	}
	if _, err := os.Stat(filepath.Join(sessionsDir, "web.yaml")); !os.IsNotExist(err) {
		t.Fatal("fuzzy edit created a duplicate root session")
	}
	updated, err := session.LoadSession(originalPath)
	if err != nil {
		t.Fatalf("LoadSession: %v", err)
	}
	if updated.Host != "10.0.0.2" {
		t.Fatalf("canonical session was not updated: %s", updated.Host)
	}
}
