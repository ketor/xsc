package xftp

import (
	"context"
	"fmt"
	"os"
	"os/user"
	posixpath "path"
	"path/filepath"
	"strconv"
	"syscall"
	"time"

	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"

	"github.com/ketor/xsc/internal/session"
	internalssh "github.com/ketor/xsc/internal/ssh"
)

// FileInfo 文件信息
type FileInfo struct {
	Name       string
	Size       int64
	Mode       os.FileMode
	ModTime    time.Time
	IsDir      bool
	Owner      string // 远程文件有效
	Group      string // 远程文件有效
	LinkTarget string // 符号链接目标
}

// FileSystem 文件系统接口（用于浏览操作）
type FileSystem interface {
	ReadDir(path string) ([]FileInfo, error)
	Stat(path string) (*FileInfo, error)
	Mkdir(path string) error
	Remove(path string) error
	Rename(old, new string) error
	Chmod(path string, mode os.FileMode) error
	Getwd() (string, error)
}

// 编译期接口检查
var (
	_ FileSystem = (*LocalFS)(nil)
	_ FileSystem = (*RemoteFS)(nil)
)

// ============================================================
// LocalFS — 本地文件系统
// ============================================================

// LocalFS 本地文件系统实现
type LocalFS struct {
	cwd string
}

// NewLocalFS 创建本地文件系统
func NewLocalFS() (*LocalFS, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("获取工作目录失败: %w", err)
	}
	return &LocalFS{cwd: cwd}, nil
}

// ReadDir 读取目录内容
func (fs *LocalFS) ReadDir(path string) ([]FileInfo, error) {
	entries, err := os.ReadDir(path)
	if err != nil {
		return nil, fmt.Errorf("读取目录失败: %w", err)
	}

	var result []FileInfo
	for _, entry := range entries {
		info, err := entry.Info()
		if err != nil {
			continue
		}

		fi := FileInfo{
			Name:    entry.Name(),
			Size:    info.Size(),
			Mode:    info.Mode(),
			ModTime: info.ModTime(),
			IsDir:   entry.IsDir(),
		}

		// 获取 owner/group 信息
		if stat, ok := info.Sys().(*syscall.Stat_t); ok {
			fi.Owner = lookupUID(stat.Uid)
			fi.Group = lookupGID(stat.Gid)
		}

		// 处理符号链接
		if info.Mode()&os.ModeSymlink != 0 {
			fullPath := filepath.Join(path, entry.Name())
			if target, err := os.Readlink(fullPath); err == nil {
				fi.LinkTarget = target
			}
		}

		result = append(result, fi)
	}
	return result, nil
}

// Stat 获取文件信息
func (fs *LocalFS) Stat(path string) (*FileInfo, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, fmt.Errorf("获取文件信息失败: %w", err)
	}

	fi := &FileInfo{
		Name:    info.Name(),
		Size:    info.Size(),
		Mode:    info.Mode(),
		ModTime: info.ModTime(),
		IsDir:   info.IsDir(),
	}

	if stat, ok := info.Sys().(*syscall.Stat_t); ok {
		fi.Owner = lookupUID(stat.Uid)
		fi.Group = lookupGID(stat.Gid)
	}

	if info.Mode()&os.ModeSymlink != 0 {
		if target, err := os.Readlink(path); err == nil {
			fi.LinkTarget = target
		}
	}

	return fi, nil
}

// Mkdir 创建目录
func (fs *LocalFS) Mkdir(path string) error {
	if err := os.MkdirAll(path, 0755); err != nil {
		return fmt.Errorf("创建目录失败: %w", err)
	}
	return nil
}

// Remove 删除文件或目录
func (fs *LocalFS) Remove(path string) error {
	if err := os.RemoveAll(path); err != nil {
		return fmt.Errorf("删除失败: %w", err)
	}
	return nil
}

// Rename 重命名
func (fs *LocalFS) Rename(old, new string) error {
	if err := os.Rename(old, new); err != nil {
		return fmt.Errorf("重命名失败: %w", err)
	}
	return nil
}

