package main

import (
	"fmt"
	"os"

	"mcp_project/pkg/chatbot"
)

func main() {
	// Attempt to load .env if available
	chatbot.LoadDotEnv(".env")

	apiKey := os.Getenv("ANTHROPIC_API_KEY")

	if apiKey == "" {
		fmt.Println("=================================================================")
		fmt.Println("NOTE: ANTHROPIC_API_KEY is not set.")
		fmt.Println("Running in SIMULATION MODE to demonstrate the agentic tool-use loop.")
		fmt.Println("To use live Claude, add your key to a .env file or export it:")
		fmt.Println("  export ANTHROPIC_API_KEY=\"your-api-key\"")
		fmt.Println("=================================================================")

		// Run simulation of the query
		chatbot.SimulateQuery("Search for 2 papers on 'LLM interpretability'")
		return
	}

	// Live mode with Claude
	fmt.Println("Connected to Anthropic Claude.")
	client := chatbot.NewAnthropicClient(apiKey)
	chatbot.ChatLoop(client, "claude-sonnet-4-6")
}
