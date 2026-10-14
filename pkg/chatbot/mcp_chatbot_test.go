package chatbot

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestMCPChatBotWithMockClaudeAndServer(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 1. Create a mock MCP Server exposing a test tool
	server := mcp.NewServer(&mcp.Implementation{
		Name:    "test-research-server",
		Version: "v0.1.0",
	}, nil)

	type EchoInput struct {
		Message string `json:"message" jsonschema:"the message to echo"`
	}

	mcp.AddTool(server, &mcp.Tool{
		Name:        "echo_tool",
		Description: "Echoes a message back",
	}, func(_ context.Context, _ *mcp.CallToolRequest, in EchoInput) (*mcp.CallToolResult, any, error) {
		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: "Echo: " + in.Message},
			},
		}, nil, nil
	})

	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	go func() {
		_ = server.Run(ctx, serverTransport)
	}()

	// 2. Connect an MCP ClientSession
	client := mcp.NewClient(&mcp.Implementation{
		Name:    "test-chatbot-client",
		Version: "v0.1.0",
	}, nil)

	session, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatalf("failed to connect client to MCP server: %v", err)
	}
	defer session.Close()

	// 3. Create mock Claude API server
	callCount := 0
	mockClaude := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++

		var req RequestPayload
		_ = json.NewDecoder(r.Body).Decode(&req)

		w.Header().Set("Content-Type", "application/json")

		if callCount == 1 {
			// Turn 1: Claude asks to use echo_tool
			resp := ResponsePayload{
				ID:   "msg_mcp_1",
				Role: "assistant",
				Content: []ContentBlock{
					{
						Type: "text",
						Text: "Calling echo tool...",
					},
					{
						Type: "tool_use",
						ID:   "toolu_mcp_01",
						Name: "echo_tool",
						Input: map[string]any{
							"message": "hello MCP",
						},
					},
				},
				StopReason: "tool_use",
			}
			json.NewEncoder(w).Encode(resp)
			return
		}

		if callCount == 2 {
			// Turn 2: Verify that tool_result contains output from MCP server
			lastMsg := req.Messages[len(req.Messages)-1]
			contentBlocks, ok := lastMsg.Content.([]any)
			if !ok {
				t.Fatalf("expected lastMsg.Content to be []any, got %T", lastMsg.Content)
			}
			firstBlock := contentBlocks[0].(map[string]any)
			expectedText := "Echo: hello MCP"
			if firstBlock["content"] != expectedText {
				t.Errorf("expected tool_result content %q, got %q", expectedText, firstBlock["content"])
			}

			// Claude final answer
			resp := ResponsePayload{
				ID:   "msg_mcp_2",
				Role: "assistant",
				Content: []ContentBlock{
					{
						Type: "text",
						Text: "Echo tool executed successfully via MCP!",
					},
				},
				StopReason: "end_turn",
			}
			json.NewEncoder(w).Encode(resp)
			return
		}

		t.Fatalf("unexpected call count: %d", callCount)
	}))
	defer mockClaude.Close()

	// 4. Initialize MCPChatBot with mock Claude and MCP Session
	bot := NewMCPChatBot()
	bot.Session = session
	bot.Anthropic = &AnthropicClient{
		APIKey:     "test-key",
		BaseURL:    mockClaude.URL,
		HTTPClient: mockClaude.Client(),
	}

	// Fetch tools from MCP server
	listResp, err := session.ListTools(ctx, nil)
	if err != nil {
		t.Fatalf("failed to list tools: %v", err)
	}
	for _, tool := range listResp.Tools {
		bot.AvailableTools = append(bot.AvailableTools, Tool{
			Name:        tool.Name,
			Description: tool.Description,
			InputSchema: tool.InputSchema,
		})
	}

	// 5. Run ProcessQuery
	err = bot.ProcessQuery(ctx, "Test echo")
	if err != nil {
		t.Fatalf("ProcessQuery failed: %v", err)
	}

	if callCount != 2 {
		t.Errorf("expected 2 turns, got %d", callCount)
	}
}
