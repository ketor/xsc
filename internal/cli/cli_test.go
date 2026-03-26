package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"gopkg.in/yaml.v3"
)

// === Exit Code Tests ===

func TestExitCodes(t *testing.T) {
	tests := []struct {
		name     string
		got      int
		expected int
	}{
		{"ExitOK", ExitOK, 0},
		{"ExitUsage", ExitUsage, 2},
		{"ExitConfig", ExitConfig, 3},
		{"ExitNotFound", ExitNotFound, 4},
		{"ExitAuthFailed", ExitAuthFailed, 5},
		{"ExitConnFailed", ExitConnFailed, 6},
		{"ExitRemoteFailed", ExitRemoteFailed, 7},
		{"ExitFileOp", ExitFileOp, 8},
		{"ExitPartial", ExitPartial, 9},
		{"ExitTimeout", ExitTimeout, 124},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.got != tt.expected {
				t.Errorf("%s = %d, want %d", tt.name, tt.got, tt.expected)
			}
		})
	}
}

// === CLIError Tests ===

func TestCLIError_Error(t *testing.T) {
	e := NewCLIError(ExitNotFound, "会话未找到", "path/to/session")
	if e.Error() != "会话未找到" {
		t.Errorf("Error() = %q, want %q", e.Error(), "会话未找到")
	}
}

func TestPrintError_JSON(t *testing.T) {
	var buf bytes.Buffer
	e := NewCLIError(ExitNotFound, "not found", "detail info")
	PrintError(&buf, e, true)

	var result struct {
		Error struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
			Detail  string `json:"detail"`
		} `json:"error"`
	}
	if err := json.Unmarshal(buf.Bytes(), &result); err != nil {
		t.Fatalf("JSON 解析失败: %v", err)
	}
	if result.Error.Code != ExitNotFound {
		t.Errorf("code = %d, want %d", result.Error.Code, ExitNotFound)
	}
	if result.Error.Message != "not found" {
		t.Errorf("message = %q, want %q", result.Error.Message, "not found")
	}
	if result.Error.Detail != "detail info" {
		t.Errorf("detail = %q, want %q", result.Error.Detail, "detail info")
	}
}

func TestPrintError_Text(t *testing.T) {
	// 有 detail
	var buf bytes.Buffer
	e := NewCLIError(ExitNotFound, "not found", "some detail")
	PrintError(&buf, e, false)
	expected := "错误: not found (some detail)\n"
	if buf.String() != expected {
		t.Errorf("output = %q, want %q", buf.String(), expected)
	}

	// 无 detail
	buf.Reset()
	e2 := NewCLIError(ExitNotFound, "not found", "")
	PrintError(&buf, e2, false)
	expected2 := "错误: not found\n"
	if buf.String() != expected2 {
		t.Errorf("output = %q, want %q", buf.String(), expected2)
	}
}

// === Printer Tests ===

func TestPrinter_Print_Text_String(t *testing.T) {
	var out bytes.Buffer
	p := NewPrinter(false, &out, &bytes.Buffer{})
	p.Print("hello world")
	if out.String() != "hello world\n" {
		t.Errorf("output = %q, want %q", out.String(), "hello world\n")
	}
}

func TestPrinter_Print_Text_StringSlice(t *testing.T) {
	var out bytes.Buffer
	p := NewPrinter(false, &out, &bytes.Buffer{})
	p.Print([]string{"line1", "line2", "line3"})
	expected := "line1\nline2\nline3\n"
	if out.String() != expected {
		t.Errorf("output = %q, want %q", out.String(), expected)
	}
}

func TestPrinter_Print_Text_Struct(t *testing.T) {
	var out bytes.Buffer
	p := NewPrinter(false, &out, &bytes.Buffer{})
	data := struct{ Name string }{"test"}
	p.Print(data)
	if out.String() != "{test}\n" {
		t.Errorf("output = %q, want %q", out.String(), "{test}\n")
	}
}

func TestPrinter_Print_JSON(t *testing.T) {
	var out bytes.Buffer
	p := NewPrinter(true, &out, &bytes.Buffer{})
	data := map[string]string{"key": "value"}
	p.Print(data)

	var result map[string]string
	if err := json.Unmarshal(out.Bytes(), &result); err != nil {
		t.Fatalf("JSON 解析失败: %v", err)
	}
	if result["key"] != "value" {
		t.Errorf("key = %q, want %q", result["key"], "value")
	}
}

func TestPrinter_PrintErr_WritesToErr(t *testing.T) {
	var out, errBuf bytes.Buffer
	p := NewPrinter(false, &out, &errBuf)
	e := NewCLIError(ExitNotFound, "error msg", "")
	p.PrintErr(e)

	if out.Len() != 0 {
		t.Errorf("不应写入 stdout，但得到: %q", out.String())
	}
	if errBuf.Len() == 0 {
		t.Error("应写入 stderr，但为空")
	}
}

func TestPrinter_IsJSON(t *testing.T) {
	p1 := NewPrinter(true, &bytes.Buffer{}, &bytes.Buffer{})
	if !p1.IsJSON() {
		t.Error("IsJSON() = false, want true")
	}
	p2 := NewPrinter(false, &bytes.Buffer{}, &bytes.Buffer{})
	if p2.IsJSON() {
		t.Error("IsJSON() = true, want false")
	}
}

// === WriteYAML Tests ===

func TestWriteYAML_Normal(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.yaml")

	data := struct {
		Name string `yaml:"name"`
		Port int    `yaml:"port"`
	}{"xsc", 22}

	if err := WriteYAML(path, data); err != nil {
		t.Fatalf("WriteYAML 失败: %v", err)
	}

	// 读回验证
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("读取文件失败: %v", err)
	}

	var result struct {
		Name string `yaml:"name"`
		Port int    `yaml:"port"`
	}
	if err := yaml.Unmarshal(content, &result); err != nil {
		t.Fatalf("YAML 解析失败: %v", err)
	}
	if result.Name != "xsc" || result.Port != 22 {
		t.Errorf("内容不匹配: got %+v", result)
	}
}

func TestWriteYAML_CreatesDirectory(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sub", "dir", "test.yaml")

	data := map[string]string{"key": "value"}
	if err := WriteYAML(path, data); err != nil {
		t.Fatalf("WriteYAML 失败: %v", err)
	}

	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Error("文件未创建")
	}
}

func TestWriteYAML_FilePermission(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "perm.yaml")

	if err := WriteYAML(path, "test"); err != nil {
		t.Fatalf("WriteYAML 失败: %v", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat 失败: %v", err)
	}

	perm := info.Mode().Perm()
	if perm != 0600 {
		t.Errorf("文件权限 = %o, want 0600", perm)
	}
}
