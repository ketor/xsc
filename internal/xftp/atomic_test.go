package xftp

import (
	"os"
	"path/filepath"
	"testing"
)

func TestAtomicLocalFileCommitReplacesTarget(t *testing.T) {
	target := filepath.Join(t.TempDir(), "target.txt")
	if err := os.WriteFile(target, []byte("old"), 0600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	file, err := NewAtomicLocalFile(target)
	if err != nil {
		t.Fatalf("NewAtomicLocalFile: %v", err)
	}
	defer file.Abort()
	if _, err := file.Write([]byte("new")); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if err := file.Commit(0640); err != nil {
		t.Fatalf("Commit: %v", err)
	}
	data, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(data) != "new" {
		t.Fatalf("target = %q", data)
	}
	info, err := os.Stat(target)
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if info.Mode().Perm() != 0640 {
		t.Fatalf("mode = %o", info.Mode().Perm())
	}
}

func TestAtomicLocalFileAbortPreservesTarget(t *testing.T) {
	target := filepath.Join(t.TempDir(), "target.txt")
	if err := os.WriteFile(target, []byte("old"), 0600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	file, err := NewAtomicLocalFile(target)
	if err != nil {
		t.Fatalf("NewAtomicLocalFile: %v", err)
	}
	if _, err := file.Write([]byte("partial")); err != nil {
		t.Fatalf("Write: %v", err)
	}
	file.Abort()
	data, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(data) != "old" {
		t.Fatalf("abort changed target to %q", data)
	}
}
