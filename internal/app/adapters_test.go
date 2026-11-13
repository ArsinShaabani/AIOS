package app

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/ArsinShaabani/AIOS/internal/domain"
	"github.com/ArsinShaabani/AIOS/internal/ports"
)

func TestToolAdaptersAgainstRealServices(t *testing.T) {
	a, err := New(context.Background(), testConfig(t), nil, "test")
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close(context.Background())
	ctx := context.Background()
	cmd := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"ok":true,"detail":"adapter ack"}`)
	}))
	defer cmd.Close()

	device, _, err := a.Devices.Register(ctx, "u1", &domain.Device{
		Name: "Adapter lamp", Kind: domain.DeviceLight, Protocol: domain.ProtoHTTP,
		Metadata:     map[string]any{"command_url": cmd.URL},
		Capabilities: []domain.Capability{{Name: "power", Type: "switch"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	api := deviceAPI{svc: a.Devices}
	list, err := api.List(ctx, "u1")
	if err != nil || len(list) != 1 {
		t.Fatalf("list: %v %d", err, len(list))
	}
	if resolved, err := api.Resolve(ctx, "u1", "Adapter lamp"); err != nil || resolved.ID != device.ID {
		t.Fatalf("resolve: %v", err)
	}
	result, err := api.Command(ctx, "u1", "Adapter lamp", "power", true)
	if err != nil || !result.OK {
		t.Fatalf("command: %v %+v", err, result)
	}
	state, err := api.Telemetry(ctx, "u1", "Adapter lamp")
	if err != nil || state == nil {
		t.Fatalf("telemetry: %v", err)
	}

	mem := memoryAPI{svc: a.Memory}
	saved, err := mem.Save(ctx, "u1", "likes adapter tests", "preference")
	if err != nil || saved.ID == "" || saved.Kind != domain.KindPreference {
		t.Fatalf("save: %v %+v", err, saved)
	}
	results, err := mem.Search(ctx, "u1", "adapter")
	if err != nil || len(results) == 0 {
		t.Fatalf("search: %v %+v", err, results)
	}
}

func TestBusAndUsageRecorder(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	bus := NewBus()
	bus.Start(ctx)
	events := make(chan ports.Event, 4)
	bus.Subscribe("test.event", func(_ context.Context, evt ports.Event) {
		events <- evt
	})
	bus.Subscribe("*", func(_ context.Context, evt ports.Event) {
		// wildcard subscription must also fire
	})
	bus.Publish(ctx, ports.Event{Name: "test.event", UserID: "u1", Payload: map[string]any{"x": 1}})
	select {
	case evt := <-events:
		if evt.UserID != "u1" {
			t.Fatalf("event: %+v", evt)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("event not delivered")
	}
	bus.Stop()

	a, err := New(context.Background(), testConfig(t), nil, "test")
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close(context.Background())
	recorder := NewUsageRecorder(a.Repos.Usage, a.Log)
	recorder.Start(context.Background())
	recorder.Record(context.Background(), domain.UsageRecord{
		ID: "u1", UserID: "u1", Provider: "local", Model: "m", Kind: "chat",
		PromptTokens: 5, CompletionTokens: 2, CostUSD: 0.001, LatencyMS: 12,
	})
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if n, _ := a.Repos.Usage.Count(context.Background()); n >= 1 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	recorder.Stop()
	if n, _ := a.Repos.Usage.Count(context.Background()); n < 1 {
		t.Fatal("usage record not persisted")
	}
	if quoteJSON("a\"b") != `"a\"b"` {
		t.Fatalf("quoteJSON: %s", quoteJSON("a\"b"))
	}
}