// Chmod 修改权限
func (fs *LocalFS) Chmod(path string, mode os.FileMode) error {
	if err := os.Chmod(path, mode); err != nil {
		return fmt.Errorf("修改权限失败: %w", err)
	}
	return nil
}

// Getwd 获取当前工作目录
func (fs *LocalFS) Getwd() (string, error) {
	return fs.cwd, nil
}

// lookupUID 根据 UID 查找用户名
func lookupUID(uid uint32) string {
	u, err := user.LookupId(strconv.FormatUint(uint64(uid), 10))
	if err != nil {
		return strconv.FormatUint(uint64(uid), 10)
	}
	return u.Username
}

// lookupGID 根据 GID 查找组名
func lookupGID(gid uint32) string {
	g, err := user.LookupGroupId(strconv.FormatUint(uint64(gid), 10))
	if err != nil {
		return strconv.FormatUint(uint64(gid), 10)
	}
	return g.Name
}

// ============================================================
// RemoteFS — 远程 SFTP 文件系统
// ============================================================

// RemoteFS 远程 SFTP 文件系统实现
type RemoteFS struct {
	sftpClient *sftp.Client
	sshClient  *ssh.Client
	cleanup    func() // SSH Agent 连接等资源的清理函数
}

// NewRemoteFS 建立受 context 控制的远程文件系统连接。
func NewRemoteFS(ctx context.Context, s *session.Session) (*RemoteFS, error) {
	sshClient, cleanup, err := internalssh.DialContext(ctx, s)
	if err != nil {
		return nil, fmt.Errorf("建立 SSH 连接失败: %w", err)
	}

	type result struct {
		client *sftp.Client
		err    error
	}
	done := make(chan result, 1)
	go func() {
		client, err := sftp.NewClient(sshClient)
		done <- result{client: client, err: err}
	}()

	var sftpClient *sftp.Client
	select {
	case <-ctx.Done():
		sshClient.Close()
		<-done
		runCleanup(cleanup)
		return nil, ctx.Err()
	case created := <-done:
		if created.err != nil {
			sshClient.Close()
			runCleanup(cleanup)
			return nil, fmt.Errorf("建立 SFTP 连接失败: %w", created.err)
		}
		sftpClient = created.client
	}
	return &RemoteFS{sftpClient: sftpClient, sshClient: sshClient, cleanup: cleanup}, nil
}

func runCleanup(cleanup func()) {
	if cleanup != nil {
		cleanup()
	}
}

// SFTPClient 返回底层 sftp.Client（供传输层直接使用）
func (fs *RemoteFS) SFTPClient() *sftp.Client {
	return fs.sftpClient
}

// Close 关闭远程文件系统连接
// 按顺序关闭：sftpClient → sshClient → cleanup
func (fs *RemoteFS) Close() error {
	var firstErr error

	if fs.sftpClient != nil {
		if err := fs.sftpClient.Close(); err != nil {
			firstErr = fmt.Errorf("关闭 SFTP 客户端失败: %w", err)
		}
	}

	if fs.sshClient != nil {
		if err := fs.sshClient.Close(); err != nil && firstErr == nil {
			firstErr = fmt.Errorf("关闭 SSH 客户端失败: %w", err)
		}
	}

	if fs.cleanup != nil {
		fs.cleanup()
	}

	return firstErr
}

