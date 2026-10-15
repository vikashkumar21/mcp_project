package main

import (
	"context"
	"strings"
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
		t.Fatalf("expected non-empty content in extract_info result")
	}

	// 3. Verify Static Resource: papers://folders
	foldersRes, err := session.ReadResource(ctx, &mcp.ReadResourceParams{
		URI: "papers://folders",
	})
	if err != nil {
		t.Fatalf("failed to read resource papers://folders: %v", err)
	}
	if len(foldersRes.Contents) == 0 || !strings.Contains(foldersRes.Contents[0].Text, "Available Topics") {
		t.Fatalf("unexpected content for papers://folders: %v", foldersRes.Contents)
	}

	// 4. Verify Dynamic Resource: papers://llm_interpretability
	topicRes, err := session.ReadResource(ctx, &mcp.ReadResourceParams{
		URI: "papers://llm_interpretability",
	})
	if err != nil {
		t.Fatalf("failed to read resource papers://llm_interpretability: %v", err)
	}
	if len(topicRes.Contents) == 0 || !strings.Contains(topicRes.Contents[0].Text, "Papers on Llm Interpretability") {
		t.Fatalf("unexpected content for papers://llm_interpretability: %v", topicRes.Contents)
	}

	// 5. Verify Prompts
	promptsList, err := session.ListPrompts(ctx, nil)
	if err != nil {
		t.Fatalf("failed to list prompts: %v", err)
	}
	if len(promptsList.Prompts) == 0 {
		t.Fatalf("expected at least 1 prompt, got 0")
	}

	promptRes, err := session.GetPrompt(ctx, &mcp.GetPromptParams{
		Name: "generate_summary",
		Arguments: map[string]string{
			"topic": "transformer architectures",
		},
	})
	if err != nil {
		t.Fatalf("failed to get prompt generate_summary: %v", err)
	}
	if len(promptRes.Messages) == 0 {
		t.Fatalf("expected prompt messages, got 0")
	}
	textContent, ok := promptRes.Messages[0].Content.(*mcp.TextContent)
	if !ok || !strings.Contains(textContent.Text, "transformer architectures") {
		t.Fatalf("prompt message did not contain topic: %v", promptRes.Messages[0].Content)
	}
}
