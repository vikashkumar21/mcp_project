package chatbot

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// MCPChatBot represents an MCP-enabled chatbot that talks to an LLM
// and invokes tools dynamically through an MCP Server session.
type MCPChatBot struct {
	Session        *mcp.ClientSession
	Anthropic      *AnthropicClient
	AvailableTools []Tool
	Model          string
}

// NewMCPChatBot initializes a new MCPChatBot.
func NewMCPChatBot() *MCPChatBot {
	return &MCPChatBot{
		Anthropic: NewAnthropicClient(""),
		Model:     DefaultModel,
	}
}

// ConnectToServer launches the MCP server subprocess and connects over stdio.
func (b *MCPChatBot) ConnectToServer(ctx context.Context, serverCommand string, serverArgs ...string) error {
	cmd := exec.CommandContext(ctx, serverCommand, serverArgs...)

	transport := &mcp.CommandTransport{
		Command: cmd,
	}

	client := mcp.NewClient(&mcp.Implementation{
		Name:    "mcp-chatbot",
		Version: "v0.1.0",
	}, nil)

	session, err := client.Connect(ctx, transport, nil)
	if err != nil {
		return fmt.Errorf("failed to connect to MCP server: %w", err)
	}
	b.Session = session

	// Request list of available tools from the MCP server
	resp, err := session.ListTools(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to list tools from MCP server: %w", err)
	}

	var toolNames []string
	b.AvailableTools = make([]Tool, 0, len(resp.Tools))
	for _, tool := range resp.Tools {
		toolNames = append(toolNames, tool.Name)
		b.AvailableTools = append(b.AvailableTools, Tool{
			Name:        tool.Name,
			Description: tool.Description,
			InputSchema: tool.InputSchema,
		})
	}

	fmt.Println("\nConnected to server with tools:", toolNames)
	return nil
}

// ProcessQuery processes a single user query through the agentic tool loop,
// delegating all tool calls to the MCP server session.
func (b *MCPChatBot) ProcessQuery(ctx context.Context, query string) error {
	messages := []Message{
		{Role: "user", Content: query},
	}

	const maxTurns = 10
	for turn := 0; turn < maxTurns; turn++ {
		response, err := b.Anthropic.CreateMessage(ctx, RequestPayload{
			Model:     b.Model,
			MaxTokens: 2024,
			Tools:     b.AvailableTools,
			Messages:  messages,
		})
		if err != nil {
			return fmt.Errorf("request failed on turn %d: %w", turn+1, err)
		}

		var assistantContent []ContentBlock
		var toolResults []ContentBlock
		hasToolUse := false

		for _, content := range response.Content {
			switch content.Type {
			case "text":
				fmt.Println(content.Text)
				assistantContent = append(assistantContent, content)

			case "tool_use":
				hasToolUse = true
				assistantContent = append(assistantContent, content)

				toolID := content.ID
				toolArgs := content.Input
				toolName := content.Name
				fmt.Printf("Calling tool %s with args %v\n", toolName, toolArgs)

				// Invoke tool over MCP ClientSession
				res, err := b.Session.CallTool(ctx, &mcp.CallToolParams{
					Name:      toolName,
					Arguments: toolArgs,
				})

				var resultText string
				if err != nil {
					resultText = fmt.Sprintf("Error calling tool via MCP: %v", err)
				} else {
					for _, item := range res.Content {
						if tc, ok := item.(*mcp.TextContent); ok {
							resultText += tc.Text
						} else {
							data, _ := json.Marshal(item)
							resultText += string(data)
						}
					}
				}

				toolResults = append(toolResults, ContentBlock{
					Type:      "tool_result",
					ToolUseID: toolID,
					Content:   resultText,
				})
			}
		}

		// 1 assistant message per turn with all content blocks
		messages = append(messages, Message{
			Role:    "assistant",
			Content: assistantContent,
		})

		if !hasToolUse {
			return nil
		}

		// 1 user message containing all tool_results for this turn
		messages = append(messages, Message{
			Role:    "user",
			Content: toolResults,
		})
	}

	return fmt.Errorf("reached maximum turns (%d) without completing the request", maxTurns)
}

