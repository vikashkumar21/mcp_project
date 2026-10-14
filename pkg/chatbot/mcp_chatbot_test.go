package chatbot

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestMCPChatBotMultiServerRouting(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 1. Create Server 1 (exposing echo_tool)
	server1 := mcp.NewServer(&mcp.Implementation{
		Name:    "test-server-1",
		Version: "v0.1.0",
	}, nil)

	type EchoInput struct {
		Message string `json:"message" jsonschema:"the message to echo"`
	}

	mcp.AddTool(server1, &mcp.Tool{
		Name:        "echo_tool",
		Description: "Echoes a message back",
	}, func(_ context.Context, _ *mcp.CallToolRequest, in EchoInput) (*mcp.CallToolResult, any, error) {
		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: "Server1 Echo: " + in.Message},
			},
		}, nil, nil
	})

	s1Transport, c1Transport := mcp.NewInMemoryTransports()
	go func() {
		_ = server1.Run(ctx, s1Transport)
	}()

	client1 := mcp.NewClient(&mcp.Implementation{Name: "c1", Version: "v0.1.0"}, nil)
	session1, err := client1.Connect(ctx, c1Transport, nil)
	if err != nil {
		t.Fatalf("failed to connect to server 1: %v", err)
	}

	// 2. Create Server 2 (exposing calc_tool)
	server2 := mcp.NewServer(&mcp.Implementation{
		Name:    "test-server-2",
		Version: "v0.1.0",
	}, nil)

	type CalcInput struct {
		Number int `json:"number" jsonschema:"number to double"`
	}

	mcp.AddTool(server2, &mcp.Tool{
		Name:        "calc_tool",
		Description: "Doubles a number",
	}, func(_ context.Context, _ *mcp.CallToolRequest, in CalcInput) (*mcp.CallToolResult, any, error) {
		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: "Result: 42"},
			},
		}, nil, nil
	})

	s2Transport, c2Transport := mcp.NewInMemoryTransports()
	go func() {
		_ = server2.Run(ctx, s2Transport)
	}()

	client2 := mcp.NewClient(&mcp.Implementation{Name: "c2", Version: "v0.1.0"}, nil)
	session2, err := client2.Connect(ctx, c2Transport, nil)
	if err != nil {
		t.Fatalf("failed to connect to server 2: %v", err)
	}

	// 3. Initialize MCPChatBot with both server sessions
	bot := NewMCPChatBot()
	defer bot.Cleanup()

	// Register session 1
	bot.Sessions = append(bot.Sessions, session1)
	resp1, _ := session1.ListTools(ctx, nil)
	for _, tool := range resp1.Tools {
		bot.ToolToSession[tool.Name] = session1
		bot.AvailableTools = append(bot.AvailableTools, Tool{
			Name:        tool.Name,
			Description: tool.Description,
			InputSchema: tool.InputSchema,
		})
	}

	// Register session 2
	bot.Sessions = append(bot.Sessions, session2)
	resp2, _ := session2.ListTools(ctx, nil)
	for _, tool := range resp2.Tools {
		bot.ToolToSession[tool.Name] = session2
		bot.AvailableTools = append(bot.AvailableTools, Tool{
			Name:        tool.Name,
			Description: tool.Description,
			InputSchema: tool.InputSchema,
		})
	}

	if len(bot.AvailableTools) != 2 {
		t.Fatalf("expected 2 tools aggregated from both servers, got %d", len(bot.AvailableTools))
	}

	// 4. Create mock Claude API that sequentially calls a tool from server 1, then finishes
	callCount := 0
	mockClaude := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++

		var req RequestPayload
		_ = json.NewDecoder(r.Body).Decode(&req)
		w.Header().Set("Content-Type", "application/json")

		if callCount == 1 {
			// Turn 1: Claude calls echo_tool (belonging to Server 1)
			json.NewEncoder(w).Encode(ResponsePayload{
				ID:   "msg_mcp_1",
				Role: "assistant",
				Content: []ContentBlock{
					{
						Type: "tool_use",
						ID:   "toolu_01",
						Name: "echo_tool",
						Input: map[string]any{
							"message": "hello multi-server",
						},
					},
				},
				StopReason: "tool_use",
			})
			return
		}

		if callCount == 2 {
			// Turn 2: Verify result came from Server 1
			lastMsg := req.Messages[len(req.Messages)-1]
			contentBlocks := lastMsg.Content.([]any)
			firstBlock := contentBlocks[0].(map[string]any)
			expected := "Server1 Echo: hello multi-server"
			if firstBlock["content"] != expected {
				t.Errorf("expected tool result %q, got %q", expected, firstBlock["content"])
			}

			// Final text response
			json.NewEncoder(w).Encode(ResponsePayload{
				ID:   "msg_mcp_2",
				Role: "assistant",
				Content: []ContentBlock{
					{
						Type: "text",
						Text: "Multi-server MCP tool call completed successfully!",
					},
				},
				StopReason: "end_turn",
			})
			return
		}

		t.Fatalf("unexpected call count: %d", callCount)
	}))
	defer mockClaude.Close()

	bot.Anthropic = &AnthropicClient{
		APIKey:     "test-key",
		BaseURL:    mockClaude.URL,
		HTTPClient: mockClaude.Client(),
	}

	// 5. Execute ProcessQuery
	err = bot.ProcessQuery(ctx, "Test multi server")
	if err != nil {
		t.Fatalf("ProcessQuery failed: %v", err)
	}

	if callCount != 2 {
		t.Errorf("expected 2 turns, got %d", callCount)
	}
}
