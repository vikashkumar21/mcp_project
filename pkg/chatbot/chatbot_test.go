package chatbot

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestProcessQuerySingleLoop(t *testing.T) {
	callCount := 0

	// Mock Anthropic Messages API server
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++

		var req RequestPayload
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("failed to decode request body: %v", err)
		}

		w.Header().Set("Content-Type", "application/json")

		if callCount == 1 {
			// Turn 1: Claude asks to use extract_info tool
			resp := ResponsePayload{
				ID:   "msg_turn_1",
				Role: "assistant",
				Content: []ContentBlock{
					{
						Type: "text",
						Text: "Looking up paper information...",
					},
					{
						Type: "tool_use",
						ID:   "toolu_test_123",
						Name: "extract_info",
						Input: map[string]any{
							"paper_id": "test.id.123",
						},
					},
				},
				StopReason: "tool_use",
			}
			json.NewEncoder(w).Encode(resp)
			return
		}

		if callCount == 2 {
			// Turn 2: Verify that client sent back tool_result block in user message
			lastMsg := req.Messages[len(req.Messages)-1]
			if lastMsg.Role != "user" {
				t.Errorf("expected last message role to be 'user', got %q", lastMsg.Role)
			}

			// Claude returns final text answer
			resp := ResponsePayload{
				ID:   "msg_turn_2",
				Role: "assistant",
				Content: []ContentBlock{
					{
						Type: "text",
						Text: "Here is the summary of the requested paper.",
					},
				},
				StopReason: "end_turn",
			}
			json.NewEncoder(w).Encode(resp)
			return
		}

		t.Fatalf("unexpected call count: %d", callCount)
	}))
	defer mockServer.Close()

	client := &AnthropicClient{
		APIKey:     "test-key",
		BaseURL:    mockServer.URL,
		HTTPClient: mockServer.Client(),
	}

	err := ProcessQuery(context.Background(), client, "Find paper test.id.123", "test-model")
	if err != nil {
		t.Fatalf("ProcessQuery failed: %v", err)
	}

	if callCount != 2 {
		t.Errorf("expected exactly 2 API turns, got %d", callCount)
	}
}