// ReadDir 读取远程目录内容
func (fs *RemoteFS) ReadDir(dir string) ([]FileInfo, error) {
	entries, err := fs.sftpClient.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("读取远程目录失败: %w", err)
	}

	var result []FileInfo
	for _, entry := range entries {
		fi := FileInfo{
			Name:    entry.Name(),
			Size:    entry.Size(),
			Mode:    entry.Mode(),
			ModTime: entry.ModTime(),
			IsDir:   entry.IsDir(),
		}

		// 获取 owner/group 信息
		// sftp FileInfo.Sys() 返回 *sftp.FileStat，包含 UID/GID
		if stat, ok := entry.Sys().(*sftp.FileStat); ok {
			fi.Owner = strconv.FormatUint(uint64(stat.UID), 10)
			fi.Group = strconv.FormatUint(uint64(stat.GID), 10)
		}

		// 处理符号链接
		if entry.Mode()&os.ModeSymlink != 0 {
			if target, err := fs.sftpClient.ReadLink(posixpath.Join(dir, entry.Name())); err == nil {
				fi.LinkTarget = target
			}
		}

		result = append(result, fi)
	}
	return result, nil
}

// Stat 获取远程文件信息
func (fs *RemoteFS) Stat(path string) (*FileInfo, error) {
	info, err := fs.sftpClient.Lstat(path)
	if err != nil {
		return nil, fmt.Errorf("获取远程文件信息失败: %w", err)
	}

	fi := &FileInfo{
		Name:    info.Name(),
		Size:    info.Size(),
		Mode:    info.Mode(),
		ModTime: info.ModTime(),
		IsDir:   info.IsDir(),
	}

	if stat, ok := info.Sys().(*sftp.FileStat); ok {
		fi.Owner = strconv.FormatUint(uint64(stat.UID), 10)
		fi.Group = strconv.FormatUint(uint64(stat.GID), 10)
	}

	if info.Mode()&os.ModeSymlink != 0 {
		if target, err := fs.sftpClient.ReadLink(path); err == nil {
			fi.LinkTarget = target
		}
	}

	return fi, nil
}

// Mkdir 在远程创建目录
func (fs *RemoteFS) Mkdir(path string) error {
	if err := fs.sftpClient.MkdirAll(path); err != nil {
		return fmt.Errorf("创建远程目录失败: %w", err)
	}
	return nil
}

// Remove 删除远程文件或目录
func (fs *RemoteFS) Remove(path string) error {
	// 先检查是否为目录
	info, err := fs.sftpClient.Lstat(path)
	if err != nil {
		return fmt.Errorf("删除远程文件失败: %w", err)
	}

	if info.IsDir() {
		// 递归删除目录内容
		if err := fs.removeDir(path); err != nil {
			return err
		}
		return nil
	}

	if err := fs.sftpClient.Remove(path); err != nil {
		return fmt.Errorf("删除远程文件失败: %w", err)
	}
	return nil
}

// removeDir 递归删除远程目录
func (fs *RemoteFS) removeDir(dir string) error {
	entries, err := fs.sftpClient.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("读取远程目录失败: %w", err)
	}

	for _, entry := range entries {
		fullPath := posixpath.Join(dir, entry.Name())
		if entry.IsDir() {
			if err := fs.removeDir(fullPath); err != nil {
				return err
			}
		} else {
			if err := fs.sftpClient.Remove(fullPath); err != nil {
				return fmt.Errorf("删除远程文件失败: %w", err)
			}
		}
	}

	if err := fs.sftpClient.RemoveDirectory(dir); err != nil {
		return fmt.Errorf("删除远程目录失败: %w", err)
	}
	return nil
}

// Rename 远程重命名
func (fs *RemoteFS) Rename(old, new string) error {
	if err := fs.sftpClient.Rename(old, new); err != nil {
		return fmt.Errorf("远程重命名失败: %w", err)
	}
	return nil
}

// Chmod 修改远程文件权限
func (fs *RemoteFS) Chmod(path string, mode os.FileMode) error {
	if err := fs.sftpClient.Chmod(path, mode); err != nil {
		return fmt.Errorf("修改远程权限失败: %w", err)
	}
	return nil
}

// Getwd 获取远程当前工作目录
func (fs *RemoteFS) Getwd() (string, error) {
	cwd, err := fs.sftpClient.Getwd()
	if err != nil {
		return "", fmt.Errorf("获取远程工作目录失败: %w", err)
	}
	return cwd, nil
}
