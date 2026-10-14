package papers

import (
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

const PaperDir = "papers"

// PaperInfo represents the metadata for a single arXiv paper.
type PaperInfo struct {
	Title     string   `json:"title"`
	Authors   []string `json:"authors"`
	Summary   string   `json:"summary"`
	PDFURL    string   `json:"pdf_url"`
	Published string   `json:"published"`
}

// Atom feed XML structs to parse arXiv API response.
type feed struct {
	XMLName xml.Name `xml:"feed"`
	Entries []entry  `xml:"entry"`
}

type entry struct {
	ID        string   `xml:"id"`
	Title     string   `xml:"title"`
	Summary   string   `xml:"summary"`
	Published string   `xml:"published"`
	Authors   []author `xml:"author"`
	Links     []link   `xml:"link"`
}

type author struct {
	Name string `xml:"name"`
}

type link struct {
	Href  string `xml:"href,attr"`
	Rel   string `xml:"rel,attr"`
	Type  string `xml:"type,attr"`
	Title string `xml:"title,attr"`
}

var multiSpaceRegex = regexp.MustCompile(`\s+`)

func cleanWhitespace(s string) string {
	return strings.TrimSpace(multiSpaceRegex.ReplaceAllString(s, " "))
}

func getShortID(rawID string) string {
	parts := strings.Split(rawID, "/abs/")
	if len(parts) > 1 {
		return parts[len(parts)-1]
	}
	return rawID
}

// SearchPapers searches for papers on arXiv based on a topic and stores their information.
//
// Args:
//
//	topic: The topic to search for
//	maxResults: Maximum number of results to retrieve (default: 5 if <= 0)
//
// Returns:
//
//	List of paper IDs found in the search and an error if any occurred.
func SearchPapers(topic string, maxResults int) ([]string, error) {
	if maxResults <= 0 {
		maxResults = 5
	}

	// Create directory for this topic: papers/<topic_normalized>
	topicDirName := strings.ReplaceAll(strings.ToLower(topic), " ", "_")
	path := filepath.Join(PaperDir, topicDirName)
	if err := os.MkdirAll(path, 0755); err != nil {
		return nil, fmt.Errorf("failed to create directory %s: %w", path, err)
	}

	filePath := filepath.Join(path, "papers_info.json")

	// Try to load existing papers info from local cache
	papersInfo := make(map[string]PaperInfo)
	if data, err := os.ReadFile(filePath); err == nil {
		_ = json.Unmarshal(data, &papersInfo)
	}

	// Build arXiv query URL
	baseURL := "https://export.arxiv.org/api/query"
	params := url.Values{}
	params.Set("search_query", topic)
	params.Set("max_results", fmt.Sprintf("%d", maxResults))
	params.Set("sortBy", "relevance")
	params.Set("sortOrder", "descending")

	reqURL := fmt.Sprintf("%s?%s", baseURL, params.Encode())

	// Execute HTTP request with custom User-Agent (recommended by arXiv API)
	req, err := http.NewRequest("GET", reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("User-Agent", "mcp-learning-client/1.0 (https://github.com/modelcontextprotocol; research-bot)")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil || resp.StatusCode != http.StatusOK {
		var errMsg string
		if err != nil {
			errMsg = err.Error()
		} else {
			body, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			errMsg = fmt.Sprintf("HTTP %d (%s)", resp.StatusCode, strings.TrimSpace(string(body)))
		}

		// If we already have saved papers for this topic, fallback to cache
		if len(papersInfo) > 0 {
			var cachedIDs []string
			for id := range papersInfo {
				cachedIDs = append(cachedIDs, id)
				if len(cachedIDs) >= maxResults {
					break
				}
			}
			fmt.Printf("[Cache Fallback] arXiv API unavailable (%s). Using %d locally saved papers.\n", errMsg, len(cachedIDs))
			return cachedIDs, nil
		}

		return nil, fmt.Errorf("arXiv API error: %s", errMsg)
	}
	defer resp.Body.Close()

	// Parse Atom XML
	var atomFeed feed
	decoder := xml.NewDecoder(resp.Body)
	if err := decoder.Decode(&atomFeed); err != nil {
		return nil, fmt.Errorf("failed to decode arXiv XML response: %w", err)
	}

	// Process each paper and add to papersInfo
	paperIDs := make([]string, 0, len(atomFeed.Entries))
	for _, item := range atomFeed.Entries {
		shortID := getShortID(item.ID)
		paperIDs = append(paperIDs, shortID)

		// Find PDF URL
		pdfURL := ""
		for _, l := range item.Links {
			if l.Title == "pdf" || l.Type == "application/pdf" {
				pdfURL = l.Href
				break
			}
		}
		if pdfURL == "" {
			pdfURL = fmt.Sprintf("https://arxiv.org/pdf/%s.pdf", shortID)
		}

		// Format published date to YYYY-MM-DD
		publishedDate := item.Published
		if t, err := time.Parse(time.RFC3339, item.Published); err == nil {
			publishedDate = t.Format("2006-01-02")
		} else if len(item.Published) >= 10 {
			publishedDate = item.Published[:10]
		}

		// Extract authors
		authors := make([]string, 0, len(item.Authors))
		for _, a := range item.Authors {
			name := strings.TrimSpace(a.Name)
			if name != "" {
				authors = append(authors, name)
			}
		}

		papersInfo[shortID] = PaperInfo{
			Title:     cleanWhitespace(item.Title),
			Authors:   authors,
			Summary:   strings.TrimSpace(item.Summary),
			PDFURL:    pdfURL,
			Published: publishedDate,
		}
	}

	// Save updated papers_info to json file
	outputData, err := json.MarshalIndent(papersInfo, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("failed to serialize papers info: %w", err)
	}

	if err := os.WriteFile(filePath, outputData, 0644); err != nil {
		return nil, fmt.Errorf("failed to write %s: %w", filePath, err)
	}

	fmt.Printf("Results are saved in: %s\n", filePath)

	return paperIDs, nil
}

// ExtractInfo searches for information about a specific paper across all topic directories.
//
// Args:
//
//	paperID: The ID of the paper to look for
//
// Returns:
//
//	JSON string with paper information if found, error message if not found
func ExtractInfo(paperID string) string {
	entries, err := os.ReadDir(PaperDir)
	if err != nil {
		return fmt.Sprintf("There's no saved information related to paper %s.", paperID)
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		itemPath := filepath.Join(PaperDir, entry.Name())
		filePath := filepath.Join(itemPath, "papers_info.json")

		fileInfo, err := os.Stat(filePath)
		if err != nil || fileInfo.IsDir() {
			continue
		}

		data, err := os.ReadFile(filePath)
		if err != nil {
			fmt.Printf("Error reading %s: %v\n", filePath, err)
			continue
		}

		var papersInfo map[string]PaperInfo
		if err := json.Unmarshal(data, &papersInfo); err != nil {
			fmt.Printf("Error reading %s: %v\n", filePath, err)
			continue
		}

		if info, ok := papersInfo[paperID]; ok {
			formatted, err := json.MarshalIndent(info, "", "  ")
			if err != nil {
				return fmt.Sprintf("Error encoding paper info: %v", err)
			}
			return string(formatted)
		}
	}

	return fmt.Sprintf("There's no saved information related to paper %s.", paperID)
}
