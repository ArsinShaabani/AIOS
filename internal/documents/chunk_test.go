package documents

import (
	"strings"
	"testing"
)

func TestChunkTextBoundsAndOverlap(t *testing.T) {
	text := strings.Repeat("Paragraph one about refunds and billing. ", 20) + "\n\n" +
		strings.Repeat("Second section about shipping and delivery. ", 20)
	chunks := ChunkText(text, 400, 80, 50)
	if len(chunks) < 2 {
		t.Fatalf("expected multiple chunks, got %d", len(chunks))
	}
	for i, c := range chunks {
		if len([]rune(c)) > 400 {
			t.Fatalf("chunk %d too long: %d", i, len([]rune(c)))
		}
		if strings.TrimSpace(c) == "" {
			t.Fatalf("chunk %d empty", i)
		}
	}
	// A long token without spaces is still split.
	long := strings.Repeat("x", 2000)
	chunks = ChunkText(long, 300, 0, 3)
	if len(chunks) != 3 {
		t.Fatalf("maxChunks not enforced: %d", len(chunks))
	}
	if len(ChunkText("   ", 300, 0, 3)) != 0 {
		t.Fatal("blank text produced chunks")
	}
	if got := ChunkText("short", 100, 0, 3); len(got) != 1 || got[0] != "short" {
		t.Fatalf("short text: %v", got)
	}
	// Overlap must make progress even when larger than the remaining text.
	over := ChunkText(strings.Repeat("word ", 500), 120, 119, 10)
	if len(over) < 2 {
		t.Fatalf("overlap chunks: %d", len(over))
	}
}

func TestExtractTextFormats(t *testing.T) {
	htmlDoc := `<html><head><title>Docs</title><style>x{}</style></head><body>
		<script>evil()</script><h1>Refunds</h1><p>Refunds take five days.</p></body></html>`
	text := ExtractText(htmlDoc, "text/html")
	if strings.Contains(text, "evil") || strings.Contains(text, "x{}") {
		t.Fatalf("scripts/styles leaked: %q", text)
	}
	if !strings.Contains(text, "Refunds") || !strings.Contains(text, "five days") {
		t.Fatalf("html text missing: %q", text)
	}
	plain := ExtractText("line one\nline two", "text/plain")
	if plain != "line one\nline two" {
		t.Fatalf("plain text changed: %q", plain)
	}
	normalized := ChunkText("a\r\n\r\n\r\n\r\nb", 100, 0, 2)
	if len(normalized) != 1 || strings.Contains(normalized[0], "\r") {
		t.Fatalf("normalize: %v", normalized)
	}
}
