package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"mcp_project/pkg/research"
)

func main() {
	server := research.NewServer()

	// Run over standard I/O transport (stdio)
	log.Println("Starting research MCP server on stdio...")
	if err := server.Run(context.Background(), &mcp.StdioTransport{}); err != nil {
		fmt.Fprintf(os.Stderr, "Server error: %v\n", err)
		os.Exit(1)
	}
}
