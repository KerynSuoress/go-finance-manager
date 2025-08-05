package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/KerynSuoress/finance-manager/internal/analyzer"
	"github.com/KerynSuoress/finance-manager/internal/extractor"
	"github.com/KerynSuoress/finance-manager/internal/loader"
	"github.com/KerynSuoress/finance-manager/internal/models"
)

// readTextFile reads the content of a text file
func readTextFile(filePath string) (string, error) {
	content, err := os.ReadFile(filePath)
	if err != nil {
		return "", fmt.Errorf("failed to read text file %s: %v", filePath, err)
	}
	return string(content), nil
}

func main() {
	// Command line flags
	var (
		outputFolder = flag.String("o", "output", "Path to output folder for reports (default: output)")
	)
	flag.Parse()

	// Step 1: Load all PDFs from toProcess folder
	fmt.Println("📁 Loading PDFs from toProcess folder...")
	pdfLoader := loader.New("toProcess")
	if err := pdfLoader.Load(); err != nil {
		log.Fatalf("Failed to load PDFs: %v", err)
	}
	fmt.Printf("✓ Found %d PDF files to process\n", len(pdfLoader.PDFs))

	// Step 2: Initialize text extractor
	fmt.Println("🔧 Initializing PDF text extractor...")
	textExtractor := extractor.New()

	// Step 3: Initialize AI analyzer
	fmt.Println("🤖 Initializing Claude AI analyzer...")
	aiAnalyzer, err := analyzer.NewAnalyzer()
	if err != nil {
		log.Fatalf("Failed to create AI analyzer: %v\nPlease check your CLAUDE_API_KEY environment variable", err)
	}

	// Step 4: Process each PDF and collect all transactions
	fmt.Println("📊 Processing PDFs and extracting transactions...")
	var allTransactions []*models.Transaction

	for i, pdf := range pdfLoader.PDFs {
		fmt.Printf("Processing file %d/%d: %s\n", i+1, len(pdfLoader.PDFs), pdf)

		// Generate output filename using the same logic as before
		var textOutputPath string
		fullPath := filepath.Join("toProcess", pdf)
		if *outputFolder == "" {
			base := filepath.Base(fullPath)
			name := base[:len(base)-len(filepath.Ext(base))]
			textOutputPath = filepath.Join("output", name+"_extracted.txt")
		} else {
			base := filepath.Base(fullPath)
			name := base[:len(base)-len(filepath.Ext(base))]
			textOutputPath = filepath.Join(*outputFolder, name+"_extracted.txt")
		}

		// Extract text from PDF
		if err := textExtractor.ExtractToFile(fullPath, textOutputPath); err != nil {
			fmt.Printf("⚠️  Warning: Failed to extract text from %s: %v\n", pdf, err)
			continue
		}

		// Read the extracted text
		text, err := readTextFile(textOutputPath)
		if err != nil {
			fmt.Printf("⚠️  Warning: Failed to read extracted text from %s: %v\n", pdf, err)
			continue
		}

		// Use AI to extract transactions from the text
		transactions, err := aiAnalyzer.ExtractTransactionsFromText(text, pdf)
		if err != nil {
			fmt.Printf("⚠️  Warning: Failed to extract transactions from %s: %v\n", pdf, err)
			continue
		}

		fmt.Printf("✓ Extracted %d transactions from %s\n", len(transactions), pdf)
		allTransactions = append(allTransactions, transactions...)
	}

	// Step 5: Report total transactions found
	fmt.Printf("\n🎯 Total transactions extracted: %d\n", len(allTransactions))

	if len(allTransactions) == 0 {
		fmt.Println("❌ No transactions found. Check your PDF files and try again.")
		return
	}

	// Step 6: Categorize all transactions using AI
	fmt.Println("🏷️  Categorizing transactions using Claude AI...")
	if err := aiAnalyzer.CategorizeTransactions(allTransactions); err != nil {
		fmt.Printf("⚠️  Warning: Categorization failed: %v\n", err)
		fmt.Println("Continuing with uncategorized transactions...")
	} else {
		fmt.Println("✓ Transaction categorization complete")
	}

	// Step 7: Generate consolidated reports (always in the specified output folder)
	reportFolder := *outputFolder
	if reportFolder == "" {
		reportFolder = "reports" // Default for reports if no output specified
	}

	fmt.Printf("📈 Generating consolidated analysis reports in %s...\n", reportFolder)
	if err := aiAnalyzer.GenerateReports(allTransactions, reportFolder); err != nil {
		log.Fatalf("Failed to generate reports: %v", err)
	}

	// Step 8: Success summary
	fmt.Println("\n🎉 Finance analysis complete!")
	fmt.Printf("📊 Processed %d PDF files\n", len(pdfLoader.PDFs))
	fmt.Printf("💰 Analyzed %d transactions\n", len(allTransactions))
	fmt.Printf("📁 Check the %s folder for your analysis results\n", reportFolder)
	fmt.Println("   - transactions_YYYYMMDD.csv (detailed transaction data)")
	fmt.Println("   - summary_YYYYMMDD.txt (spending analysis summary)")
}
