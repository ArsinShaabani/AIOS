package documents

import (
	"strings"

	"golang.org/x/net/html"
)

// ExtractText turns an uploaded document into plain text. HTML is stripped with
// a tokenizer; all other supported types are treated as UTF-8 text.
func ExtractText(content, mime string) string {
	trimmed := strings.TrimSpace(content)
	if strings.Contains(mime, "pdf") {
		return extractPDF([]byte(content))
	}
	if strings.Contains(mime, "html") || strings.Contains(mime, "xhtml") ||
		strings.HasPrefix(trimmed, "<!doctype") || strings.HasPrefix(trimmed, "<html") {
		return stripHTML(content)
	}
	// JSON/CSV/Markdown/plain text are kept verbatim (normalised on chunking).
	return strings.ReplaceAll(content, "\x00", " ")
}

func stripHTML(raw string) string {
	z := html.NewTokenizer(strings.NewReader(raw))
	var b strings.Builder
	skip := 0
	for {
		tt := z.Next()
		if tt == html.ErrorToken {
			break
		}
		switch tt {
		case html.TextToken:
			if skip == 0 {
				txt := strings.TrimSpace(string(z.Text()))
				if txt != "" {
					if b.Len() > 0 {
						b.WriteString(" ")
					}
					b.WriteString(txt)
				}
			}
		case html.StartTagToken:
			name, _ := z.TagName()
			switch string(name) {
			case "script", "style", "noscript", "svg", "template", "iframe", "head":
				skip++
			case "p", "div", "br", "li", "tr", "h1", "h2", "h3", "h4", "section", "article":
				b.WriteString("\n")
			}
		case html.EndTagToken:
			name, _ := z.TagName()
			switch string(name) {
			case "script", "style", "noscript", "svg", "template", "iframe", "head":
				if skip > 0 {
					skip--
				}
			case "p", "div", "li", "tr", "h1", "h2", "h3", "h4", "section", "article":
				b.WriteString("\n")
			}
		}
	}
	return strings.TrimSpace(b.String())
}
