package llm

func firstOr(s, fallback string) string {
	if s == "" {
		return fallback
	}
	return s
}

// estimateTokensText roughly estimates tokens for cost accounting when a
// provider does not report usage (local embeddings).
func estimateTokensText(text string) int {
	runes := len([]rune(text))
	return max(runes/4, 1)
}
