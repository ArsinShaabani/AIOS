package documents

import (
	"bytes"
	"io"
	"os"

	"github.com/ledongthuc/pdf"
)

// extractPDF reads text from a PDF using a pure-Go parser. Scanned/image-only
// PDFs need OCR and therefore produce no text (rejected by Ingest).
func extractPDF(raw []byte) string {
	tmp, err := os.CreateTemp("", "aios-doc-*.pdf")
	if err != nil {
		return ""
	}
	name := tmp.Name()
	defer os.Remove(name)
	if _, err := tmp.Write(raw); err != nil {
		_ = tmp.Close()
		return ""
	}
	if err := tmp.Close(); err != nil {
		return ""
	}
	var text string
	func() {
		defer func() { _ = recover() }() // malformed PDFs must not crash the process
		file, reader, err := pdf.Open(name)
		if err != nil {
			return
		}
		defer file.Close()
		plain, err := reader.GetPlainText()
		if err != nil {
			return
		}
		var buf bytes.Buffer
		_, _ = io.Copy(&buf, io.LimitReader(plain, 8<<20))
		text = buf.String()
	}()
	return text
}
