package xftp

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"path"
	"path/filepath"

	"github.com/pkg/sftp"
)

// AtomicRemoteFile 在目标目录写临时文件，成功后替换目标。
type AtomicRemoteFile struct {
	client *sftp.Client
	file   *sftp.File
	temp   string
	target string
	closed bool
}

func NewAtomicRemoteFile(client *sftp.Client, target string) (*AtomicRemoteFile, error) {
	if client == nil || target == "" {
		return nil, fmt.Errorf("SFTP client and target are required")
	}
	dir := path.Dir(target)
	if err := client.MkdirAll(dir); err != nil {
		return nil, fmt.Errorf("create remote directory: %w", err)
	}
	for range 8 {
		temp := path.Join(dir, ".xsc-tmp-"+randomSuffix())
		file, err := client.OpenFile(temp, os.O_WRONLY|os.O_CREATE|os.O_EXCL)
		if err == nil {
			return &AtomicRemoteFile{client: client, file: file, temp: temp, target: target}, nil
		}
		if !os.IsExist(err) {
			return nil, fmt.Errorf("create remote temporary file: %w", err)
		}
	}
	return nil, fmt.Errorf("create unique remote temporary file")
}

func (f *AtomicRemoteFile) Write(p []byte) (int, error) {
	return f.file.Write(p)
}

func (f *AtomicRemoteFile) Commit(mode os.FileMode) error {
	if !f.closed {
		if err := f.file.Close(); err != nil {
			return fmt.Errorf("close remote temporary file: %w", err)
		}
		f.closed = true
	}
	if err := f.client.Chmod(f.temp, mode.Perm()); err != nil {
		return fmt.Errorf("set remote file mode: %w", err)
	}
	if err := f.client.PosixRename(f.temp, f.target); err == nil {
		return nil
	}
	return f.commitWithRollback()
}

func (f *AtomicRemoteFile) commitWithRollback() error {
	if _, err := f.client.Lstat(f.target); err != nil {
		if os.IsNotExist(err) {
			if err := f.client.Rename(f.temp, f.target); err != nil {
				return fmt.Errorf("rename remote temporary file: %w", err)
			}
			return nil
		}
		return fmt.Errorf("inspect remote target: %w", err)
	}

	backup := f.target + ".xsc-backup-" + randomSuffix()
	if err := f.client.Rename(f.target, backup); err != nil {
		return fmt.Errorf("backup remote target: %w", err)
	}
	if err := f.client.Rename(f.temp, f.target); err != nil {
		_ = f.client.Rename(backup, f.target)
		return fmt.Errorf("replace remote target: %w", err)
	}
	if err := f.client.Remove(backup); err != nil {
		return fmt.Errorf("remove remote backup %s: %w", backup, err)
	}
	return nil
}

func (f *AtomicRemoteFile) Abort() {
	if f == nil {
		return
	}
	if !f.closed {
		_ = f.file.Close()
		f.closed = true
	}
	_ = f.client.Remove(f.temp)
}

// AtomicLocalFile 为下载提供同目录原子替换。
type AtomicLocalFile struct {
	file   *os.File
	temp   string
	target string
	closed bool
}

func NewAtomicLocalFile(target string) (*AtomicLocalFile, error) {
	if target == "" {
		return nil, fmt.Errorf("local target is required")
	}
	dir := filepath.Dir(target)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, fmt.Errorf("create local directory: %w", err)
	}
	file, err := os.CreateTemp(dir, ".xsc-tmp-*")
	if err != nil {
		return nil, fmt.Errorf("create local temporary file: %w", err)
	}
	if err := file.Chmod(0600); err != nil {
		file.Close()
		os.Remove(file.Name())
		return nil, fmt.Errorf("secure local temporary file: %w", err)
	}
	return &AtomicLocalFile{file: file, temp: file.Name(), target: target}, nil
}

func (f *AtomicLocalFile) Write(p []byte) (int, error) {
	return f.file.Write(p)
}

func (f *AtomicLocalFile) Commit(mode os.FileMode) error {
	if err := f.file.Sync(); err != nil {
		return fmt.Errorf("sync local temporary file: %w", err)
	}
	if !f.closed {
		if err := f.file.Close(); err != nil {
			return fmt.Errorf("close local temporary file: %w", err)
		}
		f.closed = true
	}
	if err := os.Chmod(f.temp, mode.Perm()); err != nil {
		return fmt.Errorf("set local file mode: %w", err)
	}
	if err := os.Rename(f.temp, f.target); err != nil {
		return fmt.Errorf("replace local target: %w", err)
	}
	return nil
}

func (f *AtomicLocalFile) Abort() {
	if f == nil {
		return
	}
	if !f.closed {
		_ = f.file.Close()
		f.closed = true
	}
	_ = os.Remove(f.temp)
}

func randomSuffix() string {
	var value [8]byte
	if _, err := rand.Read(value[:]); err != nil {
		return fmt.Sprintf("%d", os.Getpid())
	}
	return hex.EncodeToString(value[:])
}
