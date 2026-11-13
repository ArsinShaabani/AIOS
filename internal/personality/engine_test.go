package personality

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/ArsinShaabani/AIOS/internal/domain"
	"github.com/ArsinShaabani/AIOS/internal/platform"
)

type fakeRepo struct {
	items map[string]domain.Personality
}

func newFakeRepo() *fakeRepo { return &fakeRepo{items: map[string]domain.Personality{}} }

func (f *fakeRepo) Create(_ context.Context, p *domain.Personality) error {
	if _, ok := f.items[p.ID]; ok {
		return platform.Conflict("personality.exists", "exists")
	}
	f.items[p.ID] = *p
	return nil
}
func (f *fakeRepo) Update(_ context.Context, p *domain.Personality) error {
	if _, ok := f.items[p.ID]; !ok {
		return platform.NotFound("personality.missing", "missing")
	}
	f.items[p.ID] = *p
	return nil
}
func (f *fakeRepo) Get(_ context.Context, id string) (*domain.Personality, error) {
	p, ok := f.items[id]
	if !ok {
		return nil, platform.NotFound("personality.missing", "missing")
	}
	return &p, nil
}
func (f *fakeRepo) Delete(_ context.Context, id string) error {
	delete(f.items, id)
	return nil
}
func (f *fakeRepo) ListAvailable(_ context.Context, ownerID string) ([]domain.Personality, error) {
	out := make([]domain.Personality, 0, len(f.items))
	for _, p := range f.items {
		if p.OwnerID == "" || p.OwnerID == ownerID {
			out = append(out, p)
		}
	}
	return out, nil
}
func (f *fakeRepo) Count(context.Context) (int, error) { return len(f.items), nil }

func TestSeedResolveCompose(t *testing.T) {
	repo := newFakeRepo()
	engine := New(repo)
	if err := engine.Seed(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(repo.items) != len(Presets()) {
		t.Fatalf("presets not seeded: %d", len(repo.items))
	}
	if err := engine.Seed(context.Background()); err != nil {
		t.Fatal(err) // idempotent
	}
	p := engine.Resolve(context.Background(), DefaultID)
	if p == nil || p.Name != "Balanced" {
		t.Fatalf("resolve default: %+v", p)
	}
	fallback := engine.Resolve(context.Background(), "does-not-exist")
	if fallback == nil {
		t.Fatal("unknown id must fall back")
	}

	hd := domain.Personality{Name: "Test", Tone: "playful", Humor: 99, EmotionalLevel: -3, ResponseLength: "epic"}
	hd.Clamp()
	if hd.Humor != 10 || hd.EmotionalLevel != 0 || hd.ResponseLength != "medium" {
		t.Fatalf("clamp: %+v", hd)
	}
	text := Compose(&domain.Personality{Name: "Ada", Tone: "warm", Humor: 8, EmotionalLevel: 9, ResponseLength: "short", CustomPrompt: "Use Farsi."})
	for _, want := range []string{"Ada", "playful", "warm and expressive", "1-3 sentences", "Use Farsi."} {
		if !strings.Contains(text, want) {
			t.Fatalf("compose missing %q: %s", want, text)
		}
	}
	full := SystemPrompt("BASE", &domain.Personality{Name: "P"}, "AGENT", "EXTRA")
	for _, want := range []string{"BASE", "Persona: P", "AGENT", "EXTRA", "never invent"} {
		if !strings.Contains(full, want) {
			t.Fatalf("system prompt missing %q", want)
		}
	}
	_ = time.Now
}
