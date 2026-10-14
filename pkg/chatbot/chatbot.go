package chatbot

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"
)

const DefaultModel = "claude-sonnet-4-6"

// ProcessQuery processes a single user query through the agentic tool-use loop.
func ProcessQuery(ctx context.Context, client *AnthropicClient, query string, model string) error {
	if model == "" {
		model = DefaultModel
	}

	messages := []Message{
		{Role: "user", Content: query},
	}

	const maxTurns = 10
	for turn := range maxTurns {
		response, err := client.CreateMessage(ctx, RequestPayload{
			Model:     model,
			MaxTokens: 2024,
			Tools:     Tools,
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
				fmt.Printf("\n[LLM Action] Calling tool %q with args: %v\n", toolName, toolArgs)

				result := ExecuteTool(toolName, toolArgs)
				toolResults = append(toolResults, ContentBlock{
					Type:      "tool_result",
					ToolUseID: toolID,
					Content:   result,
				})
			}
		}

		// 1 assistant message per turn, containing ALL content blocks (text + tool_use)
		messages = append(messages, Message{
			Role:    "assistant",
			Content: assistantContent,
		})

		// If no tools were called, Claude has provided its final response
		if !hasToolUse {
			return nil
		}

		// 1 user message containing ALL tool_results for that turn
		messages = append(messages, Message{
			Role:    "user",
			Content: toolResults,
		})
	}

	return fmt.Errorf("reached maximum turns (%d) without completing the request", maxTurns)
}

// ChatLoop runs the interactive chat session until 'quit' is typed.
func ChatLoop(client *AnthropicClient, model string) {
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

		if err := ProcessQuery(context.Background(), client, query, model); err != nil {
			fmt.Printf("\nError: %v\n", err)
		}
	}
}

// SimulateQuery demonstrates the agent tool loop without requiring a live Anthropic API key.
// It mocks the LLM response to show the complete end-to-end execution flow.
func SimulateQuery(query string) {
	fmt.Printf("[Simulation] User Query: %q\n", query)

	// Step 1: Simulate Claude deciding to call search_papers
	mockResponse1 := ResponsePayload{
		ID:   "msg_sim_1",
		Role: "assistant",
		Content: []ContentBlock{
			{
				Type: "text",
				Text: "I'll search arXiv for 2 papers on 'LLM interpretability'.",
			},
			{
				Type: "tool_use",
				ID:   "toolu_sim_01",
				Name: "search_papers",
				Input: map[string]any{
					"topic":       "LLM interpretability",
					"max_results": 2,
				},
			},
		},
		StopReason: "tool_use",
	}

	fmt.Printf("\nAssistant: %s\n", mockResponse1.Content[0].Text)
	toolBlock := mockResponse1.Content[1]
	fmt.Printf("Calling tool %s with args %v\n", toolBlock.Name, toolBlock.Input)

	// Step 2: Execute tool locally
	toolResult := ExecuteTool(toolBlock.Name, toolBlock.Input)
	fmt.Printf("[Tool Output]: %s\n", toolResult)

	// If the tool failed or returned no results, an LLM halts and reports the issue to the user
	if strings.HasPrefix(toolResult, "Error") || strings.Contains(toolResult, "didn't return any results") {
		fmt.Printf("\nAssistant: I encountered an issue while searching arXiv (%s). Please try again in a few moments.\n", toolResult)
		return
	}

	// Step 3: Simulate Claude receiving tool result and calling extract_info on the first paper
	firstID := strings.Split(toolResult, ", ")[0]
	fmt.Printf("\nAssistant: Now retrieving detailed information for paper %s...\n", firstID)

	mockResponse2 := ResponsePayload{
		ID:   "msg_sim_2",
		Role: "assistant",
		Content: []ContentBlock{
			{
				Type: "tool_use",
				ID:   "toolu_sim_02",
				Name: "extract_info",
				Input: map[string]any{
					"paper_id": firstID,
				},
			},
		},
		StopReason: "tool_use",
	}

	toolBlock2 := mockResponse2.Content[0]
	fmt.Printf("Calling tool %s with args %v\n", toolBlock2.Name, toolBlock2.Input)
	infoResult := ExecuteTool(toolBlock2.Name, toolBlock2.Input)
	fmt.Printf("[Tool Output]:\n%s\n", infoResult)

	// Step 4: Final turn
	fmt.Println("\nAssistant: I found the paper details and summarized them successfully!")
}
