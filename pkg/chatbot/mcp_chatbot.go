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

// ServerConfig defines the execution or network configuration for an MCP server.
type ServerConfig struct {
	// For local stdio processes:
	Command string            `json:"command,omitempty"`
	Args    []string          `json:"args,omitempty"`
	Env     map[string]string `json:"env,omitempty"`

	// For remote HTTP servers:
	URL       string `json:"url,omitempty"`
	Transport string `json:"transport,omitempty"` // "streamable-http" (default if URL given) or "sse"
}

// ConfigFile represents the server_config.json configuration structure.
type ConfigFile struct {
	MCPServers map[string]ServerConfig `json:"mcpServers"`
}

// MCPChatBot represents an MCP-enabled chatbot that talks to an LLM
// and routes tool, resource, and prompt invocations across multiple MCP Server sessions.
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
// It supports both local processes (stdio) and remote endpoints (Streamable HTTP / SSE).
func (b *MCPChatBot) ConnectToServer(ctx context.Context, serverName string, cfg ServerConfig) error {
	var transport mcp.Transport

	if cfg.URL != "" {
		if strings.EqualFold(cfg.Transport, "sse") {
			transport = &mcp.SSEClientTransport{
				Endpoint: cfg.URL,
			}
		} else {
			// Streamable HTTP is the modern MCP standard
			transport = &mcp.StreamableClientTransport{
				Endpoint: cfg.URL,
			}
		}
	} else if cfg.Command != "" {
		cmd := exec.CommandContext(ctx, cfg.Command, cfg.Args...)
		if len(cfg.Env) > 0 {
			cmd.Env = os.Environ()
			for k, v := range cfg.Env {
				cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", k, v))
			}
		}
		transport = &mcp.CommandTransport{
			Command: cmd,
		}
	} else {
		return fmt.Errorf("server %q must configure either 'command' or 'url'", serverName)
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

// GetResource queries connected MCP servers for a given URI (e.g. papers://folders).
func (b *MCPChatBot) GetResource(ctx context.Context, uri string) (string, error) {
	for _, session := range b.Sessions {
		res, err := session.ReadResource(ctx, &mcp.ReadResourceParams{
			URI: uri,
		})
		if err == nil && len(res.Contents) > 0 {
			var sb strings.Builder
			for _, content := range res.Contents {
				sb.WriteString(content.Text)
			}
			return sb.String(), nil
		}
	}
	return "", fmt.Errorf("resource %q not found on any connected server", uri)
}

// ListPrompts queries connected MCP servers and prints available prompt templates.
func (b *MCPChatBot) ListPrompts(ctx context.Context) error {
	fmt.Println("\nAvailable Prompts:")
	foundAny := false

	for _, session := range b.Sessions {
		res, err := session.ListPrompts(ctx, nil)
		if err != nil {
			continue
		}
		for _, p := range res.Prompts {
			foundAny = true
			fmt.Printf("- %s: %s\n", p.Name, p.Description)
			if len(p.Arguments) > 0 {
				var argDescs []string
				for _, arg := range p.Arguments {
					req := ""
					if arg.Required {
						req = " (required)"
					}
					argDescs = append(argDescs, fmt.Sprintf("%s%s", arg.Name, req))
				}
				fmt.Printf("  Arguments: %s\n", strings.Join(argDescs, ", "))
			}
		}
	}

	if !foundAny {
		fmt.Println("No prompts available.")
	}
	return nil
}

// ExecutePrompt fetches a prompt template from connected servers and executes it with the LLM.
func (b *MCPChatBot) ExecutePrompt(ctx context.Context, promptName string, args map[string]string) error {
	for _, session := range b.Sessions {
		res, err := session.GetPrompt(ctx, &mcp.GetPromptParams{
			Name:      promptName,
			Arguments: args,
		})
		if err == nil && len(res.Messages) > 0 {
			var fullPrompt strings.Builder
			for _, msg := range res.Messages {
				if tc, ok := msg.Content.(*mcp.TextContent); ok {
					fullPrompt.WriteString(tc.Text)
					fullPrompt.WriteString("\n")
				}
			}
			promptQuery := strings.TrimSpace(fullPrompt.String())
			fmt.Printf("\n[Prompt Template Generated]:\n%s\n\n", promptQuery)
			return b.ProcessQuery(ctx, promptQuery)
		}
	}
	return fmt.Errorf("prompt %q not found on any connected server", promptName)
}

// ProcessQuery processes a single user query through the agentic tool loop,
// routing each tool call to its corresponding MCP server session.
func (b *MCPChatBot) ProcessQuery(ctx context.Context, query string) error {
	messages := []Message{
		{Role: "user", Content: query},
	}

	const maxTurns = 10
	for turn := range maxTurns {
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

// ChatLoop runs the interactive chat session for the MCP chatbot supporting
// slash commands (/prompts, /prompt <name>) and resource lookups (@folders, @<topic>).
func (b *MCPChatBot) ChatLoop(ctx context.Context) {
	fmt.Println("\nMCP Chatbot Started!")
	fmt.Println("Type your queries, '@folders' / '@<topic>' for resources, '/prompts' or '/prompt <name>' for prompts, or 'quit' to exit.")

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

		if after, ok := strings.CutPrefix(query, "@"); ok {
			// Resource query: @folders or @<topic>
			resourceName := after
			uri := "papers://" + resourceName
			content, err := b.GetResource(ctx, uri)
			if err != nil {
				fmt.Printf("Error fetching resource: %v\n", err)
			} else {
				fmt.Println(content)
			}
		} else if strings.EqualFold(query, "/prompts") {
			// List available prompts
			if err := b.ListPrompts(ctx); err != nil {
				fmt.Printf("Error listing prompts: %v\n", err)
			}
		} else if after0, ok0 := strings.CutPrefix(query, "/prompt "); ok0 {
			// Execute prompt template: /prompt <name> [arg1=val1 arg2=val2]
			parts := strings.Fields(after0)
			if len(parts) == 0 {
				fmt.Println("Usage: /prompt <name> [arg1=val1 arg2=val2]")
				continue
			}
			promptName := parts[0]
			args := make(map[string]string)
			for _, part := range parts[1:] {
				kv := strings.SplitN(part, "=", 2)
				if len(kv) == 2 {
					args[kv[0]] = kv[1]
				}
			}
			if err := b.ExecutePrompt(ctx, promptName, args); err != nil {
				fmt.Printf("Error executing prompt: %v\n", err)
			}
		} else {
			// Regular query processed by LLM
			if err := b.ProcessQuery(ctx, query); err != nil {
				fmt.Printf("\nError: %v\n", err)
			}
		}
		fmt.Println()
	}
}

// SimulateQuery demonstrates MCP Resources, Prompts, and Tools end-to-end without requiring an API key.
func (b *MCPChatBot) SimulateQuery(ctx context.Context, query string) error {
	fmt.Println("=================================================================")
	fmt.Println("1. DEMONSTRATING MCP RESOURCES (@folders & @<topic>)")
	fmt.Println("=================================================================")
	fmt.Println("User: @folders")
	foldersRes, err := b.GetResource(ctx, "papers://folders")
	if err != nil {
		fmt.Printf("Error getting resource papers://folders: %v\n", err)
	} else {
		fmt.Print(foldersRes)
	}

	fmt.Println("\nUser: @llm_interpretability")
	topicRes, err := b.GetResource(ctx, "papers://llm_interpretability")
	if err != nil {
		fmt.Printf("Error getting resource papers://llm_interpretability: %v\n", err)
	} else {
		// Print first 500 characters of resource output
		if len(topicRes) > 600 {
			fmt.Printf("%s\n... (truncated for display)\n", topicRes[:600])
		} else {
			fmt.Println(topicRes)
		}
	}

	fmt.Println("\n=================================================================")
	fmt.Println("2. DEMONSTRATING MCP PROMPTS (/prompts & /prompt <name>)")
	fmt.Println("=================================================================")
	fmt.Println("User: /prompts")
	_ = b.ListPrompts(ctx)

	fmt.Println("\nUser: /prompt generate_summary topic=llm_reasoning")
	for _, session := range b.Sessions {
		pRes, err := session.GetPrompt(ctx, &mcp.GetPromptParams{
			Name: "generate_summary",
			Arguments: map[string]string{
				"topic": "llm_reasoning",
			},
		})
		if err == nil && len(pRes.Messages) > 0 {
			if tc, ok := pRes.Messages[0].Content.(*mcp.TextContent); ok {
				fmt.Printf("\n[Prompt Generated from Server]:\n%s\n", tc.Text)
			}
			break
		}
	}

	fmt.Println("\n=================================================================")
	fmt.Println("3. DEMONSTRATING MCP TOOLS (search_papers & extract_info)")
	fmt.Println("=================================================================")
	fmt.Printf("[User Query]: %q\n", query)

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
