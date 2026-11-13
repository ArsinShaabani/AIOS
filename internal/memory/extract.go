package memory

import (
	"crypto/sha256"
	"encoding/hex"
	"regexp"
	"strings"

	"github.com/ArsinShaabani/AIOS/internal/domain"
)

// Extraction patterns. They are intentionally conservative: AIOS must never
// silently persist secrets, and noisy extraction erodes user trust. Both English
// and Persian (Farsi) phrasings are recognised.
type extractRule struct {
	re         *regexp.Regexp
	kind       domain.MemoryKind
	importance float64
	template   string // %s is the captured value
	tags       []string
}

var extractRules = []extractRule{
	{regexp.MustCompile(`(?i)\bmy name is ([A-Za-z][A-Za-z '\-]{1,60})`), domain.KindFact, 0.8, "User's name is %s", []string{"name"}},
	{regexp.MustCompile(`(?i)\bcall me ([A-Za-z][A-Za-z '\-]{1,60})`), domain.KindPreference, 0.75, "User prefers to be called %s", []string{"name", "preference"}},
	{regexp.MustCompile(`(?i)\bi live in ([^.!?\n]{2,80})`), domain.KindFact, 0.7, "User lives in %s", []string{"location"}},
	{regexp.MustCompile(`(?i)\bi(?:'m| am) from ([^.!?\n]{2,80})`), domain.KindFact, 0.65, "User is from %s", []string{"location"}},
	{regexp.MustCompile(`(?i)\bi (?:like|love|prefer|enjoy) ([^.!?\n]{2,120})`), domain.KindPreference, 0.65, "User likes %s", []string{"preference"}},
	{regexp.MustCompile(`(?i)\bmy favou?rite ([a-z ]{2,40}) is ([^.!?\n]{2,100})`), domain.KindPreference, 0.7, "User's favourite %s is %s", []string{"preference"}},
	{regexp.MustCompile(`(?i)\bmy ([a-z ]{2,40}) is ([^.!?\n]{2,120})`), domain.KindFact, 0.6, "User's %s is %s", []string{"profile"}},
	{regexp.MustCompile(`(?i)\bi work (?:as|at) ([^.!?\n]{2,100})`), domain.KindFact, 0.65, "User works as/at %s", []string{"work"}},
	{regexp.MustCompile(`(?i)\bremember (?:that )?(.+)`), domain.KindFact, 0.9, "%s", []string{"explicit"}},
	// Persian
	{regexp.MustCompile(`اسم من (.{2,60})(?: است| هست|ه)`), domain.KindFact, 0.8, "نام کاربر %s است", []string{"name"}},
	{regexp.MustCompile(`من (?:در|توی) (.{2,80}) (?:زندگی می‌کنم|زندگی میكنم|هستم)`), domain.KindFact, 0.7, "کاربر در %s زندگی می‌کند", []string{"location"}},
	{regexp.MustCompile(`من (.{2,100}) (?:را )?(?:دوست دارم|علاقه دارم)`), domain.KindPreference, 0.65, "کاربر %s را دوست دارد", []string{"preference"}},
	{regexp.MustCompile(`یادت باشه (.{2,200})`), domain.KindFact, 0.9, "%s", []string{"explicit"}},
	{regexp.MustCompile(`به یاد داشته باش (.{2,200})`), domain.KindFact, 0.9, "%s", []string{"explicit"}},
	{regexp.MustCompile(`سن من (.{1,3})`), domain.KindFact, 0.7, "سن کاربر %s سال است", []string{"profile"}},
}

var secretRe = regexp.MustCompile(`(?i)(password|passwd|secret|api[_ ]?key|token|private key|رمز|گذرواژه|کلید)`)
var questionRe = regexp.MustCompile(`[?؟]\s*$`)

// Extract turns a user utterance into candidate memories.
func Extract(text string) []domain.Memory {
	cleaned := strings.TrimSpace(text)
	if len(cleaned) < 4 || len(cleaned) > 2000 {
		return nil
	}
	if questionRe.MatchString(cleaned) && !strings.Contains(strings.ToLower(cleaned), "remember") {
		return nil
	}
	if secretRe.MatchString(cleaned) {
		// Never auto-store credentials; explicit secret management is separate.
		return nil
	}
	seen := map[string]struct{}{}
	out := make([]domain.Memory, 0, 2)
	for _, rule := range extractRules {
		matches := rule.re.FindAllStringSubmatch(cleaned, 2)
		for _, m := range matches {
			var content string
			switch {
			case strings.Count(rule.template, "%s") == 2 && len(m) >= 3:
				content = sprintf2(rule.template, strings.TrimSpace(m[1]), strings.TrimSpace(m[2]))
			case len(m) >= 2:
				content = sprintf1(rule.template, strings.TrimSpace(m[1]))
			default:
				continue
			}
			content = strings.TrimSpace(strings.TrimRight(content, ".,;!?"))
			if len([]rune(content)) < 6 {
				continue
			}
			content = truncateRunes(content, 300)
			key := strings.ToLower(content)
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			out = append(out, domain.Memory{
				Tier:       domain.TierLong,
				Kind:       rule.kind,
				Content:    content,
				Summary:    truncateRunes(content, 160),
				Tags:       rule.tags,
				Importance: rule.importance,
			})
		}
	}
	return out
}

// Fingerprint returns a stable hash for a memory's content (dedupe/logging).
func Fingerprint(content string) string {
	sum := sha256.Sum256([]byte(strings.ToLower(strings.TrimSpace(content))))
	return hex.EncodeToString(sum[:12])
}

func sprintf1(format, a string) string {
	if strings.Count(format, "%s") == 1 {
		return strings.Replace(format, "%s", a, 1)
	}
	return format
}

func sprintf2(format, a, b string) string {
	parts := strings.SplitN(format, "%s", 3)
	if len(parts) != 3 {
		return sprintf1(format, a)
	}
	return parts[0] + a + parts[1] + b + parts[2]
}

func truncateRunes(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n-1]) + "…"
}
