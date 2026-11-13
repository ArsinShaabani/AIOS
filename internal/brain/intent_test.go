package brain

import "testing"

func TestDetectIntents(t *testing.T) {
	cases := []struct {
		text string
		want string
	}{
		{"hello there", "greeting"},
		{"turn on the kitchen light", "device_control"},
		{"remind me tomorrow at 9", "schedule"},
		{"search the latest AI news", "search"},
		{"what is the capital of France?", "question"},
		{"write a python function to sort a list", "code"},
		{"remember that I prefer tea", "memory"},
		{"سلام", "greeting"},
		{"فردا به من یادآوری کن", "schedule"},
	}
	for _, c := range cases {
		got := Detect(c.text)
		if got.Name != c.want {
			t.Errorf("Detect(%q)=%s want %s", c.text, got.Name, c.want)
		}
		if got.Confidence <= 0 || got.Confidence > 1 {
			t.Errorf("bad confidence for %q: %f", c.text, got.Confidence)
		}
	}
}

func TestNormalizeSpace(t *testing.T) {
	if got := normalizeSpace("  hello \n\t world  "); got != "hello world" {
		t.Fatalf("got %q", got)
	}
}
