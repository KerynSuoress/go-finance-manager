package analyzer

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/KerynSuoress/finance-manager/internal/models"

	"github.com/joho/godotenv"
)

// ClaudeAPIResponse represents the response from Claude API
type ClaudeAPIResponse struct {
	Type    string `json:"type"`
	Content []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content"`
	Usage struct {
		InputTokens  int `json:"input_tokens"`
		OutputTokens int `json:"output_tokens"`
	} `json:"usage"`
}

// ClaudeAPIRequest represents the request to Claude API
type ClaudeAPIRequest struct {
	Model       string    `json:"model"`
	MaxTokens   int       `json:"max_tokens"`
	Messages    []Message `json:"messages"`
	Temperature float64   `json:"temperature"`
}

// Message represents a message in the Claude API conversation
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// Analyzer handles transaction categorization and analysis
type Analyzer struct {
	apiKey     string
	model      string
	maxTokens  int
	httpClient *http.Client

	// Cost-saver / debug controls
	dryRun         bool
	debugDir       string
	onlyFirstChunk bool
	maxRequests    int
	requestsMade   int
}

// NewAnalyzer creates a new analyzer instance
func NewAnalyzer() (*Analyzer, error) {
	// Load environment variables
	if err := godotenv.Load(); err != nil {
		return nil, fmt.Errorf("failed to load .env file: %v", err)
	}

	apiKey := os.Getenv("CLAUDE_API_KEY")
	if apiKey == "" {
		return nil, fmt.Errorf("CLAUDE_API_KEY environment variable is required")
	}

	// Use Claude Sonnet 4 as default model; allow override via env
	model := os.Getenv("CLAUDE_MODEL")
	if strings.TrimSpace(model) == "" {
		model = "claude-sonnet-4-20250514"
	}

	// Reasonable output token cap; allow override via env
	maxTokens := 2048
	if v := os.Getenv("CLAUDE_MAX_TOKENS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			maxTokens = n
		}
	}

	// HTTP timeout (in seconds) can be overridden via env var
	timeoutSeconds := 120
	if v := os.Getenv("CLAUDE_HTTP_TIMEOUT_SECONDS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			timeoutSeconds = n
		}
	}

	return &Analyzer{
		apiKey:         apiKey,
		model:          model,
		maxTokens:      maxTokens,
		httpClient:     &http.Client{Timeout: time.Duration(timeoutSeconds) * time.Second},
		dryRun:         false,
		debugDir:       "",
		onlyFirstChunk: false,
		maxRequests:    0,
		requestsMade:   0,
	}, nil
}

// EnableDryRun toggles dry-run mode (no external API calls; responses are mocked)
func (a *Analyzer) EnableDryRun(d bool) { a.dryRun = d }

// SetDebugDir sets a directory where request/response JSON will be saved
func (a *Analyzer) SetDebugDir(dir string) { a.debugDir = strings.TrimSpace(dir) }

// SetOnlyFirstChunk restricts processing to the first prompt chunk per file
func (a *Analyzer) SetOnlyFirstChunk(b bool) { a.onlyFirstChunk = b }

// SetMaxRequests caps total external API requests (0 = unlimited)
func (a *Analyzer) SetMaxRequests(n int) {
	if n >= 0 {
		a.maxRequests = n
	}
}

func (a *Analyzer) saveDebugFile(prefix string, data []byte) {
	if strings.TrimSpace(a.debugDir) == "" {
		return
	}
	_ = os.MkdirAll(a.debugDir, 0755)
	fname := fmt.Sprintf("%s/%s_%s.json", a.debugDir, prefix, time.Now().Format("20060102_150405_000000"))
	_ = os.WriteFile(fname, data, 0644)
}

// CategorizeTransactions uses Claude API to categorize all transactions
func (a *Analyzer) CategorizeTransactions(transactions []*models.Transaction) error {
	if len(transactions) == 0 {
		fmt.Println("No transactions to categorize")
		return nil
	}

	fmt.Printf("Categorizing %d transactions using Claude API...\n", len(transactions))

	// Process transactions in batches to avoid rate limits and token overflows
	batchSize := 30
	for i := 0; i < len(transactions); i += batchSize {
		end := i + batchSize
		if end > len(transactions) {
			end = len(transactions)
		}

		batch := transactions[i:end]
		if err := a.categorizeBatch(batch); err != nil {
			return fmt.Errorf("failed to categorize batch %d: %v", i/batchSize+1, err)
		}

		// Small delay between batches to be respectful to the API
		if end < len(transactions) {
			time.Sleep(1 * time.Second)
		}
	}

	fmt.Println("✓ Transaction categorization complete")
	return nil
}

