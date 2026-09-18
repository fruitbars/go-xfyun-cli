package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/fruitbars/go-xfyun-cli/internal/config"
	"github.com/fruitbars/go-xfyun-cli/internal/mcpserver"
	"github.com/fruitbars/go-xfyun-cli/internal/version"
)

func main() {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "--version", "version":
			fmt.Fprintln(os.Stdout, version.Current)
			return
		case "--help", "-h", "help":
			fmt.Fprintln(os.Stdout, "xfyun-ai-mcp - XFYun stdio MCP server\n\nConfigure XFYUN_APP_ID, XFYUN_API_KEY, and XFYUN_API_SECRET, then launch without arguments from an MCP host.")
			return
		default:
			fmt.Fprintf(os.Stderr, "xfyun-ai-mcp: unknown argument %q\n", os.Args[1])
			os.Exit(2)
		}
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	var credentials config.Credentials
	credentials.FromEnv()
	server := mcpserver.New(credentials)
	if err := server.Run(ctx, &mcp.StdioTransport{}); err != nil && ctx.Err() == nil {
		fmt.Fprintf(os.Stderr, "xfyun-ai-mcp: %v\n", err)
		os.Exit(1)
	}
}
