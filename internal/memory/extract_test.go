package memory

import (
	"strings"
	"testing"

	"github.com/ArsinShaabani/AIOS/internal/domain"
)

func TestExtractEnglish(t *testing.T) {
	cases := []struct {
		text string
		want string
		kind domain.MemoryKind
	}{
		{"My name is Ada Lovelace", "name is Ada Lovelace", domain.KindFact},
		{"I live in Tehran", "lives in Tehran", domain.KindFact},
		{"I love dark chocolate", "likes dark chocolate", domain.KindPreference},
		{"My favourite colour is teal", "favourite colour is teal", domain.KindPreference},
		{"Remember that the garage code is 4411", "garage code is 4411", domain.KindFact},
	}
	for _, c := range cases {
		got := Extract(c.text)
		if len(got) == 0 {
			t.Fatalf("no memory extracted from %q", c.text)
		}
		found := false
		for _, m := range got {
			if strings.Contains(m.Content, c.want) {
				found = true
				if m.Kind != c.kind {
					t.Fatalf("%q kind=%s want %s", c.text, m.Kind, c.kind)
				}
				if m.Importance <= 0 || m.Summary == "" {
					t.Fatal("memory metadata missing")
				}
			}
		}
		if !found {
			t.Fatalf("%q extracted %+v", c.text, got)
		}
	}
}

func TestExtractPersian(t *testing.T) {
	got := Extract("اسم من آرش است")
	if len(got) == 0 || !strings.Contains(got[0].Content, "آرش") {
		t.Fatalf("persian name not extracted: %+v", got)
	}
	got = Extract("یادت باشه فردا ساعت ۹ جلسه دارم")
	if len(got) == 0 || !strings.Contains(got[0].Content, "جلسه") {
		t.Fatalf("persian explicit memory not extracted: %+v", got)
	}
}

func TestExtractSkipsSecretsAndQuestions(t *testing.T) {
	if got := Extract("my password is hunter2"); len(got) != 0 {
		t.Fatalf("secrets must never be auto-stored: %+v", got)
	}
	if got := Extract("What is my name?"); len(got) != 0 {
		t.Fatalf("questions must not be stored: %+v", got)
	}
	if got := Extract("hi"); len(got) != 0 {
		t.Fatalf("too-short input stored: %+v", got)
	}
	if Fingerprint("abc") == Fingerprint("abd") {
		t.Fatal("fingerprints must differ")
	}
}
