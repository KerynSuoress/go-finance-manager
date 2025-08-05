package loader

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type PDFLoader struct {
	pdfFolderPath string
	PDFs          []string
}

func New(pdfFolderPath string) *PDFLoader {
	return &PDFLoader{
		pdfFolderPath: pdfFolderPath,
		PDFs:          make([]string, 0),
	}
}

func (l *PDFLoader) Load() error {
	if _, err := os.Stat(l.pdfFolderPath); os.IsNotExist(err) {
		return fmt.Errorf("PDF dir does not exist: %s", l.pdfFolderPath)
	}

	files, err := os.ReadDir(l.pdfFolderPath)
	if err != nil {
		return fmt.Errorf("failed to read PDF dir: %s", err)
	}

	for _, file := range files {
		if strings.ToLower(filepath.Ext(file.Name())) == ".pdf" {
			l.PDFs = append(l.PDFs, file.Name())
		}
	}
	if len(l.PDFs) == 0 {
		return fmt.Errorf("no PDFs found in dir: %s", l.pdfFolderPath)
	}
	return nil
}
