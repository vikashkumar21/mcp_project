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

	// Locate research_server binary or fallback to 'go run'
	serverCmd := "./research_server"
	var serverArgs []string
	if _, err := os.Stat(serverCmd); err != nil {
		serverCmd = "go"
		serverArgs = []string{"run", "./cmd/research_server"}
	}

	fmt.Println("Starting and connecting to Research MCP Server...")
	if err := bot.ConnectToServer(ctx, serverCmd, serverArgs...); err != nil {
		fmt.Fprintf(os.Stderr, "Error connecting to MCP server: %v\n", err)
		os.Exit(1)
	}
	defer bot.Session.Close()

	apiKey := os.Getenv("ANTHROPIC_API_KEY")
	if apiKey == "" {
		fmt.Println("=================================================================")
		fmt.Println("NOTE: ANTHROPIC_API_KEY is not set.")
		fmt.Println("Running MCP ChatBot in SIMULATION MODE.")
		fmt.Println("The client will invoke tools on the real MCP server subprocess!")
		fmt.Println("To use live Claude, add your key to a .env file or export it:")
		fmt.Println("  export ANTHROPIC_API_KEY=\"your-api-key\"")
		fmt.Println("=================================================================")

		if err := bot.SimulateQuery(ctx, "Search for 2 papers on 'LLM interpretability'"); err != nil {
			fmt.Printf("Simulation error: %v\n", err)
		}
		return
	}

	// Live Claude interactive session with MCP
	fmt.Println("Connected to Anthropic Claude via MCP!")
	bot.Anthropic = chatbot.NewAnthropicClient(apiKey)
	bot.ChatLoop(ctx)
}