// ExtractTransactionsFromText uses Claude API to extract transactions from raw PDF text
func (a *Analyzer) ExtractTransactionsFromText(text string, source string) ([]*models.Transaction, error) {
	fmt.Printf("Extracting transactions from PDF text using Claude API...\n")

	// For very large statements, split into manageable chunks to avoid timeouts
	chunkSize := 12000
	if v := os.Getenv("EXTRACTION_CHUNK_SIZE"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 1000 {
			chunkSize = n
		}
	}
	chunks := splitTextForExtraction(text, chunkSize)
	if a.onlyFirstChunk && len(chunks) > 1 {
		chunks = chunks[:1]
	}
	if len(chunks) > 1 {
		fmt.Printf("Large statement detected. Splitting into %d chunks...\n", len(chunks))
	}

	var allTransactions []*models.Transaction
	for i, chunk := range chunks {
		if len(chunks) > 1 {
			fmt.Printf("Processing chunk %d/%d...\n", i+1, len(chunks))
		}

		transactions, err := a.extractFromChunkRecursive(chunk, source, i+1, len(chunks), 0)
		if err != nil {
			return nil, err
		}
		allTransactions = append(allTransactions, transactions...)

		if i < len(chunks)-1 {
			time.Sleep(200 * time.Millisecond)
		}
	}

	fmt.Printf("✓ Extracted %d transactions from PDF text\n", len(allTransactions))
	return allTransactions, nil
}

// buildExtractionPrompt creates a prompt for transaction extraction from PDF text
func (a *Analyzer) buildExtractionPrompt(text string, source string) string {
	var sb strings.Builder

	sb.WriteString("You are a financial transaction extractor. Analyze the following bank statement text and extract all financial transactions.\n\n")
	sb.WriteString("For each transaction, identify:\n")
	sb.WriteString("1. Date (in YYYY-MM-DD format)\n")
	sb.WriteString("2. Description (merchant name, transaction details)\n")
	sb.WriteString("3. Amount (positive for income/credits, negative for expenses/debits)\n")
	sb.WriteString("4. Transaction type (debit/credit)\n\n")

	sb.WriteString("Statement source: " + source + "\n\n")
	sb.WriteString("Statement text:\n")
	sb.WriteString(text)
	sb.WriteString("\n\n")

	sb.WriteString("Extract all transactions and respond ONLY with a JSON array (no preface, no explanation, no code fences). Format exactly like this:\n")
	sb.WriteString("[\n")
	sb.WriteString("  {\n")
	sb.WriteString("    \"date\": \"2025-01-15\",\n")
	sb.WriteString("    \"description\": \"RESTAURANT ABC\",\n")
	sb.WriteString("    \"amount\": -125000.00,\n")
	sb.WriteString("    \"type\": \"debit\"\n")
	sb.WriteString("  }\n")
	sb.WriteString("]\n\n")

	sb.WriteString("CRITICAL RULES:\n")
	sb.WriteString("- Handle Colombian Peso (COP) amounts with comma as decimal separator (e.g., 125.000,50)\n")
	sb.WriteString("- Convert amounts to standard format (e.g., 125000.50)\n")
	sb.WriteString("- Only extract actual financial transactions, not summary information\n")
	sb.WriteString("- If you see duplicate transactions with opposite signs for the same merchant on the same date, only include the NET transaction\n")
	sb.WriteString("- For example: if you see 'RESTAURANT ABC -1000' and 'RESTAURANT ABC +1000' on the same date, skip both\n")
	sb.WriteString("- If you see 'RESTAURANT ABC -1000' and 'RESTAURANT ABC +500' on the same date, include only the net: 'RESTAURANT ABC -500'\n")
	sb.WriteString("- If no transactions are found, return an empty array []\n")
	sb.WriteString("- Do not include any commentary, headings, or markdown. Output must be a JSON array only.\n")

	return sb.String()
}

