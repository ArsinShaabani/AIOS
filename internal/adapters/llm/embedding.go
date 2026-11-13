package llm

import (
	"hash/fnv"
	"math"
	"strings"
	"unicode"

	"github.com/ArsinShaabani/AIOS/internal/ports"
)

// HashEmbedding tokenises text and projects tokens/bigrams/trigrams into a
// fixed-dimension signed vector. It is deterministic, allocation-light and needs
// no GPU, giving useful lexical recall on a Raspberry Pi.
func HashEmbedding(text string, dim int) []float32 {
	if dim <= 0 {
		dim = 384
	}
	vec := make([]float32, dim)
	tokens := tokenize(text)
	if len(tokens) == 0 {
		return vec
	}
	add := func(tok string, weight float32) {
		h := fnv.New64a()
		_, _ = h.Write([]byte(tok))
		sum := h.Sum64()
		idx := int(sum % uint64(dim))
		if sum&(1<<63) != 0 {
			vec[idx] -= weight
		} else {
			vec[idx] += weight
		}
	}
	for i, t := range tokens {
		add(t, 1)
		if i > 0 {
			add(tokens[i-1]+" "+t, 0.6)
		}
		if len(t) >= 4 {
			for j := 0; j+3 <= len(t); j++ {
				add(t[j:j+3], 0.25)
			}
		}
	}
	norm := float64(0)
	for _, v := range vec {
		norm += float64(v) * float64(v)
	}
	norm = math.Sqrt(norm)
	if norm > 0 {
		for i := range vec {
			vec[i] = float32(float64(vec[i]) / norm)
		}
	}
	return vec
}

func tokenize(text string) []string {
	fields := strings.FieldsFunc(strings.ToLower(text), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})
	out := make([]string, 0, len(fields))
	for _, f := range fields {
		runes := []rune(f)
		if len(runes) > 1 || (len(runes) == 1 && unicode.IsDigit(runes[0])) {
			out = append(out, f)
		}
	}
	return out
}

func estimateTokens(messages []ports.ChatMessage) int {
	n := 0
	for _, m := range messages {
		n += estimateTokensText(m.Content)
	}
	return n
}

func estimateTokensText(text string) int {
	// ~4 characters per token is the standard heuristic; denser scripts
	// (Farsi, CJK) still get a reasonable estimate.
	runes := len([]rune(text))
	return max(runes/4, 1)
}

func firstOr(s, fallback string) string {
	if s == "" {
		return fallback
	}
	return s
}
