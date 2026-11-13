package llm

import (
	"context"
	"sync"

	"github.com/ArsinShaabani/AIOS/internal/domain"
	"github.com/ArsinShaabani/AIOS/internal/platform"
	"github.com/ArsinShaabani/AIOS/internal/ports"
)

type fakeProvider struct {
	name       string
	failures   int
	chatCalls  int
	embedCalls int
	mu         sync.Mutex
	err        error
	content    string
	embed      []float32
	embedErr   error
	streamErr  error
}

func (f *fakeProvider) Name() string { return f.name }
func (f *fakeProvider) Capabilities() ports.Capabilities {
	return ports.Capabilities{Tools: true, Streaming: true, Embeddings: f.embed != nil}
}
func (f *fakeProvider) Models() []string { return []string{f.name + "-model"} }
func (f *fakeProvider) Chat(ctx context.Context, req ports.ChatRequest) (*ports.ChatResponse, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.chatCalls++
	if f.failures > 0 {
		f.failures--
		err := f.err
		if err == nil {
			err = platform.Unavailable("fake.down", "provider down")
		}
		return nil, err
	}
	content := f.content
	if content == "" {
		content = "ok"
	}
	return &ports.ChatResponse{
		Provider: f.name, Model: req.Model, Content: content, FinishReason: "stop",
		Usage: ports.Usage{PromptTokens: 10, CompletionTokens: 5, TotalTokens: 15},
	}, nil
}
func (f *fakeProvider) Stream(ctx context.Context, req ports.ChatRequest) (<-chan ports.Chunk, error) {
	f.mu.Lock()
	f.chatCalls++
	err := f.streamErr
	f.mu.Unlock()
	if err != nil {
		return nil, err
	}
	ch := make(chan ports.Chunk, 4)
	ch <- ports.Chunk{Delta: "he"}
	ch <- ports.Chunk{Delta: "llo", Done: true, Usage: &ports.Usage{PromptTokens: 1, CompletionTokens: 2}}
	close(ch)
	return ch, nil
}
func (f *fakeProvider) Embed(ctx context.Context, model string, inputs []string) ([][]float32, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.embedCalls++
	if f.embedErr != nil {
		return nil, f.embedErr
	}
	if f.embed == nil {
		return nil, ports.ErrNotSupported
	}
	out := make([][]float32, len(inputs))
	for i := range inputs {
		out[i] = f.embed
	}
	return out, nil
}
func (f *fakeProvider) Ping(context.Context) error { return nil }

type usageSink struct {
	mu   sync.Mutex
	recs []domain.UsageRecord
}

func (u *usageSink) Record(_ context.Context, rec domain.UsageRecord) {
	u.mu.Lock()
	defer u.mu.Unlock()
	u.recs = append(u.recs, rec)
}

func (u *usageSink) count() int {
	u.mu.Lock()
	defer u.mu.Unlock()
	return len(u.recs)
}
