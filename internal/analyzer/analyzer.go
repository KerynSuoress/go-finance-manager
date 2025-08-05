package analyzer

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
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

	// Use Claude Sonnet 4 as default model
	model := "claude-sonnet-4-20250514" // Latest model

	maxTokens := 4096 // Default max tokens

	return &Analyzer{
		apiKey:     apiKey,
		model:      model,
		maxTokens:  maxTokens,
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}, nil
}

// CategorizeTransactions uses Claude API to categorize all transactions
func (a *Analyzer) CategorizeTransactions(transactions []*models.Transaction) error {
	if len(transactions) == 0 {
		fmt.Println("No transactions to categorize")
		return nil
	}

	fmt.Printf("Categorizing %d transactions using Claude API...\n", len(transactions))

	// Process transactions in batches to avoid rate limits
	batchSize := 100
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

	// Build prompt for transaction extraction
	prompt := a.buildExtractionPrompt(text, source)

	// Call Claude API
	request := ClaudeAPIRequest{
		Model:       a.model,
		MaxTokens:   a.maxTokens,
		Temperature: 0.1, // Low temperature for consistent extraction
		Messages: []Message{
			{
				Role:    "user",
				Content: prompt,
			},
		},
	}

	response, err := a.callClaudeAPI(request)
	if err != nil {
		return nil, fmt.Errorf("failed to call Claude API: %v", err)
	}

	// Parse the response to extract transactions
	transactions, err := a.parseExtractionResponse(response, source)
	if err != nil {
		return nil, fmt.Errorf("failed to parse extraction response: %v", err)
	}

	fmt.Printf("✓ Extracted %d transactions from PDF text\n", len(transactions))
	return transactions, nil
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

	sb.WriteString("Extract all transactions and respond in JSON format like this:\n")
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

	return sb.String()
}

// Add this function to extract JSON from Claude's response
func extractJSONFromResponse(content string) (string, error) {
	content = strings.TrimSpace(content)

	// Handle Haiku format: "Based on the statement, here are the extracted transactions:\n["
	// Look for the first [ to start JSON
	jsonStart := strings.Index(content, "[")
	if jsonStart != -1 {
		// Find the last ] to end JSON
		jsonEnd := strings.LastIndex(content, "]")
		if jsonEnd != -1 && jsonEnd > jsonStart {
			return content[jsonStart : jsonEnd+1], nil
		}
	}

	// Fallback: Handle Sonnet format with ```json blocks
	start := strings.Index(content, "```json")
	if start == -1 {
		start = strings.Index(content, "```")
		if start == -1 {
			return "", fmt.Errorf("no JSON block found in response")
		}
	}

	// Find the start of actual JSON (after the ```json)
	jsonStart = strings.Index(content[start:], "[")
	if jsonStart == -1 {
		return "", fmt.Errorf("no JSON array found in response")
	}
	jsonStart += start

	// Find the end of JSON block
	end := strings.Index(content[jsonStart:], "```")
	if end == -1 {
		end = strings.LastIndex(content, "]")
		if end == -1 {
			return "", fmt.Errorf("no closing bracket found in JSON")
		}
		return content[jsonStart : end+1], nil
	}

	return content[jsonStart : jsonStart+end], nil
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
		return nil, fmt.Errorf("failed to extract JSON from response: %v", err)
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
		Model:       "claude-3-5-haiku-latest",
		MaxTokens:   a.maxTokens,
		Temperature: 0.3, // Low temperature for consistent categorization
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

	sb.WriteString("You are a financial transaction categorizer. Analyze the following transactions and categorize each one with a main category and subcategory. If you are not completely sure about the category, use the 'Other' category, otherwise, Use standard financial categories like:\n\n")
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

	sb.WriteString("Respond in JSON format like this:\n")
	sb.WriteString("[\n")
	sb.WriteString("  {\n")
	sb.WriteString("    \"index\": 0,\n")
	sb.WriteString("    \"category\": \"Food & Dining\",\n")
	sb.WriteString("    \"subcategory\": \"Restaurants\",\n")
	sb.WriteString("    \"confidence\": 0.95\n")
	sb.WriteString("  }\n")
	sb.WriteString("]\n\n")

	sb.WriteString("Here are the transactions to categorize:\n\n")

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

	// Create HTTP request
	req, err := http.NewRequest("POST", "https://api.anthropic.com/v1/messages", bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %v", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", a.apiKey)
	req.Header.Set("anthropic-version", "2023-06-01")

	// Make the request
	resp, err := a.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to make API request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API request failed with status %d", resp.StatusCode)
	}

	// Parse response
	var response ClaudeAPIResponse
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return nil, fmt.Errorf("failed to decode response: %v", err)
	}

	return &response, nil
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
