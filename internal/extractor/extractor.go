package extractor

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

// PDFExtractor handles PDF text extraction using Python
type PDFExtractor struct {
	pythonScript string
}

// New creates a new PDFExtractor instance
func New() *PDFExtractor {
	return &PDFExtractor{
		pythonScript: "scripts/extract_text.py",
	}
}

// ExtractToFile extracts text from PDF and saves to a text file
func (e *PDFExtractor) ExtractToFile(pdfPath, outputPath string) error {
	// Validate input file exists
	if _, err := os.Stat(pdfPath); os.IsNotExist(err) {
		return fmt.Errorf("PDF file does not exist: %s", pdfPath)
	}

	// Ensure output directory exists
	outputDir := filepath.Dir(outputPath)
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return fmt.Errorf("failed to create output directory: %w", err)
	}

	// Execute Python script
	cmd := exec.Command("python", e.pythonScript, pdfPath, outputPath)

	// Capture both stdout and stderr
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("extraction failed: %w\nOutput: %s", err, string(output))
	}

	// Verify output file was created
	if _, err := os.Stat(outputPath); os.IsNotExist(err) {
		return fmt.Errorf("output file was not created: %s", outputPath)
	}

	fmt.Printf("Extraction successful: %s\n", string(output))
	return nil
}
