package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"mcp_project/pkg/research"
)

func TestRemoteServerStreamableHTTP(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	server := research.NewServer()

	// 1. Create Streamable HTTP handler
	opts := &mcp.StreamableHTTPOptions{
		Stateless:                  false,
		DisableLocalhostProtection: true,
	}
	handler := mcp.NewStreamableHTTPHandler(func(r *http.Request) *mcp.Server {
		return server
	}, opts)

	mux := http.NewServeMux()
	mux.Handle("/mcp", handler)
	mux.Handle("/mcp/", handler)

	ts := httptest.NewServer(mux)
	defer ts.Close()

	// 2. Connect client using StreamableClientTransport
	client := mcp.NewClient(&mcp.Implementation{
		Name:    "remote-test-client",
		Version: "v0.1.0",
	}, nil)

	transport := &mcp.StreamableClientTransport{
		Endpoint: ts.URL + "/mcp",
	}

	session, err := client.Connect(ctx, transport, nil)
	if err != nil {
		t.Fatalf("failed to connect via Streamable HTTP: %v", err)
	}
	defer session.Close()

	// 3. Verify ListTools
	toolsResp, err := session.ListTools(ctx, nil)
	if err != nil {
		t.Fatalf("failed to list tools via Streamable HTTP: %v", err)
	}
	if len(toolsResp.Tools) != 2 {
		t.Errorf("expected 2 tools, got %d", len(toolsResp.Tools))
	}

	// 4. Verify CallTool
	callRes, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name: "extract_info",
		Arguments: map[string]any{
			"paper_id": "test_id_remote",
		},
	})
	if err != nil {
		t.Fatalf("failed to call tool via Streamable HTTP: %v", err)
	}
	if len(callRes.Content) == 0 {
		t.Errorf("expected non-empty tool content")
	}

	// 5. Verify ReadResource
	res, err := session.ReadResource(ctx, &mcp.ReadResourceParams{
		URI: "papers://folders",
	})
	if err != nil {
		t.Fatalf("failed to read resource via Streamable HTTP: %v", err)
	}
	if len(res.Contents) == 0 || !strings.Contains(res.Contents[0].Text, "Available Topics") {
		t.Errorf("unexpected resource content: %v", res.Contents)
	}

	// 6. Verify GetPrompt
	pRes, err := session.GetPrompt(ctx, &mcp.GetPromptParams{
		Name: "generate_summary",
		Arguments: map[string]string{
			"topic": "remote research",
		},
	})
	if err != nil {
		t.Fatalf("failed to get prompt via Streamable HTTP: %v", err)
	}
	if len(pRes.Messages) == 0 {
		t.Errorf("expected prompt messages via Streamable HTTP")
	}
}

func TestRemoteServerSSE(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	server := research.NewServer()

	// 1. Create SSE handler
	sseHandler := mcp.NewSSEHandler(func(r *http.Request) *mcp.Server {
		return server
	}, nil)

	mux := http.NewServeMux()
	mux.Handle("/sse", sseHandler)
	mux.Handle("/sse/", sseHandler)

	ts := httptest.NewServer(mux)
	defer ts.Close()

	// 2. Connect client using SSEClientTransport
	client := mcp.NewClient(&mcp.Implementation{
		Name:    "sse-test-client",
		Version: "v0.1.0",
	}, nil)

	transport := &mcp.SSEClientTransport{
		Endpoint: ts.URL + "/sse",
	}

	session, err := client.Connect(ctx, transport, nil)
	if err != nil {
		t.Fatalf("failed to connect via SSE: %v", err)
	}
	defer session.Close()

	// 3. Verify ListTools
	toolsResp, err := session.ListTools(ctx, nil)
	if err != nil {
		t.Fatalf("failed to list tools via SSE: %v", err)
	}
	if len(toolsResp.Tools) != 2 {
		t.Errorf("expected 2 tools, got %d", len(toolsResp.Tools))
	}
}