// buildExtractionPromptWithChunk is like buildExtractionPrompt but adds chunk context
func (a *Analyzer) buildExtractionPromptWithChunk(text string, source string, chunkIndex int, totalChunks int) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("You are extracting transactions from a bank statement. This is chunk %d of %d. Only extract transactions that appear in this chunk. Do not infer transactions from other parts.\n\n", chunkIndex, totalChunks))
	sb.WriteString(a.buildExtractionPrompt(text, source))
	return sb.String()
}

// extractFromChunkRecursive attempts to extract transactions from a text chunk.
// If the model returns non-JSON output, it splits the chunk and retries recursively up to a small depth.
func (a *Analyzer) extractFromChunkRecursive(chunk string, source string, chunkIndex int, totalChunks int, depth int) ([]*models.Transaction, error) {
	// Safety: limit recursion depth
	if depth > 3 {
		return nil, fmt.Errorf("failed to parse extraction response after multiple attempts")
	}

	prompt := a.buildExtractionPromptWithChunk(chunk, source, chunkIndex, totalChunks)
	request := ClaudeAPIRequest{
		Model:       a.model,
		MaxTokens:   a.maxTokens,
		Temperature: 0.1,
		Messages: []Message{{
			Role:    "user",
			Content: prompt,
		}},
	}

	response, err := a.callClaudeAPI(request)
	if err != nil {
		return nil, fmt.Errorf("failed to call Claude API: %v", err)
	}

	transactions, parseErr := a.parseExtractionResponse(response, source)
	if parseErr == nil {
		return transactions, nil
	}

	// If parse failed and the chunk is large enough, split and retry recursively
	if len(chunk) > 6000 {
		fmt.Printf("Parse failed on chunk (len=%d). Splitting and retrying...\n", len(chunk))
		mid := len(chunk) / 2
		left := chunk[:mid]
		right := chunk[mid:]
		leftTx, _ := a.extractFromChunkRecursive(left, source, chunkIndex, totalChunks, depth+1)
		rightTx, _ := a.extractFromChunkRecursive(right, source, chunkIndex, totalChunks, depth+1)
		combined := append(leftTx, rightTx...)
		if len(combined) > 0 {
			return combined, nil
		}
	}

	// Log a small snippet for diagnostics
	if len(response.Content) > 0 {
		msg := response.Content[0].Text
		if len(msg) > 200 {
			msg = msg[:200]
		}
		fmt.Printf("Model returned non-JSON output (first 200 chars): %s\n", strings.ReplaceAll(msg, "\n", " "))
	}

	return nil, fmt.Errorf("failed to parse extraction response: %v", parseErr)
}

// Add this function to extract JSON from Claude's response
func extractJSONFromResponse(content string) (string, error) {
	content = strings.TrimSpace(content)

	// Helper: extract first well-formed JSON array by bracket matching, ignoring text inside quotes
	findJSONArray := func(s string) (string, bool) {
		inString := false
		escaped := false
		depth := 0
		start := -1
		for i, r := range s {
			ch := byte(r)
			if inString {
				if escaped {
					escaped = false
					continue
				}
				if ch == '\\' {
					escaped = true
					continue
				}
				if ch == '"' {
					inString = false
				}
				continue
			}
			if ch == '"' {
				inString = true
				continue
			}
			if ch == '[' {
				if depth == 0 {
					start = i
				}
				depth++
			} else if ch == ']' {
				if depth > 0 {
					depth--
					if depth == 0 && start != -1 {
						return s[start : i+1], true
					}
				}
			}
		}
		return "", false
	}

	if arr, ok := findJSONArray(content); ok {
		return arr, nil
	}

	// Fallback: look inside fenced code blocks
	if start := strings.Index(content, "```json"); start != -1 {
		if end := strings.Index(content[start+7:], "```"); end != -1 { // 7 = len("```json")
			block := content[start+7 : start+7+end]
			if arr, ok := findJSONArray(block); ok {
				return arr, nil
			}
		}
	}
	if start := strings.Index(content, "```"); start != -1 {
		if end := strings.Index(content[start+3:], "```"); end != -1 { // 3 = len("```")
			block := content[start+3 : start+3+end]
			if arr, ok := findJSONArray(block); ok {
				return arr, nil
			}
		}
	}

	return "", fmt.Errorf("no JSON array found in response")
}

