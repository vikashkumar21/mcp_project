package chatbot

import (
	"fmt"
	"strings"

	"mcp_project/pkg/papers"
)

// Property defines a JSON schema property.
type Property struct {
	Type        string `json:"type"`
	Description string `json:"description"`
	Default     any    `json:"default,omitempty"`
}

// InputSchema defines the JSON schema for tool inputs.
type InputSchema struct {
	Type       string              `json:"type"`
	Properties map[string]Property `json:"properties"`
	Required   []string            `json:"required"`
}

// Tool defines a tool schema provided to the LLM.
type Tool struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	InputSchema any    `json:"input_schema"`
}

// Tools provides the tool schemas passed to Anthropic Claude.
var Tools = []Tool{
	{
		Name:        "search_papers",
		Description: "Search for papers on arXiv based on a topic and store their information.",
		InputSchema: InputSchema{
			Type: "object",
			Properties: map[string]Property{
				"topic": {
					Type:        "string",
					Description: "The topic to search for",
				},
				"max_results": {
					Type:        "integer",
					Description: "Maximum number of results to retrieve",
					Default:     5,
				},
			},
			Required: []string{"topic"},
		},
	},
	{
		Name:        "extract_info",
		Description: "Search for information about a specific paper across all topic directories.",
		InputSchema: InputSchema{
			Type: "object",
			Properties: map[string]Property{
				"paper_id": {
					Type:        "string",
					Description: "The ID of the paper to look for",
				},
			},
			Required: []string{"paper_id"},
		},
	},
}

// ExecuteTool dispatches tool calls to the matching function and formats the output.
func ExecuteTool(toolName string, toolArgs map[string]any) string {
	switch toolName {
	case "search_papers":
		topic, _ := toolArgs["topic"].(string)

		maxResults := 5
		if mr, ok := toolArgs["max_results"]; ok {
			switch v := mr.(type) {
			case float64:
				maxResults = int(v)
			case int:
				maxResults = v
			}
		}

		paperIDs, err := papers.SearchPapers(topic, maxResults)
		if err != nil {
			return fmt.Sprintf("Error searching papers: %v", err)
		}
		if len(paperIDs) == 0 {
			return "The operation completed but didn't return any results."
		}
		return strings.Join(paperIDs, ", ")

	case "extract_info":
		paperID, _ := toolArgs["paper_id"].(string)
		return papers.ExtractInfo(paperID)

	default:
		return fmt.Sprintf("Error: Unknown tool %q", toolName)
	}
}
