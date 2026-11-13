package sqlstore

import (
	"context"
	"testing"
	"time"

	"github.com/ArsinShaabani/AIOS/internal/domain"
	"github.com/ArsinShaabani/AIOS/internal/platform"
	"github.com/ArsinShaabani/AIOS/internal/ports"
)

func TestDeviceRepository(t *testing.T) {
	ctx := context.Background()
	r := newTestStore(t)
	d := &domain.Device{
		ID: "dev1", UserID: "u1", Name: "Lamp", Kind: domain.DeviceLight, Protocol: domain.ProtoMQTT,
		Room: "Kitchen", Status: "offline", Capabilities: []domain.Capability{{Name: "power", Type: "switch"}},
		Metadata: map[string]any{"command_topic": "home/lamp/set"}, TokenHash: "hash", TokenPrefix: "dev_prefix",
	}
	if err := r.Devices.Create(ctx, d); err != nil {
		t.Fatal(err)
	}
	got, err := r.Devices.Get(ctx, "dev1")
	if err != nil || got.Name != "Lamp" || len(got.Capabilities) != 1 {
		t.Fatalf("get device: %v %+v", err, got)
	}
	byToken, err := r.Devices.GetByTokenPrefix(ctx, "dev_prefix")
	if err != nil || byToken.ID != "dev1" {
		t.Fatalf("token lookup: %v", err)
	}
	got.Room = "Study"
	if err := r.Devices.Update(ctx, got); err != nil {
		t.Fatal(err)
	}
	if err := r.Devices.SetStatus(ctx, "dev1", "online", time.Now()); err != nil {
		t.Fatal(err)
	}
	list, total, err := r.Devices.ListByUser(ctx, "u1", ports.DeviceFilter{Kind: domain.DeviceLight, Status: "online", Query: "lamp"}, platform.Page{Limit: 10})
	if err != nil || total != 1 || len(list) != 1 {
		t.Fatalf("list user: %v %d %+v", err, total, list)
	}
	all, total, err := r.Devices.ListAll(ctx, ports.DeviceFilter{}, platform.Page{Limit: 10})
	if err != nil || total != 1 || len(all) != 1 {
		t.Fatalf("list all: %v %d", err, total)
	}
	if n, err := r.Devices.Count(ctx); err != nil || n != 1 {
		t.Fatalf("count: %v %d", err, n)
	}
	statuses, err := r.Devices.CountByStatus(ctx)
	if err != nil || statuses["online"] != 1 {
		t.Fatalf("status counts: %v %+v", err, statuses)
	}
	if err := r.Devices.Delete(ctx, "dev1"); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Devices.Get(ctx, "dev1"); !platform.IsNotFound(err) {
		t.Fatalf("deleted device readable: %v", err)
	}
}

func TestPluginRepositoryAndAudit(t *testing.T) {
	ctx := context.Background()
	r := newTestStore(t)
	p := &domain.Plugin{ID: "p1", Name: "weather", Version: "1.0.0", Kind: "http", Endpoint: "http://x",
		Permissions: []string{"internet"}, Tools: []domain.PluginTool{{Name: "w", Description: "d"}}, Enabled: true}
	if err := r.Plugins.Upsert(ctx, p); err != nil {
		t.Fatal(err)
	}
	p.Version = "1.1.0"
	if err := r.Plugins.Upsert(ctx, p); err != nil {
		t.Fatal(err)
	}
	got, err := r.Plugins.Get(ctx, "p1")
	if err != nil || got.Version != "1.1.0" || len(got.Tools) != 1 {
		t.Fatalf("plugin get: %v %+v", err, got)
	}
	if list, err := r.Plugins.List(ctx); err != nil || len(list) != 1 {
		t.Fatalf("plugin list: %v", err)
	}
	if n, _ := r.Plugins.Count(ctx); n != 1 {
		t.Fatalf("plugin count: %d", n)
	}
	if err := r.Plugins.Delete(ctx, "p1"); err != nil {
		t.Fatal(err)
	}

	base := time.Now().UTC().Add(-time.Hour)
	for i, action := range []string{"auth.login", "device.command", "device.command"} {
		e := &domain.AuditEvent{ID: "a" + string(rune('1'+i)), ActorID: "u1", ActorType: "user", Action: action,
			Resource: "device", Status: "success", IP: "127.0.0.1", CreatedAt: base.Add(time.Duration(i) * time.Minute)}
		if err := r.Audit.Create(ctx, e); err != nil {
			t.Fatal(err)
		}
	}
	list, total, err := r.Audit.List(ctx, ports.AuditFilter{ActorID: "u1", Action: "device.command"}, platform.Page{Limit: 10})
	if err != nil || total != 2 || len(list) != 2 {
		t.Fatalf("audit list: %v %d", err, total)
	}
	if _, total, err := r.Audit.List(ctx, ports.AuditFilter{Status: "success", From: &base,
		To: func() *time.Time { x := time.Now().Add(time.Hour); return &x }()}, platform.Page{}); err != nil || total != 3 {
		t.Fatalf("audit window: %v %d", err, total)
	}
	if n, _ := r.Audit.Count(ctx); n != 3 {
		t.Fatalf("audit count: %d", n)
	}
	if n, err := r.Audit.DeleteBefore(ctx, time.Now().Add(time.Hour)); err != nil || n != 3 {
		t.Fatalf("audit prune: %v %d", err, n)
	}
}

func TestUsageRepository(t *testing.T) {
	ctx := context.Background()
	r := newTestStore(t)
	now := time.Now().UTC()
	for i := 0; i < 3; i++ {
		rec := &domain.UsageRecord{
			ID: "u" + string(rune('1'+i)), UserID: "u1", Provider: "openai", Model: "gpt",
			Kind: "chat", PromptTokens: 10, CompletionTokens: 5, CostUSD: 0.01, LatencyMS: 100,
			CreatedAt: now.AddDate(0, 0, -i),
		}
		if err := r.Usage.Create(ctx, rec); err != nil {
			t.Fatal(err)
		}
	}
	totals, err := r.Usage.Totals(ctx, "u1", now.AddDate(0, 0, -7), now.Add(time.Hour))
	if err != nil || totals.Requests != 3 || totals.PromptTokens != 30 || totals.CompletionTokens != 15 {
		t.Fatalf("totals: %v %+v", err, totals)
	}
	if all, err := r.Usage.Totals(ctx, "", now.AddDate(0, 0, -7), now.Add(time.Hour)); err != nil || all.Requests != 3 {
		t.Fatalf("totals all: %v %+v", err, all)
	}
	daily, err := r.Usage.DailySeries(ctx, "u1", now.AddDate(0, 0, -7), now.Add(time.Hour))
	if err != nil || len(daily) < 1 {
		t.Fatalf("daily: %v %+v", err, daily)
	}
	byProvider, err := r.Usage.ByProvider(ctx, now.AddDate(0, 0, -7), now.Add(time.Hour))
	if err != nil || len(byProvider) != 1 || byProvider[0].Provider != "openai" {
		t.Fatalf("by provider: %v %+v", err, byProvider)
	}
	if n, _ := r.Usage.Count(ctx); n != 3 {
		t.Fatalf("usage count: %d", n)
	}
}
