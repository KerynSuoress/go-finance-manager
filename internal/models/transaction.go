// Package models contains the core data structures for the finance analyzer application.
// This package defines the fundamental types used throughout the application,
// including transactions, personal information, and configuration settings.
//
// Key Design Principles:
// - Clear separation of concerns with focused data structures
// - Type safety through Go's strong typing system
// - Immutable design where possible for thread safety
// - Comprehensive documentation for learning and maintenance
package models

import (
	"time"
)

// Transaction represents a single financial transaction extracted from a bank statement.
// This is the core data structure that flows through the entire application pipeline.
//
// Architecture Role:
// - Input: Raw transaction data from PDF parsing
// - Processing: Categorized and analyzed by AI
// - Output: Used for report generation and insights
//
// Design Patterns Used:
// - Value Object: Immutable data structure
// - Builder Pattern: Can be constructed step by step
// - Observer Pattern: Can be processed by multiple analyzers
type Transaction struct {
	// Date represents when the transaction occurred.
	// Uses Go's time.Time for robust date handling and timezone support.
	// Format: ISO 8601 (2006-01-02T15:04:05Z07:00)
	Date time.Time

	// Description contains the merchant name or transaction description.
	// This is the raw text extracted from the bank statement.
	// Examples: "SUPERMERCADO CENTRAL", "GASOLINA SHELL"
	Description string

	// Amount represents the transaction value in the local currency.
	// Stored as float64 for precision in financial calculations.
	// Positive values typically represent debits (money spent).
	// Negative values typically represent credits (money received).
	Amount float64

	// Type indicates whether this is a debit (money spent) or credit (money received).
	// Uses a custom enum for type safety and clear intent.
	// This helps distinguish between purchases and payments/refunds.
	Type TransactionType

	// Balance represents the account balance after this transaction.
	// Optional field that may not be available in all bank statements.
	// Useful for reconciliation and balance verification.
	Balance float64

	// Category is the AI-generated spending category.
	// Examples: "Food & Dining", "Transportation", "Shopping"
	// This field is populated by the Claude API during categorization.
	Category string

	// Subcategory provides more specific categorization within the main category.
	// Examples: "Restaurants", "Gas", "Electronics"
	// Helps with detailed spending analysis and budgeting.
	Subcategory string

	// Confidence represents the AI's confidence level in the categorization.
	// Range: 0.0 (no confidence) to 1.0 (complete confidence)
	// Used to filter out low-confidence categorizations or flag for review.
	Confidence float64

	// RawText contains the original text line from the bank statement.
	// Useful for debugging, validation, and audit trails.
	// Preserves the exact format from the source document.
	RawText string

	// Source identifies which bank statement file this transaction came from.
	// Format: filename (e.g., "Extracto_875208547_202507_TARJETA_MASTERCARD_7002.pdf")
	// Helps with data lineage and troubleshooting.
	Source string
}

// TransactionType represents the nature of a financial transaction.
// This enum provides type safety and clear intent over using strings.
//
// Go Enum Pattern:
// - Uses const with iota for automatic numbering
// - Provides clear, self-documenting values
// - Prevents invalid transaction types
type TransactionType int

// Enum values for TransactionType
const (
	// Debit represents money spent (purchases, withdrawals, fees)
	// This is the most common type in credit card statements.
	// Amount is typically positive for debits.
	Debit TransactionType = iota

	// Credit represents money received (payments, refunds, deposits)
	// Common in bank account statements and credit card payments.
	// Amount is typically negative for credits.
	Credit
)

// String method provides human-readable representation of TransactionType.
// This implements the Stringer interface for better debugging and logging.
//
// Go Interface Pattern:
// - Implements fmt.Stringer interface
// - Enables automatic string conversion in fmt.Printf, log statements
// - Makes debugging easier with readable output
func (t TransactionType) String() string {
	switch t {
	case Debit:
		return "Debit"
	case Credit:
		return "Credit"
	default:
		return "Unknown"
	}
}