// ChatLoop runs the interactive chat session for the MCP chatbot.
func (b *MCPChatBot) ChatLoop(ctx context.Context) {
	fmt.Println("\nMCP Chatbot Started!")
	fmt.Println("Type your queries or 'quit' to exit.")

	scanner := bufio.NewScanner(os.Stdin)
	for {
		fmt.Print("\nQuery: ")
		if !scanner.Scan() {
			break
		}

		query := strings.TrimSpace(scanner.Text())
		if strings.EqualFold(query, "quit") {
			fmt.Println("Goodbye!")
			break
		}
		if query == "" {
			continue
		}

		if err := b.ProcessQuery(ctx, query); err != nil {
			fmt.Printf("\nError: %v\n", err)
		}
		fmt.Println()
	}
}

// ConnectToServerAndRun starts the server, connects, and enters the chat loop.
func (b *MCPChatBot) ConnectToServerAndRun(ctx context.Context, serverCommand string, serverArgs ...string) error {
	if err := b.ConnectToServer(ctx, serverCommand, serverArgs...); err != nil {
		return err
	}
	defer b.Session.Close()

	b.ChatLoop(ctx)
	return nil
}

// SimulateQuery demonstrates the MCP chatbot end-to-end flow with a real MCP server session
// without requiring an Anthropic API key.
func (b *MCPChatBot) SimulateQuery(ctx context.Context, query string) error {
	fmt.Printf("[MCP Simulation] User Query: %q\n", query)

	// Step 1: Simulate Claude deciding to call search_papers
	fmt.Println("\nAssistant: I'll search arXiv for 2 papers on 'LLM interpretability'.")
	toolName1 := "search_papers"
	toolArgs1 := map[string]any{
		"topic":       "LLM interpretability",
		"max_results": 2,
	}
	fmt.Printf("Calling tool %s with args %v\n", toolName1, toolArgs1)

	// Invoke search_papers over MCP
	res1, err := b.Session.CallTool(ctx, &mcp.CallToolParams{
		Name:      toolName1,
		Arguments: toolArgs1,
	})
	if err != nil {
		return fmt.Errorf("MCP call to %s failed: %w", toolName1, err)
	}

	var output1 string
	for _, item := range res1.Content {
		if tc, ok := item.(*mcp.TextContent); ok {
			output1 += tc.Text
		}
	}
	fmt.Printf("[MCP Tool Output]: %s\n", output1)

	if strings.HasPrefix(output1, "Error") || strings.Contains(output1, "didn't return any results") {
		fmt.Printf("\nAssistant: I encountered an issue searching arXiv (%s).\n", output1)
		return nil
	}

	// Parse first paper ID from JSON array or string
	var paperIDs []string
	if err := json.Unmarshal([]byte(output1), &paperIDs); err != nil || len(paperIDs) == 0 {
		paperIDs = strings.Split(output1, ", ")
	}
	firstID := strings.Trim(paperIDs[0], " \"\n\r\t")

	// Step 2: Simulate Claude receiving paper IDs and calling extract_info over MCP
	fmt.Printf("\nAssistant: Now retrieving detailed information for paper %s...\n", firstID)
	toolName2 := "extract_info"
	toolArgs2 := map[string]any{
		"paper_id": firstID,
	}
	fmt.Printf("Calling tool %s with args %v\n", toolName2, toolArgs2)

	res2, err := b.Session.CallTool(ctx, &mcp.CallToolParams{
		Name:      toolName2,
		Arguments: toolArgs2,
	})
	if err != nil {
		return fmt.Errorf("MCP call to %s failed: %w", toolName2, err)
	}

	var output2 string
	for _, item := range res2.Content {
		if tc, ok := item.(*mcp.TextContent); ok {
			output2 += tc.Text
		}
	}
	fmt.Printf("[MCP Tool Output]:\n%s\n", output2)

	// Step 3: Assistant final turn
	fmt.Println("\nAssistant: I found the paper details via MCP and summarized them successfully!")
	return nil
}
