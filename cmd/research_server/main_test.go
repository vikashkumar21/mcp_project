package main

import (
	"context"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestResearchServer(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	server := NewResearchServer()
	serverTransport, clientTransport := mcp.NewInMemoryTransports()

	go func() {
		_ = server.Run(ctx, serverTransport)
	}()

	client := mcp.NewClient(&mcp.Implementation{
		Name:    "test-client",
		Version: "v0.0.1",
	}, nil)

	session, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatalf("client failed to connect: %v", err)
	}
	defer session.Close()

	// 1. Verify ListTools
	toolsList, err := session.ListTools(ctx, nil)
	if err != nil {
		t.Fatalf("failed to list tools: %v", err)
	}

	if len(toolsList.Tools) != 2 {
		t.Fatalf("expected 2 tools, got %d", len(toolsList.Tools))
	}

	toolNames := map[string]bool{}
	for _, tool := range toolsList.Tools {
		toolNames[tool.Name] = true
	}
	if !toolNames["search_papers"] || !toolNames["extract_info"] {
		t.Errorf("expected search_papers and extract_info tools, got %v", toolNames)
	}

	// 2. Verify CallTool extract_info
	res, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name: "extract_info",
		Arguments: map[string]any{
			"paper_id": "test_missing_id",
		},
	})
	if err != nil {
		t.Fatalf("call extract_info failed: %v", err)
	}

	if len(res.Content) == 0 {
		t.Fatalf("expected content in result, got 0")
	}

	text := res.Content[0].(*mcp.TextContent).Text
	expectedSub := "There's no saved information related to paper test_missing_id."
	if text != expectedSub {
		t.Errorf("expected %q, got %q", expectedSub, text)
	}
}
