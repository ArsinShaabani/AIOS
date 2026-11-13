package documents

import "strings"

// ChunkText splits text into overlapping chunks, preferring paragraph and
// sentence boundaries so retrieved snippets stay readable. maxChunks bounds the
// cost of a hostile or huge upload.
func ChunkText(text string, size, overlap, maxChunks int) []string {
	if size < 50 {
		size = 900
	}
	if overlap < 0 {
		overlap = 0
	}
	if overlap >= size {
		overlap = size / 4
	}
	if maxChunks <= 0 {
		maxChunks = 500
	}
	clean := normalize(text)
	runes := []rune(clean)
	if len(runes) == 0 {
		return nil
	}
	if len(runes) <= size {
		return []string{strings.TrimSpace(clean)}
	}
	out := make([]string, 0, min(len(runes)/size+1, maxChunks))
	start := 0
	for start < len(runes) && len(out) < maxChunks {
		end := min(start+size, len(runes))
		if end < len(runes) {
			window := string(runes[start:end])
			cut := bestCut(window)
			if cut < len(window)/2 {
				cut = len(window)
			}
			end = start + len([]rune(window[:cut]))
		}
		chunk := strings.TrimSpace(string(runes[start:end]))
		if chunk != "" {
			out = append(out, chunk)
		}
		if end >= len(runes) {
			break
		}
		next := end - overlap
		if next <= start {
			next = end
		}
		start = next
	}
	return out
}

// bestCut finds a natural boundary near the end of a window.
func bestCut(window string) int {
	minCut := len(window) * 6 / 10
	for _, sep := range []string{"\n\n", "\n", ". ", "! ", "? ", "؟ ", "۔ ", "، ", " "} {
		if idx := strings.LastIndex(window, sep); idx > minCut {
			return idx + len(sep)
		}
	}
	return len(window)
}

func normalize(text string) string {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")
	text = strings.ReplaceAll(text, "\x00", " ")
	lines := strings.Split(text, "\n")
	var b strings.Builder
	blank := 0
	for _, line := range lines {
		line = strings.Join(strings.Fields(line), " ")
		if line == "" {
			blank++
			if blank > 1 {
				continue
			}
			b.WriteString("\n")
			continue
		}
		blank = 0
		if b.Len() > 0 && !strings.HasSuffix(b.String(), "\n") {
			b.WriteString("\n")
		}
		b.WriteString(line)
	}
	return strings.TrimSpace(b.String())
}
