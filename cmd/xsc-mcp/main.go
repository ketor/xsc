package main

import (
	"context"
	"log"
	"os"

	"github.com/ketor/xsc/internal/mcp"
	"github.com/ketor/xsc/pkg/version"
	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

func main() {
	// MCP Server 通过 stdio 通信，日志必须输出到 stderr
	log.SetOutput(os.Stderr)

	server := mcpsdk.NewServer(&mcpsdk.Implementation{
		Name:    "xsc-mcp",
		Version: version.Version,
	}, nil)

	// 注册所有 MCP 工具
	mcp.RegisterTools(server)

	// 通过 stdio 传输运行
	if err := server.Run(context.Background(), &mcpsdk.StdioTransport{}); err != nil {
		log.Fatalf("MCP server 错误: %v", err)
	}
}