// buildJSONArrayFromObjects scans the content for standalone JSON objects and
// returns a synthesized JSON array string composed of those objects.
// This is a last-resort fallback when the model outputs multiple objects without wrapping them in an array.
func buildJSONArrayFromObjects(content string) (string, error) {
	// Strip code fences if present to reduce noise
	cleaned := content
	if i := strings.Index(cleaned, "```"); i != -1 {
		// remove all fenced blocks
		re := regexp.MustCompile("```[a-zA-Z]*[\\s\\S]*?```")
		cleaned = re.ReplaceAllString(cleaned, "")
	}

	inString := false
	escaped := false
	depth := 0
	start := -1
	var objects []string
	for i := 0; i < len(cleaned); i++ {
		ch := cleaned[i]
		if inString {
			if escaped {
				escaped = false
				continue
			}
			if ch == '\\' {
				escaped = true
				continue
			}
			if ch == '"' {
				inString = false
			}
			continue
		}
		if ch == '"' {
			inString = true
			continue
		}
		if ch == '{' {
			if depth == 0 {
				start = i
			}
			depth++
		} else if ch == '}' {
			if depth > 0 {
				depth--
				if depth == 0 && start != -1 {
					objects = append(objects, cleaned[start:i+1])
					start = -1
				}
			}
		}
	}

	if len(objects) == 0 {
		return "", fmt.Errorf("no standalone JSON objects found")
	}

	// Join objects into a JSON array
	array := "[" + strings.Join(objects, ",") + "]"
	// quick sanity check
	var tmp []map[string]interface{}
	if err := json.Unmarshal([]byte(array), &tmp); err != nil {
		return "", fmt.Errorf("failed to validate synthesized JSON array: %v", err)
	}
	return array, nil
}

// parseExtractionResponse parses the Claude API response to extract transactions
func (a *Analyzer) parseExtractionResponse(response *ClaudeAPIResponse, source string) ([]*models.Transaction, error) {
	if len(response.Content) == 0 {
		return nil, fmt.Errorf("no content in API response")
	}

	// Extract the text content from the response
	content := response.Content[0].Text

	// Extract just the JSON part
	jsonContent, err := extractJSONFromResponse(content)
	if err != nil {
		// Fallback: try to build an array from individual JSON objects
		if fallback, ferr := buildJSONArrayFromObjects(content); ferr == nil {
			jsonContent = fallback
		} else {
			return nil, fmt.Errorf("failed to extract JSON from response: %v", err)
		}
	}

	// Try to parse as JSON
	var transactions []struct {
		Date        string  `json:"date"`
		Description string  `json:"description"`
		Amount      float64 `json:"amount"`
		Type        string  `json:"type"`
	}

	if err := json.Unmarshal([]byte(jsonContent), &transactions); err != nil {
		return nil, fmt.Errorf("failed to parse JSON response: %v\nJSON content: %s", err, jsonContent)
	}

	// Convert to our Transaction model
	var result []*models.Transaction
	for _, t := range transactions {
		// Parse the date
		date, err := time.Parse("2006-01-02", t.Date)
		if err != nil {
			fmt.Printf("Warning: Could not parse date '%s', skipping transaction\n", t.Date)
			continue
		}

		// Determine transaction type
		transactionType := models.Debit
		if t.Type == "credit" {
			transactionType = models.Credit
		}

		transaction := &models.Transaction{
			Date:        date,
			Description: t.Description,
			Amount:      t.Amount,
			Type:        transactionType,
			Source:      source,
			RawText:     t.Description, // Use description as raw text for now
		}

		result = append(result, transaction)
	}

	return result, nil
}

// categorizeBatch categorizes a batch of transactions
func (a *Analyzer) categorizeBatch(transactions []*models.Transaction) error {
	// Build the prompt for categorization
	prompt := a.buildCategorizationPrompt(transactions)

	// Create the API request
	request := ClaudeAPIRequest{
		Model:       a.model, // use same configured model for consistency
		MaxTokens:   a.maxTokens,
		Temperature: 0.2, // Slightly lower temp for deterministic labels
		Messages: []Message{
			{
				Role:    "user",
				Content: prompt,
			},
		},
	}

	// Make the API call
	response, err := a.callClaudeAPI(request)
	if err != nil {
		return err
	}

	// Parse the response and update transactions
	return a.parseCategorizationResponse(response, transactions)
}

