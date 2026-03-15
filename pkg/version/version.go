package version

import "fmt"

// 编译时通过 -ldflags 注入
var (
	Version   = "dev"
	GitCommit = "unknown"
	BuildDate = "unknown"
)

// String 返回格式化的版本信息
func String(name string) string {
	return fmt.Sprintf("%s version %s (commit %s, built %s)", name, Version, GitCommit, BuildDate)
}
