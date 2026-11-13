package e2e

import (
	"strings"
	"testing"

	"github.com/go-pdf/fpdf"
)

// documentsTestPDF builds a tiny text PDF for the ingest test.
func documentsTestPDF(t *testing.T) []byte {
	t.Helper()
	doc := fpdf.New("P", "mm", "A4", "")
	doc.AddPage()
	doc.SetFont("Helvetica", "", 12)
	doc.MultiCell(0, 6, "Warranty terms: coverage lasts two years.", "", "L", false)
	var buf strings.Builder
	if err := doc.Output(&builderWriter{&buf}); err != nil {
		t.Fatal(err)
	}
	return []byte(buf.String())
}

type builderWriter struct{ b *strings.Builder }

func (w *builderWriter) Write(p []byte) (int, error) { return w.b.Write(p) }
