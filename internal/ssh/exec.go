package ssh

import (
	"bytes"
	"context"
	"fmt"
	"sync"

	"github.com/ketor/xsc/internal/session"
	gossh "golang.org/x/crypto/ssh"
)

const DefaultMaxOutputBytes int64 = 16 << 20

// CommandResult 是远程命令执行结果。Stdout 和 Stderr 分别受 MaxOutputBytes 限制。
type CommandResult struct {
	Stdout          string
	Stderr          string
	ExitCode        int
	StdoutTruncated bool
	StderrTruncated bool
}

// RunCommand 在统一的连接、超时和输出边界内执行远程命令。
func RunCommand(ctx context.Context, s *session.Session, command string, maxOutputBytes int64) (CommandResult, error) {
	if command == "" {
		return CommandResult{}, fmt.Errorf("command is required")
	}
	if maxOutputBytes <= 0 {
		maxOutputBytes = DefaultMaxOutputBytes
	}

	client, cleanup, err := DialContext(ctx, s)
	if err != nil {
		return CommandResult{}, err
	}
	defer client.Close()
	defer runCleanup(cleanup)

	sshSession, err := client.NewSession()
	if err != nil {
		return CommandResult{}, fmt.Errorf("create SSH session: %w", err)
	}
	defer sshSession.Close()

	stdout := newLimitedBuffer(maxOutputBytes)
	stderr := newLimitedBuffer(maxOutputBytes)
	sshSession.Stdout = stdout
	sshSession.Stderr = stderr

	done := make(chan error, 1)
	go func() {
		done <- sshSession.Run(command)
	}()

	var runErr error
	select {
	case runErr = <-done:
	case <-ctx.Done():
		_ = sshSession.Signal(gossh.SIGKILL)
		_ = sshSession.Close()
		client.Close()
		runErr = <-done
		if runErr == nil {
			runErr = ctx.Err()
		} else {
			runErr = ctx.Err()
		}
	}

	result := CommandResult{
		Stdout:          stdout.String(),
		Stderr:          stderr.String(),
		StdoutTruncated: stdout.Truncated(),
		StderrTruncated: stderr.Truncated(),
	}
	if runErr == nil {
		return result, nil
	}
	if exitErr, ok := runErr.(*gossh.ExitError); ok {
		result.ExitCode = exitErr.ExitStatus()
		return result, nil
	}
	return result, runErr
}

type limitedBuffer struct {
	mu        sync.Mutex
	buffer    bytes.Buffer
	remaining int64
	truncated bool
}

func newLimitedBuffer(limit int64) *limitedBuffer {
	return &limitedBuffer{remaining: limit}
}

func (b *limitedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	originalLen := len(p)
	if int64(len(p)) > b.remaining {
		p = p[:max(0, int(b.remaining))]
		b.truncated = true
	}
	if len(p) > 0 {
		_, _ = b.buffer.Write(p)
		b.remaining -= int64(len(p))
	}
	if len(p) < originalLen {
		b.truncated = true
	}
	return originalLen, nil
}

func (b *limitedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buffer.String()
}

func (b *limitedBuffer) Truncated() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.truncated
}
