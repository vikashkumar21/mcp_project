package chatbot

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

// ContentBlock represents a polymorphic content block in Anthropic Messages API.
type ContentBlock struct {
	Type      string         `json:"type"`                 // "text", "tool_use", "tool_result"
	Text      string         `json:"text,omitempty"`       // for type="text"
	ID        string         `json:"id,omitempty"`         // for type="tool_use"
	Name      string         `json:"name,omitempty"`       // for type="tool_use"
	Input     map[string]any `json:"input,omitempty"`      // for type="tool_use"
	ToolUseID string         `json:"tool_use_id,omitempty"`// for type="tool_result"
	Content   string         `json:"content,omitempty"`    // for type="tool_result"
}

// Message represents a single chat turn.
type Message struct {
	Role    string `json:"role"`    // "user" or "assistant"
	Content any    `json:"content"` // string or []ContentBlock
}

// RequestPayload represents the request body sent to Anthropic Messages API.
type RequestPayload struct {
	Model     string    `json:"model"`
	MaxTokens int       `json:"max_tokens"`
	Tools     []Tool    `json:"tools,omitempty"`
	Messages  []Message `json:"messages"`
}

// ResponsePayload represents the response body returned by Anthropic Messages API.
type ResponsePayload struct {
	ID         string         `json:"id"`
	Type       string         `json:"type"`
	Role       string         `json:"role"`
	Content    []ContentBlock `json:"content"`
	Model      string         `json:"model"`
	StopReason string         `json:"stop_reason"`
	Error      *struct {
		Type    string `json:"type"`
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

// AnthropicClient is a lightweight client for the Anthropic Messages API.
type AnthropicClient struct {
	APIKey     string
	BaseURL    string
	HTTPClient *http.Client
}

// NewAnthropicClient creates a client using the ANTHROPIC_API_KEY environment variable.
func NewAnthropicClient(apiKey string) *AnthropicClient {
	if apiKey == "" {
		apiKey = os.Getenv("ANTHROPIC_API_KEY")
	}
	return &AnthropicClient{
		APIKey:  apiKey,
		BaseURL: "https://api.anthropic.com/v1/messages",
		HTTPClient: &http.Client{
			Timeout: 60 * time.Second,
		},
	}
}

// CreateMessage sends a message request to Anthropic Messages API.
func (c *AnthropicClient) CreateMessage(ctx context.Context, payload RequestPayload) (*ResponsePayload, error) {
	if c.APIKey == "" {
		return nil, fmt.Errorf("ANTHROPIC_API_KEY is not set. Please set it in your environment or .env file")
	}

	bodyBytes, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", c.BaseURL, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", c.APIKey)
	req.Header.Set("anthropic-version", "2023-06-01")

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request to Anthropic failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	var res ResponsePayload
	if err := json.Unmarshal(respBody, &res); err != nil {
		return nil, fmt.Errorf("failed to parse response JSON: %w (raw: %s)", err, string(respBody))
	}

	if resp.StatusCode != http.StatusOK {
		errMsg := string(respBody)
		if res.Error != nil && res.Error.Message != "" {
			errMsg = res.Error.Message
		}
		return nil, fmt.Errorf("Anthropic API error (HTTP %d): %s", resp.StatusCode, errMsg)
	}

	return &res, nil
}

// LoadDotEnv loads KEY=VALUE pairs from a .env file if it exists.
func LoadDotEnv(filePath string) {
	file, err := os.Open(filePath)
	if err != nil {
		return
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) == 2 {
			k := strings.TrimSpace(parts[0])
			v := strings.TrimSpace(parts[1])
			v = strings.Trim(v, `"'`)
			if os.Getenv(k) == "" {
				os.Setenv(k, v)
			}
		}
	}
}
