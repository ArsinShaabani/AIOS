package tools

import (
	"context"
	"strings"

	"golang.org/x/net/html"

	"github.com/ArsinShaabani/AIOS/internal/platform"
)

// Page is extracted readable content.
type Page struct {
	URL     string `json:"url"`
	Title   string `json:"title"`
	Content string `json:"content"`
	Excerpt string `json:"excerpt"`
}

// Fetch retrieves and extracts readable text from a page.
func (w *WebClient) Fetch(ctx context.Context, rawURL string) (Page, error) {
	body, contentType, err := w.Get(ctx, rawURL, 0)
	if err != nil {
		return Page{}, err
	}
	page := Page{URL: rawURL}
	if strings.Contains(contentType, "json") {
		page.Title = rawURL
		page.Content = platform.Truncate(string(body), 12000)
		page.Excerpt = platform.Truncate(string(body), 300)
		return page, nil
	}
	title, text := ExtractText(body)
	page.Title = title
	if page.Title == "" {
		page.Title = rawURL
	}
	page.Content = platform.Truncate(text, 12000)
	page.Excerpt = platform.Truncate(text, 300)
	return page, nil
}

// ExtractText strips markup and returns the title plus readable text.
func ExtractText(raw []byte) (string, string) {
	doc, err := html.Parse(strings.NewReader(string(raw)))
	if err != nil {
		return "", platform.Truncate(string(raw), 4000)
	}
	var title string
	var b strings.Builder
	skip := 0
	hidden := map[string]bool{"script": true, "style": true, "noscript": true, "svg": true, "template": true, "iframe": true}
	blocks := map[string]bool{"p": true, "div": true, "br": true, "li": true, "tr": true, "h1": true,
		"h2": true, "h3": true, "h4": true, "section": true, "article": true}
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		isTitle := false
		if n.Type == html.ElementNode {
			if hidden[n.Data] {
				skip++
			}
			if n.Data == "title" {
				title = strings.TrimSpace(nodeText(n))
				isTitle = true
				skip++
			}
		}
		if n.Type == html.TextNode && skip == 0 {
			txt := strings.TrimSpace(n.Data)
			if txt != "" {
				if b.Len() > 0 {
					b.WriteString(" ")
				}
				b.WriteString(txt)
			}
		}
		for child := n.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
		if n.Type == html.ElementNode {
			if hidden[n.Data] {
				skip--
			}
			if isTitle {
				skip--
			}
			if blocks[n.Data] {
				b.WriteString("\n")
			}
		}
	}
	walk(doc)
	return title, collapseWhitespace(b.String())
}

func collapseWhitespace(s string) string {
	lines := strings.Split(s, "\n")
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		cleaned := strings.Join(fields, " ")
		for _, punct := range []string{".", ",", ";", ":", "!", "?", "%", ")"} {
			cleaned = strings.ReplaceAll(cleaned, " "+punct, punct)
		}
		cleaned = strings.ReplaceAll(cleaned, "( ", "(")
		out = append(out, cleaned)
	}
	return strings.Join(out, "\n")
}
