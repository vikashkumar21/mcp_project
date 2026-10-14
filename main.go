package main

import (
	"context"
	"fmt"
	"os"

	"mcp_project/pkg/chatbot"
)

func main() {
	// Attempt to load .env if available
	chatbot.LoadDotEnv(".env")

	ctx := context.Background()
	bot := chatbot.NewMCPChatBot()

	// Connect to all configured reference and custom MCP servers from server_config.json
	fmt.Println("Connecting to MCP Servers defined in server_config.json...")
	if err := bot.ConnectToServers(ctx, "server_config.json"); err != nil {
		fmt.Fprintf(os.Stderr, "Error loading server configuration: %v\n", err)
		os.Exit(1)
	}
	defer bot.Cleanup()

	apiKey := os.Getenv("ANTHROPIC_API_KEY")
	if apiKey == "" {
		fmt.Println("=================================================================")
		fmt.Println("NOTE: ANTHROPIC_API_KEY is not set.")
		fmt.Println("Running MCP ChatBot in SIMULATION MODE.")
		fmt.Println("The client will invoke tools across the real MCP server subprocesses!")
		fmt.Println("To use live Claude, add your key to a .env file or export it:")
		fmt.Println("  export ANTHROPIC_API_KEY=\"your-api-key\"")
		fmt.Println("=================================================================")

		if err := bot.SimulateQuery(ctx, "Search for 2 papers on 'LLM interpretability'"); err != nil {
			fmt.Printf("Simulation error: %v\n", err)
		}
		return
	}

	// Live Claude interactive session with all connected MCP servers
	fmt.Println("\nConnected to Anthropic Claude via multi-server MCP!")
	bot.Anthropic = chatbot.NewAnthropicClient(apiKey)
	bot.ChatLoop(ctx)
}
