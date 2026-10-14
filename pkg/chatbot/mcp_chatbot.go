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

// ServerConfig defines the command line execution configuration for an MCP server.
type ServerConfig struct {
	Command string            `json:"command"`
	Args    []string          `json:"args"`
	Env     map[string]string `json:"env,omitempty"`
}

// ConfigFile represents the server_config.json configuration structure.
type ConfigFile struct {
	MCPServers map[string]ServerConfig `json:"mcpServers"`
}

// MCPChatBot represents an MCP-enabled chatbot that talks to an LLM
// and routes tool invocations across multiple 1-to-1 MCP Server sessions.
type MCPChatBot struct {
	Sessions       []*mcp.ClientSession
	ToolToSession  map[string]*mcp.ClientSession
	Anthropic      *AnthropicClient
	AvailableTools []Tool
	Model          string
}

// NewMCPChatBot initializes a new MCPChatBot.
func NewMCPChatBot() *MCPChatBot {
	return &MCPChatBot{
		Sessions:       make([]*mcp.ClientSession, 0),
		ToolToSession:  make(map[string]*mcp.ClientSession),
		AvailableTools: make([]Tool, 0),
		Anthropic:      NewAnthropicClient(""),
		Model:          DefaultModel,
	}
}

// ConnectToServer connects to a single MCP server given its name and configuration.
func (b *MCPChatBot) ConnectToServer(ctx context.Context, serverName string, cfg ServerConfig) error {
	cmd := exec.CommandContext(ctx, cfg.Command, cfg.Args...)
	if len(cfg.Env) > 0 {
		cmd.Env = os.Environ()
		for k, v := range cfg.Env {
			cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", k, v))
		}
	}

	transport := &mcp.CommandTransport{
		Command: cmd,
	}

	client := mcp.NewClient(&mcp.Implementation{
		Name:    "mcp-chatbot",
		Version: "v0.1.0",
	}, nil)

	session, err := client.Connect(ctx, transport, nil)
	if err != nil {
		return fmt.Errorf("failed to connect to %s: %w", serverName, err)
	}

	resp, err := session.ListTools(ctx, nil)
	if err != nil {
		_ = session.Close()
		return fmt.Errorf("failed to list tools from %s: %w", serverName, err)
	}

	b.Sessions = append(b.Sessions, session)

	var toolNames []string
	for _, tool := range resp.Tools {
		toolNames = append(toolNames, tool.Name)
		b.ToolToSession[tool.Name] = session
		b.AvailableTools = append(b.AvailableTools, Tool{
			Name:        tool.Name,
			Description: tool.Description,
			InputSchema: tool.InputSchema,
		})
	}

	fmt.Printf("\nConnected to %s with tools: %v\n", serverName, toolNames)
	return nil
}

// ConnectToServers reads server_config.json and establishes 1-to-1 connections to each server.
func (b *MCPChatBot) ConnectToServers(ctx context.Context, configPath string) error {
	data, err := os.ReadFile(configPath)
	if err != nil {
		return fmt.Errorf("error loading server configuration: %w", err)
	}

	var cfg ConfigFile
	if err := json.Unmarshal(data, &cfg); err != nil {
		return fmt.Errorf("error parsing server configuration: %w", err)
	}

	for serverName, serverConfig := range cfg.MCPServers {
		if err := b.ConnectToServer(ctx, serverName, serverConfig); err != nil {
			fmt.Printf("Failed to connect to %s: %v\n", serverName, err)
		}
	}

	return nil
}

// Cleanup closes all active MCP client sessions in reverse order.
func (b *MCPChatBot) Cleanup() {
	for i := len(b.Sessions) - 1; i >= 0; i-- {
		if b.Sessions[i] != nil {
			_ = b.Sessions[i].Close()
		}
	}
}

// ProcessQuery processes a single user query through the agentic tool loop,
// routing each tool call to its corresponding MCP server session.
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

				var resultText string
				session, ok := b.ToolToSession[toolName]
				if !ok {
					resultText = fmt.Sprintf("Error: no MCP session registered for tool %q", toolName)
				} else {
					res, err := session.CallTool(ctx, &mcp.CallToolParams{
						Name:      toolName,
						Arguments: toolArgs,
					})
					if err != nil {
						resultText = fmt.Sprintf("Error calling tool %s via MCP: %v", toolName, err)
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

// SimulateQuery demonstrates multi-server MCP tool invocation without requiring an Anthropic API key.
func (b *MCPChatBot) SimulateQuery(ctx context.Context, query string) error {
	fmt.Printf("[MCP Simulation] User Query: %q\n", query)

	// Step 1: Simulate Claude calling search_papers (routed to research server)
	fmt.Println("\nAssistant: I'll search arXiv for 2 papers on 'LLM interpretability'.")
	toolName1 := "search_papers"
	toolArgs1 := map[string]any{
		"topic":       "LLM interpretability",
		"max_results": 2,
	}
	fmt.Printf("Calling tool %s with args %v\n", toolName1, toolArgs1)

	session1, ok := b.ToolToSession[toolName1]
	if !ok {
		return fmt.Errorf("no server session found for tool %q", toolName1)
	}

	res1, err := session1.CallTool(ctx, &mcp.CallToolParams{
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

	// Parse first paper ID
	var paperIDs []string
	if err := json.Unmarshal([]byte(output1), &paperIDs); err != nil || len(paperIDs) == 0 {
		paperIDs = strings.Split(output1, ", ")
	}
	firstID := strings.Trim(paperIDs[0], " \"\n\r\t")

	// Step 2: Simulate Claude calling extract_info (routed to research server)
	fmt.Printf("\nAssistant: Now retrieving detailed information for paper %s...\n", firstID)
	toolName2 := "extract_info"
	toolArgs2 := map[string]any{
		"paper_id": firstID,
	}
	fmt.Printf("Calling tool %s with args %v\n", toolName2, toolArgs2)

	session2, ok := b.ToolToSession[toolName2]
	if !ok {
		return fmt.Errorf("no server session found for tool %q", toolName2)
	}

	res2, err := session2.CallTool(ctx, &mcp.CallToolParams{
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
