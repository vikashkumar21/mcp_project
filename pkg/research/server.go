package research

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

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

// NewServer creates and configures an MCP server with research tools, resources, and prompts.
func NewServer() *mcp.Server {
	server := mcp.NewServer(&mcp.Implementation{
		Name:    "research",
		Version: "v0.1.0",
	}, nil)

	// 1. Register search_papers tool
	mcp.AddTool(server, &mcp.Tool{
		Name:        "search_papers",
		Description: "Search for papers on arXiv based on a topic and store their information.",
	}, SearchPapersHandler)

	// 2. Register extract_info tool
	mcp.AddTool(server, &mcp.Tool{
		Name:        "extract_info",
		Description: "Search for information about a specific paper across all topic directories.",
	}, ExtractInfoHandler)

	// 3. Register Static Resource: papers://folders
	server.AddResource(&mcp.Resource{
		URI:         "papers://folders",
		Name:        "available_folders",
		Description: "List all available topic folders in the papers directory.",
		MIMEType:    "text/markdown",
	}, func(ctx context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
		content := papers.GetAvailableFolders()
		return &mcp.ReadResourceResult{
			Contents: []*mcp.ResourceContents{
				{
					URI:      req.Params.URI,
					MIMEType: "text/markdown",
					Text:     content,
				},
			},
		}, nil
	})

	// 4. Register Dynamic Resource Template: papers://{topic}
	server.AddResourceTemplate(&mcp.ResourceTemplate{
		URITemplate: "papers://{topic}",
		Name:        "topic_papers",
		Description: "Get detailed information about papers on a specific topic.",
		MIMEType:    "text/markdown",
	}, func(ctx context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
		topic := strings.TrimPrefix(req.Params.URI, "papers://")
		content := papers.GetTopicPapers(topic)
		return &mcp.ReadResourceResult{
			Contents: []*mcp.ResourceContents{
				{
					URI:      req.Params.URI,
					MIMEType: "text/markdown",
					Text:     content,
				},
			},
		}, nil
	})

	// 5. Register Prompt: generate_summary
	server.AddPrompt(&mcp.Prompt{
		Name:        "generate_summary",
		Description: "Generate a comprehensive summary of research papers on a specific topic.",
		Arguments: []*mcp.PromptArgument{
			{
				Name:        "topic",
				Description: "The research topic to retrieve papers for and summarize",
				Required:    true,
			},
		},
	}, func(ctx context.Context, req *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
		topic := ""
		if req.Params != nil && req.Params.Arguments != nil {
			topic = req.Params.Arguments["topic"]
		}
		if topic == "" {
			topic = "artificial intelligence"
		}

		promptText := fmt.Sprintf("Please provide a thorough research summary and analysis of recent papers on the topic '%s'. Highlight key findings, methodologies, and open challenges. If needed, use tools to search for papers or extract paper details.", topic)

		return &mcp.GetPromptResult{
			Description: fmt.Sprintf("Summary prompt for %s", topic),
			Messages: []*mcp.PromptMessage{
				{
					Role: mcp.Role("user"),
					Content: &mcp.TextContent{
						Text: promptText,
					},
				},
			},
		}, nil
	})

	return server
}