// buildCategorizationPrompt creates a prompt for transaction categorization
func (a *Analyzer) buildCategorizationPrompt(transactions []*models.Transaction) string {
	var sb strings.Builder

	sb.WriteString("You are a financial transaction categorizer. Analyze the following transactions and categorize each one with a main category and subcategory. If you are not completely sure about the category, use 'Other'. Use standard financial categories like:\n\n")
	sb.WriteString("- Food & Dining (Restaurants, Groceries, Fast Food, Coffee)\n")
	sb.WriteString("- Transportation (Gas, Public Transit, Ride Sharing, Parking)\n")
	sb.WriteString("- Shopping (Clothing, Electronics, Home & Garden, Online Shopping)\n")
	sb.WriteString("- Entertainment (Movies, Games, Streaming Services, Events)\n")
	sb.WriteString("- Health & Fitness (Medical, Gym, Pharmacy, Wellness)\n")
	sb.WriteString("- Bills & Utilities (Electricity, Water, Internet, Phone)\n")
	sb.WriteString("- Income (Salary, Freelance, Investment, Refunds)\n")
	sb.WriteString("- Banking (ATM, Fees, Transfers)\n")
	sb.WriteString("- Travel (Flights, Hotels, Car Rental, Tourism)\n")
	sb.WriteString("- Education (Tuition, Books, Courses)\n")
	sb.WriteString("- Insurance (Health, Auto, Home, Life)\n")
	sb.WriteString("- Other (Uncategorized)\n\n")

	sb.WriteString("For each transaction, provide:\n")
	sb.WriteString("1. Main category (from the list above)\n")
	sb.WriteString("2. Subcategory (specific type within the main category)\n")
	sb.WriteString("3. Confidence level (0.0 to 1.0, where 1.0 is very confident)\n\n")

	sb.WriteString("Respond ONLY with a JSON array (no preface, no explanation). Format like this:\n")
	sb.WriteString("[\n")
	sb.WriteString("  {\n")
	sb.WriteString("    \"index\": 0,\n")
	sb.WriteString("    \"category\": \"Food & Dining\",\n")
	sb.WriteString("    \"subcategory\": \"Restaurants\",\n")
	sb.WriteString("    \"confidence\": 0.95\n")
	sb.WriteString("  }\n")
	sb.WriteString("]\n\n")

	sb.WriteString("Here are the transactions to categorize in index order (use the index to map your output):\n\n")

	for i, tx := range transactions {
		sb.WriteString(fmt.Sprintf("%d. Date: %s | Description: %s | Amount: %.2f | Type: %s\n",
			i, tx.Date.Format("2006-01-02"), tx.Description, tx.Amount, tx.Type))
	}

	return sb.String()
}

