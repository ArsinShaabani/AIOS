// Package brain is AIOS' decision core: it classifies intent, assembles context
// (personality + memory + history), chooses an agent, runs the model/tool loop
// and persists the conversation.
package brain

import (
	"strings"
	"unicode"
)

// Intent is the lightweight, rule-based classification used for routing. It is
// deliberately local (no model call) so a weak server keeps latency low.
type Intent struct {
	Name       string            `json:"name"`
	Confidence float64           `json:"confidence"`
	Entities   map[string]string `json:"entities,omitempty"`
}

type intentRule struct {
	name     string
	keywords []string
	weight   float64
}

var intentRules = []intentRule{
	{"greeting", []string{"hello", "hi ", "hey", "good morning", "good evening", "سلام", "درود", "صبح بخیر"}, 1},
	{"farewell", []string{"bye", "goodbye", "see you", "خداحافظ", "فعلا"}, 1},
	{"help", []string{"what can you do", "help me", "capabilities", "commands", "چیکار میتونی", "کمک"}, 1},
	{"device_control", []string{"turn on", "turn off", "switch on", "switch off", "set the", "brightness", "thermostat", "robot", "move forward", "stop moving",
		"روشن کن", "خاموش کن", "چراغ", "ربات", "حرکت کن", "بس کن"}, 1.4},
	{"home", []string{"home assistant", "scene", "living room", "bedroom", "kitchen", "صحنه", "اتاق", "آشپزخانه"}, 1.2},
	{"search", []string{"search", "google", "look up", "latest", "news", "price of",
		"جستجو", "سرچ", "خبر", "قیمت"}, 1.1},
	{"schedule", []string{"remind", "reminder", "timer", "alarm", "schedule", "tomorrow", "in an hour",
		"یادآوری", "تایمر", "زنگ", "فردا", "قرار"}, 1.3},
	{"memory", []string{"remember", "my name is", "i like", "i live", "یادت باشه", "اسم من", "دوست دارم"}, 1.2},
	{"email", []string{"email", "send a mail", "send mail", "ایمیل", "نامه"}, 1.2},
	{"navigation", []string{"route", "navigate", "directions", "how far", "مسیر", "ناوبری", "چقدر راه"}, 1.2},
	{"automation", []string{"automate", "every day", "every morning", "when i", "اتوماسیون", "هر روز", "هر صبح"}, 1.1},
	{"research", []string{"research", "compare", "analysis", "investigate", "pros and cons", "تحقیق", "مقایسه", "تحلیل"}, 1.0},
	{"code", []string{"code", "function", "bug", "python", "javascript", "sql", "کد", "برنامه نویسی", "باگ"}, 0.9},
	{"smalltalk", []string{"thank", "thanks", "how are you", "ممنون", "مرسی", "چطوری"}, 0.8},
}

// Detect classifies a user message.
func Detect(text string) Intent {
	lower := strings.ToLower(strings.TrimSpace(text))
	best := Intent{Name: "question", Confidence: 0.4, Entities: map[string]string{}}
	bestScore := 0.35
	for _, rule := range intentRules {
		score := 0.0
		for _, kw := range rule.keywords {
			if strings.Contains(lower, kw) {
				score += rule.weight
				if strings.HasPrefix(lower, kw) {
					score += rule.weight * 0.5
				}
			}
		}
		if score > bestScore {
			bestScore = score
			best = Intent{Name: rule.name, Confidence: minFloat64(0.99, 0.5+score/6), Entities: map[string]string{}}
		}
	}
	if best.Name == "question" {
		if isQuestion(lower) {
			best.Confidence = 0.6
		} else if len(lower) < 12 && (strings.Contains(lower, "hi") || strings.Contains(lower, "hey")) {
			best = Intent{Name: "greeting", Confidence: 0.7, Entities: map[string]string{}}
		} else {
			best.Name = "statement"
			best.Confidence = 0.4
		}
	}
	return best
}

func isQuestion(lower string) bool {
	if strings.HasSuffix(strings.TrimSpace(lower), "?") || strings.HasSuffix(strings.TrimSpace(lower), "؟") {
		return true
	}
	for _, prefix := range []string{"what", "why", "how", "who", "when", "where", "which", "can you", "could you", "is it", "are there",
		"چرا", "چگونه", "چطور", "کی", "کجا", "آیا", "چیست", "چند"} {
		if strings.HasPrefix(lower, prefix) {
			return true
		}
	}
	return false
}

func minFloat64(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}

// normalizeSpace collapses whitespace and trims control characters.
func normalizeSpace(s string) string {
	var b strings.Builder
	space := false
	for _, r := range s {
		if unicode.IsSpace(r) || unicode.IsControl(r) {
			if !space && b.Len() > 0 {
				b.WriteByte(' ')
			}
			space = true
			continue
		}
		space = false
		b.WriteRune(r)
	}
	return strings.TrimSpace(b.String())
}
