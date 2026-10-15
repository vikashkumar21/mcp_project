package papers

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// GetAvailableFolders returns a markdown formatted string of all available topic folders.
func GetAvailableFolders() string {
	var folders []string

	entries, err := os.ReadDir(PaperDir)
	if err == nil {
		for _, entry := range entries {
			if entry.IsDir() {
				infoFile := filepath.Join(PaperDir, entry.Name(), "papers_info.json")
				if _, err := os.Stat(infoFile); err == nil {
					folders = append(folders, entry.Name())
				}
			}
		}
	}

	content := "# Available Topics\n\n"
	if len(folders) > 0 {
		for _, folder := range folders {
			content += fmt.Sprintf("- %s\n", folder)
		}
		content += fmt.Sprintf("\nUse @%s to access papers in that topic.\n", folders[0])
	} else {
		content += "No topics found.\n"
	}

	return content
}

// GetTopicPapers returns markdown formatted paper details for a specific topic.
func GetTopicPapers(topic string) string {
	topicDir := strings.ToLower(strings.ReplaceAll(topic, " ", "_"))
	papersFile := filepath.Join(PaperDir, topicDir, "papers_info.json")

	data, err := os.ReadFile(papersFile)
	if err != nil {
		return fmt.Sprintf("# No papers found for topic: %s\n\nTry searching for papers on this topic first.\n", topic)
	}

	var papersData map[string]PaperInfo
	if err := json.Unmarshal(data, &papersData); err != nil {
		return fmt.Sprintf("# Error reading papers data for %s\n\nThe papers data file is corrupted.\n", topic)
	}

	words := strings.Fields(strings.ReplaceAll(topic, "_", " "))
	for i, w := range words {
		if len(w) > 0 {
			words[i] = strings.ToUpper(w[:1]) + strings.ToLower(w[1:])
		}
	}
	titleTopic := strings.Join(words, " ")

	var content strings.Builder
	fmt.Fprintf(&content, "# Papers on %s\n\nTotal papers: %d\n\n", titleTopic, len(papersData))

	for paperID, info := range papersData {
		fmt.Fprintf(&content, "## %s\n", info.Title)
		fmt.Fprintf(&content, "- **Paper ID**: %s\n", paperID)
		fmt.Fprintf(&content, "- **Authors**: %s\n", strings.Join(info.Authors, ", "))
		fmt.Fprintf(&content, "- **Published**: %s\n", info.Published)
		fmt.Fprintf(&content, "- **PDF URL**: [%s](%s)\n\n", info.PDFURL, info.PDFURL)

		summary := info.Summary
		if len(summary) > 500 {
			summary = summary[:500] + "..."
		}
		fmt.Fprintf(&content, "### Summary\n%s\n\n---\n\n", summary)
	}

	return content.String()
}
