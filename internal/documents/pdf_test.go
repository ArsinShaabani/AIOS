package documents

import (
	"strings"
	"testing"

	"github.com/go-pdf/fpdf"
)

func makePDF(t *testing.T, lines ...string) []byte {
	t.Helper()
	doc := fpdf.New("P", "mm", "A4", "")
	doc.AddPage()
	doc.SetFont("Helvetica", "", 12)
	for _, line := range lines {
		doc.MultiCell(0, 6, line, "", "L", false)
	}
	var buf strings.Builder
	if err := doc.Output(&stringWriter{&buf}); err != nil {
		t.Fatal(err)
	}
	return []byte(buf.String())
}

type stringWriter struct{ b *strings.Builder }

func (w *stringWriter) Write(p []byte) (int, error) { return w.b.Write(p) }

func TestExtractPDF(t *testing.T) {
	raw := makePDF(t, "Refund policy", "Refunds take five business days.")
	text := ExtractText(string(raw), "application/pdf")
	if !strings.Contains(text, "Refund") || !strings.Contains(text, "five business") {
		t.Fatalf("pdf text: %q", text)
	}
	// Malformed PDFs must yield no text rather than panicking.
	if got := ExtractText("not a pdf at all", "application/pdf"); strings.TrimSpace(got) != "" {
		t.Fatalf("garbage pdf produced text: %q", got)
	}
}
