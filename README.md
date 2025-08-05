# Finance Manager

A financial analysis tool that extracts and categorizes transactions from encrypted PDF bank statements using Claude AI. This tool can process multiple PDF files, extract transaction data, categorize spending, and generate comprehensive financial reports.

## 🚀 Features

- **PDF Processing**: Extract text from bank statement PDFs (including encrypted ones)
- **AI-Powered Analysis**: Uses Claude AI to intelligently extract and categorize transactions
- **Multi-Format Support**: Handles various bank statement formats
- **Comprehensive Reporting**: Generates CSV reports and spending summaries
- **Batch Processing**: Process multiple PDF files at once
- **Password Protection**: Supports encrypted PDFs with password decryption

## 📋 Prerequisites

- **Go 1.24+** - [Download here](https://golang.org/dl/)
- **Python 3.8+** - [Download here](https://www.python.org/downloads/)
- **Claude API Key** - [Get one here](https://console.anthropic.com/)

## 🛠️ Installation

### 1. Clone the repository
```bash
git clone <your-repo-url>
cd finance-manager
```

### 2. Install Python dependencies
```bash
pip install -r requirements.txt
```

### 3. Install Go dependencies
```bash
go mod download
```

### 4. Set up environment variables
```bash
# Copy the example environment file
cp .env.example .env

# Edit the .env file with your actual values
# You'll need to add your Claude API key and PDF passwords
```

## ⚙️ Configuration

Edit your `.env` file with the following variables:

### Required
- `CLAUDE_API_KEY`: Your Claude AI API key from Anthropic

### Optional (for encrypted PDFs)
- `PASS_CC`: Credit card password
- `PASS_BIRTH`: Birth date password
- `PASS_BIRTH2`: Alternative birth date password
- `PASS_SURNAME`: Surname password

### Example `.env` file:
```env
CLAUDE_API_KEY=sk-ant-api03-your-actual-key-here
PASS_CC=your_id_here
PASS_BIRTH=your_birth_date_password_here
PASS_BIRTH2=your_alternative_birth_date_password_here
PASS_SURNAME=your_surname_password_here
```

## 📁 Project Structure

```
finance-manager/
├── cmd/manager/          # Main Go application
├── internal/             # Go packages
│   ├── analyzer/         # AI analysis logic
│   ├── extractor/        # PDF text extraction
│   ├── loader/           # PDF file loading
│   └── models/           # Data models
├── scripts/              # Python utilities
├── toProcess/            # Place PDF files here
├── output/               # Generated reports
└── .env.example          # Environment template
```

## 🚀 Usage

### 1. Prepare your PDF files
Place your bank statement PDFs in the `toProcess/` directory:
```bash
cp your_bank_statement.pdf toProcess/
```

### 2. Run the analysis
```bash
# Basic usage (outputs to 'output' folder)
go run cmd/manager/main.go

# Specify custom output directory
go run cmd/manager/main.go -o reports

# Help
go run cmd/manager/main.go -h
```

### 3. Check your results
The tool will generate:
- **CSV Report**: `output/transactions_YYYYMMDD.csv` - Detailed transaction data
- **Summary Report**: `output/summary_YYYYMMDD.txt` - Spending analysis

## 📊 Output Examples

### CSV Report Format
```csv
Date,Description,Amount,Category,Subcategory,Source
2024-01-15,GROCERY STORE,-$45.67,Food & Dining,Groceries,statement.pdf
2024-01-16,GAS STATION,-$35.00,Transportation,Fuel,statement.pdf
2024-01-17,SALARY DEPOSIT,$2500.00,Income,Salary,statement.pdf
```

### Summary Report
- Total income and expenses
- Category breakdown
- Spending trends
- Net financial position

## 🔧 Advanced Usage

### Processing specific files
```bash
# The tool automatically processes all PDFs in toProcess/
# Just add your files and run the command
```

### Custom output location
```bash
go run cmd/manager/main.go -o /path/to/custom/output
```

### Using the Python script directly
```bash
# Extract text from a single PDF
python scripts/extract_text.py input.pdf output.txt
```

## 🐛 Troubleshooting

### Common Issues

1. **"CLAUDE_API_KEY environment variable is required"**
   - Make sure you've created a `.env` file with your API key
   - Verify the API key is valid and has sufficient credits

2. **"Failed to load PDFs"**
   - Check that PDF files are in the `toProcess/` directory
   - Ensure PDFs are not corrupted

3. **"Failed to extract text"**
   - For encrypted PDFs, add the correct password to your `.env` file
   - Try the Python script directly to test PDF extraction

4. **"No transactions found"**
   - Check that the PDF contains readable text (not just images)
   - Verify the PDF format is supported

### Debug Mode
```bash
# Run with verbose output
go run cmd/manager/main.go -v
```

## 🤝 Contributing

1. Fork the repository
2. Create a feature branch
3. Make your changes
4. Add tests if applicable
5. Submit a pull request

## 📝 License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.

The MIT License is a permissive license that allows for:
- Commercial use
- Modification
- Distribution
- Private use

The only requirement is that the license and copyright notice be included in all copies or substantial portions of the software.

## 🙏 Acknowledgments

- [Claude AI](https://claude.ai/) for intelligent transaction analysis
- [PyPDF2](https://pypdf2.readthedocs.io/) for PDF text extraction
- [Go](https://golang.org/) for the main application framework

---

**Note**: This tool processes financial data. Always review the generated reports for accuracy and keep your API keys secure.