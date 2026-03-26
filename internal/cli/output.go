package cli

import (
	"encoding/json"
	"fmt"
	"io"
)

type OutputFormat int

const (
	FormatText OutputFormat = iota
	FormatJSON
)

type Printer struct {
	Format OutputFormat
	Out    io.Writer // stdout
	Err    io.Writer // stderr
}

// NewPrinter 创建 Printer 实例
func NewPrinter(jsonMode bool, out, errW io.Writer) *Printer {
	f := FormatText
	if jsonMode {
		f = FormatJSON
	}
	return &Printer{Format: f, Out: out, Err: errW}
}

// Print 输出结果到 Out
func (p *Printer) Print(v any) error {
	if p.Format == FormatJSON {
		enc := json.NewEncoder(p.Out)
		enc.SetIndent("", "  ")
		return enc.Encode(v)
	}
	// text 模式：如果是 string 直接打印，否则用 fmt
	switch val := v.(type) {
	case string:
		_, err := fmt.Fprintln(p.Out, val)
		return err
	case []string:
		for _, s := range val {
			fmt.Fprintln(p.Out, s)
		}
		return nil
	default:
		_, err := fmt.Fprintf(p.Out, "%v\n", v)
		return err
	}
}

// PrintErr 输出错误到 Err（stderr）
func (p *Printer) PrintErr(e *CLIError) {
	PrintError(p.Err, e, p.Format == FormatJSON)
}

// IsJSON 返回是否 JSON 模式
func (p *Printer) IsJSON() bool {
	return p.Format == FormatJSON
}
