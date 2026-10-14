package papers

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExtractInfo(t *testing.T) {
	// Set up temporary test topic directory inside PaperDir
	testTopicDir := filepath.Join(PaperDir, "test_topic")
	if err := os.MkdirAll(testTopicDir, 0755); err != nil {
		t.Fatalf("failed to create test directory: %v", err)
	}
	defer os.RemoveAll(testTopicDir)

	mockPaperID := "9999.99999v1"
	mockData := map[string]PaperInfo{
		mockPaperID: {
			Title:     "Test Paper Title",
			Authors:   []string{"Alice", "Bob"},
			Summary:   "A test summary.",
			PDFURL:    "https://arxiv.org/pdf/9999.99999v1",
			Published: "2025-01-01",
		},
	}

	mockJSON, err := json.MarshalIndent(mockData, "", "  ")
	if err != nil {
		t.Fatalf("failed to marshal mock data: %v", err)
	}

	mockFilePath := filepath.Join(testTopicDir, "papers_info.json")
	if err := os.WriteFile(mockFilePath, mockJSON, 0644); err != nil {
		t.Fatalf("failed to write mock file: %v", err)
	}

	// 1. Test finding an existing paper
	res := ExtractInfo(mockPaperID)
	if !strings.Contains(res, "Test Paper Title") {
		t.Errorf("expected result to contain 'Test Paper Title', got: %s", res)
	}

	// 2. Test missing paper
	missingRes := ExtractInfo("non.existent.id")
	expectedMsg := "There's no saved information related to paper non.existent.id."
	if missingRes != expectedMsg {
		t.Errorf("expected %q, got %q", expectedMsg, missingRes)
	}
}
