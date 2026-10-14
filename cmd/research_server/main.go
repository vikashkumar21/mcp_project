package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"mcp_project/pkg/papers"
)

// SearchPapersInput defines the input parameters for search_papers tool.
type SearchPapersInput struct {
	Topic      string `json:"topic" jsonschema:"the topic to search for on arXiv"`
	MaxResults int    `json:"max_results,omitempty" jsonschema:"maximum number of results to retrieve (default: 5)"`
}

// ExtractInfoInput defines the input parameters for extract_info tool.
type ExtractInfoInput struct {
	PaperID string `json:"paper_id" jsonschema:"the ID of the paper to look for"`
}

// SearchPapersHandler handles the search_papers MCP tool call.
func SearchPapersHandler(ctx context.Context, req *mcp.CallToolRequest, in SearchPapersInput) (*mcp.CallToolResult, any, error) {
	maxResults := in.MaxResults
	if maxResults <= 0 {
		maxResults = 5
	}

	paperIDs, err := papers.SearchPapers(in.Topic, maxResults)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to search papers: %w", err)
	}

	data, _ := json.MarshalIndent(paperIDs, "", "  ")
	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: string(data)},
		},
	}, nil, nil
}

// ExtractInfoHandler handles the extract_info MCP tool call.
func ExtractInfoHandler(ctx context.Context, req *mcp.CallToolRequest, in ExtractInfoInput) (*mcp.CallToolResult, any, error) {
	info := papers.ExtractInfo(in.PaperID)
	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: info},
		},
	}, nil, nil
}

// NewResearchServer creates and configures an MCP server with research tools.
func NewResearchServer() *mcp.Server {
	server := mcp.NewServer(&mcp.Implementation{
		Name:    "research",
		Version: "v0.1.0",
	}, nil)

	// Register search_papers tool
	mcp.AddTool(server, &mcp.Tool{
		Name:        "search_papers",
		Description: "Search for papers on arXiv based on a topic and store their information.",
	}, SearchPapersHandler)

	// Register extract_info tool
	mcp.AddTool(server, &mcp.Tool{
		Name:        "extract_info",
		Description: "Search for information about a specific paper across all topic directories.",
	}, ExtractInfoHandler)

	return server
}

func main() {
	server := NewResearchServer()

	// Run over standard I/O transport (stdio)
	log.Println("Starting research MCP server on stdio...")
	if err := server.Run(context.Background(), &mcp.StdioTransport{}); err != nil {
		fmt.Fprintf(os.Stderr, "Server error: %v\n", err)
		os.Exit(1)
	}
}