// callClaudeAPI makes a request to the Claude API
func (a *Analyzer) callClaudeAPI(request ClaudeAPIRequest) (*ClaudeAPIResponse, error) {
	jsonData, err := json.Marshal(request)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %v", err)
	}

	// Cost-control: respect max requests cap
	if a.maxRequests > 0 && a.requestsMade >= a.maxRequests {
		return nil, fmt.Errorf("request limit reached (%d)", a.maxRequests)
	}
	a.requestsMade++

	// Debug: save request
	a.saveDebugFile("request", jsonData)

	if a.dryRun {
		// Return an empty array as a harmless mock to test the pipeline without cost
		mock := &ClaudeAPIResponse{
			Type: "message",
			Content: []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			}{
				{Type: "text", Text: "[]"},
			},
		}
		respBytes, _ := json.Marshal(mock)
		a.saveDebugFile("response_mock", respBytes)
		return mock, nil
	}

	var lastErr error
	for attempt := 1; attempt <= 3; attempt++ {
		req, err := http.NewRequest("POST", "https://api.anthropic.com/v1/messages", bytes.NewBuffer(jsonData))
		if err != nil {
			return nil, fmt.Errorf("failed to create request: %v", err)
		}

		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("x-api-key", a.apiKey)
		req.Header.Set("anthropic-version", "2023-06-01")

		resp, err := a.httpClient.Do(req)
		if err != nil {
			var netErr net.Error
			if errors.As(err, &netErr) && netErr.Timeout() && attempt < 3 {
				backoff := time.Duration(attempt*2) * time.Second
				fmt.Printf("Transient timeout from Claude API (attempt %d). Retrying in %s...\n", attempt, backoff)
				time.Sleep(backoff)
				lastErr = err
				continue
			}
			return nil, fmt.Errorf("failed to make API request: %v", err)
		}

		body, readErr := io.ReadAll(resp.Body)
		resp.Body.Close()
		if readErr != nil {
			return nil, fmt.Errorf("failed to read response body: %v", readErr)
		}

		if resp.StatusCode == http.StatusOK {
			var response ClaudeAPIResponse
			if err := json.Unmarshal(body, &response); err != nil {
				return nil, fmt.Errorf("failed to decode response: %v", err)
			}
			// Debug: save response
			a.saveDebugFile("response", body)
			return &response, nil
		}

		// Retry on 429/5xx
		if (resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500) && attempt < 3 {
			backoff := time.Duration(attempt*2) * time.Second
			fmt.Printf("Claude API returned status %d (attempt %d). Retrying in %s...\n", resp.StatusCode, attempt, backoff)
			time.Sleep(backoff)
			lastErr = fmt.Errorf("status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
			continue
		}

		return nil, fmt.Errorf("API request failed with status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	if lastErr == nil {
		lastErr = fmt.Errorf("unknown error during Claude API call")
	}
	return nil, lastErr
}

// splitTextForExtraction splits text into chunks not exceeding approx chunkSize runes.
// It prefers to split on page boundaries marked by lines beginning with "--- Page ".
func splitTextForExtraction(text string, chunkSize int) []string {
	if len(text) <= chunkSize {
		return []string{text}
	}
	lines := strings.Split(text, "\n")
	var chunks []string
	var current strings.Builder
	for _, line := range lines {
		// If adding this line would exceed the chunk size and we are at a page boundary, start a new chunk
		if current.Len()+len(line)+1 > chunkSize && strings.HasPrefix(strings.TrimSpace(line), "--- Page ") {
			chunks = append(chunks, current.String())
			current.Reset()
		}
		if current.Len() > 0 {
			current.WriteString("\n")
		}
		current.WriteString(line)
	}
	if current.Len() > 0 {
		chunks = append(chunks, current.String())
	}
	// Final safety: if any chunk is still too big, hard-split it
	var final []string
	for _, c := range chunks {
		if len(c) <= chunkSize {
			final = append(final, c)
			continue
		}
		for start := 0; start < len(c); start += chunkSize {
			end := start + chunkSize
			if end > len(c) {
				end = len(c)
			}
			final = append(final, c[start:end])
		}
	}
	return final
}

// CategorizationResult represents a single categorization result
type CategorizationResult struct {
	Index       int     `json:"index"`
	Category    string  `json:"category"`
	Subcategory string  `json:"subcategory"`
	Confidence  float64 `json:"confidence"`
}

// parseCategorizationResponse parses the Claude API response and updates transactions
func (a *Analyzer) parseCategorizationResponse(response *ClaudeAPIResponse, transactions []*models.Transaction) error {
	if len(response.Content) == 0 {
		return fmt.Errorf("no content in API response")
	}

	// Extract the text content
	content := response.Content[0].Text

	// Apply the same JSON extraction as we did for transactions
	jsonContent, err := extractJSONFromResponse(content)
	if err != nil {
		return fmt.Errorf("failed to extract JSON from categorization response: %v", err)
	}

	fmt.Printf("DEBUG - Extracted categorization JSON: %s\n", jsonContent) // Temporary debug

	// Try to parse as JSON
	var results []CategorizationResult
	if err := json.Unmarshal([]byte(jsonContent), &results); err != nil {
		return fmt.Errorf("failed to parse categorization response: %v\nJSON content: %s", err, jsonContent)
	}

	// Update transactions with categorization results
	for _, result := range results {
		if result.Index >= 0 && result.Index < len(transactions) {
			tx := transactions[result.Index]
			tx.Category = result.Category
			tx.Subcategory = result.Subcategory
			tx.Confidence = result.Confidence
		}
	}

	return nil
}

// GenerateReports creates analysis reports from categorized transactions
func (a *Analyzer) GenerateReports(transactions []*models.Transaction, outputDir string) error {
	if len(transactions) == 0 {
		return fmt.Errorf("no transactions to analyze")
	}

	// Create output directory if it doesn't exist
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return fmt.Errorf("failed to create output directory: %v", err)
	}

	// Generate CSV report
	if err := a.generateCSVReport(transactions, outputDir); err != nil {
		return fmt.Errorf("failed to generate CSV report: %v", err)
	}

	// Generate summary report
	if err := a.generateSummaryReport(transactions, outputDir); err != nil {
		return fmt.Errorf("failed to generate summary report: %v", err)
	}

	fmt.Printf("✓ Reports generated in %s\n", outputDir)
	return nil
}

