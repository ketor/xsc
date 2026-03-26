package cli

import (
	"encoding/json"
	"fmt"
	"io"
)

type CLIError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Detail  string `json:"detail,omitempty"`
}

func (e *CLIError) Error() string { return e.Message }

func NewCLIError(code int, message, detail string) *CLIError {
	return &CLIError{Code: code, Message: message, Detail: detail}
}

// PrintError 输出错误到 writer
// json=true: {"error":{"code":N,"message":"...","detail":"..."}}
// json=false: 错误: message
func PrintError(w io.Writer, e *CLIError, jsonMode bool) {
	if jsonMode {
		wrapper := struct {
			Error *CLIError `json:"error"`
		}{Error: e}
		json.NewEncoder(w).Encode(wrapper)
	} else {
		if e.Detail != "" {
			fmt.Fprintf(w, "错误: %s (%s)\n", e.Message, e.Detail)
		} else {
			fmt.Fprintf(w, "错误: %s\n", e.Message)
		}
	}
}
