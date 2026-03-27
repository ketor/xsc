package xftp

import (
	"testing"
)

// TestValidateFileName 测试路径穿越防护
func TestValidateFileName(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{"正常文件名", "myfile.txt", false},
		{"正常目录名", "mydir", false},
		{"包含斜杠", "path/file", true},
		{"包含双点前缀", "..hidden", false},
		{"纯双点", "..", true},
		{"路径穿越", "../etc/passwd", true},
		{"绝对路径", "/etc/passwd", true},
		{"中文文件名", "测试文件.txt", false},
		{"带空格文件名", "my file.txt", false},
		{"隐藏文件", ".gitignore", false},
		{"带点的文件名", "file.tar.gz", false},
		{"中间含双点", "a..b", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateFileName(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateFileName(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			}
		})
	}
}

// TestValidateFileNameEdgeCases 测试更多边界情况
func TestValidateFileNameEdgeCases(t *testing.T) {
	tests := []struct {
		input   string
		wantErr bool
	}{
		{"", false},             // 空字符串（由调用方检查）
		{".", false},            // 当前目录
		{"-", false},            // 连字符
		{"file-name_v2", false}, // 常见命名
		{"a/b/c", true},         // 多级路径
		{"...", false},           // 三个点（合法文件名）
	}

	for _, tt := range tests {
		err := validateFileName(tt.input)
		if (err != nil) != tt.wantErr {
			t.Errorf("validateFileName(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
		}
	}
}

// TestInputOpConstants 测试输入操作类型常量
func TestInputOpConstants(t *testing.T) {
	if InputOpNone != 0 {
		t.Errorf("InputOpNone 应为 0，实际 %d", InputOpNone)
	}
	if InputOpMkdir != 1 {
		t.Errorf("InputOpMkdir 应为 1，实际 %d", InputOpMkdir)
	}
	if InputOpRename != 2 {
		t.Errorf("InputOpRename 应为 2，实际 %d", InputOpRename)
	}
}

// TestYankEntryStruct 测试 yankEntry 结构体
func TestYankEntryStruct(t *testing.T) {
	entry := yankEntry{
		Name:  "test.txt",
		Path:  "/remote/test.txt",
		Size:  1024,
		IsDir: false,
	}
	if entry.Name != "test.txt" {
		t.Errorf("Name = %s, want test.txt", entry.Name)
	}
	if entry.IsDir {
		t.Error("IsDir 应为 false")
	}
}

// TestConfirmEntryStruct 测试 confirmEntry 结构体
func TestConfirmEntryStruct(t *testing.T) {
	entry := confirmEntry{
		Name:  "dir1",
		Path:  "/local/dir1",
		IsDir: true,
	}
	if entry.Name != "dir1" {
		t.Errorf("Name = %s, want dir1", entry.Name)
	}
	if !entry.IsDir {
		t.Error("IsDir 应为 true")
	}
}