// generateCSVReport creates a CSV file with all transactions
func (a *Analyzer) generateCSVReport(transactions []*models.Transaction, outputDir string) error {
	filename := fmt.Sprintf("transactions_%s.csv", time.Now().Format("20060102"))
	filepath := fmt.Sprintf("%s/%s", outputDir, filename)

	file, err := os.Create(filepath)
	if err != nil {
		return err
	}
	defer file.Close()

	// Write CSV header
	file.WriteString("Date,Description,Amount,Type,Category,Subcategory,Confidence,Source\n")

	// Write transaction data
	for _, tx := range transactions {
		line := fmt.Sprintf("%s,\"%s\",%.2f,%s,%s,%s,%.2f,%s\n",
			tx.Date.Format("2006-01-02"),
			strings.ReplaceAll(tx.Description, "\"", "\"\""), // Escape quotes
			tx.Amount,
			tx.Type,
			tx.Category,
			tx.Subcategory,
			tx.Confidence,
			tx.Source)
		file.WriteString(line)
	}

	return nil
}

// generateSummaryReport creates a summary analysis report
func (a *Analyzer) generateSummaryReport(transactions []*models.Transaction, outputDir string) error {
	filename := fmt.Sprintf("summary_%s.txt", time.Now().Format("20060102"))
	filepath := fmt.Sprintf("%s/%s", outputDir, filename)

	file, err := os.Create(filepath)
	if err != nil {
		return err
	}
	defer file.Close()

	// Calculate summary statistics
	summary := a.calculateSummary(transactions)

	// Write summary report
	file.WriteString("FINANCIAL ANALYSIS SUMMARY\n")
	file.WriteString("=========================\n\n")
	file.WriteString(fmt.Sprintf("Analysis Date: %s\n", time.Now().Format("2006-01-02 15:04:05")))
	file.WriteString(fmt.Sprintf("Total Transactions: %d\n", len(transactions)))
	file.WriteString(fmt.Sprintf("Date Range: %s to %s\n\n", summary.StartDate, summary.EndDate))

	file.WriteString("SPENDING BY CATEGORY\n")
	file.WriteString("===================\n")
	for category, amount := range summary.CategoryTotals {
		file.WriteString(fmt.Sprintf("%s: $%.2f\n", category, amount))
	}

	file.WriteString("\nINCOME SUMMARY\n")
	file.WriteString("==============\n")
	file.WriteString(fmt.Sprintf("Total Income: $%.2f\n", summary.TotalIncome))
	file.WriteString(fmt.Sprintf("Total Expenses: $%.2f\n", summary.TotalExpenses))
	file.WriteString(fmt.Sprintf("Net: $%.2f\n", summary.NetAmount))

	return nil
}

// SummaryStats holds summary statistics
type SummaryStats struct {
	StartDate      string
	EndDate        string
	CategoryTotals map[string]float64
	TotalIncome    float64
	TotalExpenses  float64
	NetAmount      float64
}

// calculateSummary calculates summary statistics from transactions
func (a *Analyzer) calculateSummary(transactions []*models.Transaction) SummaryStats {
	summary := SummaryStats{
		CategoryTotals: make(map[string]float64),
	}

	if len(transactions) == 0 {
		return summary
	}

	// Find date range
	startDate := transactions[0].Date
	endDate := transactions[0].Date

	for _, tx := range transactions {
		if tx.Date.Before(startDate) {
			startDate = tx.Date
		}
		if tx.Date.After(endDate) {
			endDate = tx.Date
		}

		// Calculate category totals
		if tx.Category != "" {
			summary.CategoryTotals[tx.Category] += tx.Amount
		}

		// Calculate income vs expenses
		if tx.Type == models.Credit {
			summary.TotalIncome += tx.Amount
		} else {
			summary.TotalExpenses += tx.Amount
		}
	}

	summary.StartDate = startDate.Format("2006-01-02")
	summary.EndDate = endDate.Format("2006-01-02")
	summary.NetAmount = summary.TotalIncome - summary.TotalExpenses

	return summary
}
