package devices

import (
	"context"
	"testing"

	"github.com/ArsinShaabani/AIOS/internal/domain"
	"github.com/ArsinShaabani/AIOS/internal/platform"
	"github.com/ArsinShaabani/AIOS/internal/ports"
)

func TestCommandTelemetryAndLifecycle(t *testing.T) {
	svc, repos, tr := testDeviceService(t)
	ctx := context.Background()
	dev, token, err := svc.Register(ctx, "u1", &domain.Device{
		Name: "Lamp", Kind: domain.DeviceLight, Protocol: domain.ProtoMQTT,
		Capabilities: []domain.Capability{
			{Name: "power", Type: "switch"},
			{Name: "brightness", Type: "number", Min: floatPtr(0), Max: floatPtr(100)},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	res, _, err := svc.Command(ctx, "u1", "Lamp", "power", true)
	if err != nil || !res.OK || tr.sent == nil || tr.sent.Capability != "power" {
		t.Fatalf("command: %v %+v", err, res)
	}
	if _, _, err := svc.Command(ctx, "u1", "Lamp", "missing", true); !platform.IsInvalid(err) {
		t.Fatalf("unknown capability: %v", err)
	}
	if _, _, err := svc.Command(ctx, "u1", "Lamp", "brightness", 500); !platform.IsInvalid(err) {
		t.Fatalf("out of range: %v", err)
	}
	tr.err = platform.Unavailable("transport.down", "down")
	if _, _, err := svc.Command(ctx, "u1", "Lamp", "power", true); err == nil {
		t.Fatal("transport error not surfaced")
	}
	tr.err = nil

	if _, err := svc.SendTo(ctx, *dev, "nope", 1); err == nil {
		t.Fatal("SendTo unknown capability accepted")
	}
	if res, err := svc.SendTo(ctx, *dev, "power", false); err != nil || !res.OK {
		t.Fatalf("SendTo: %v", err)
	}
	unreachable := *dev
	unreachable.Protocol = domain.ProtoTCP
	if _, err := svc.SendTo(ctx, unreachable, "power", true); err == nil {
		t.Fatal("missing transport accepted")
	}

	svc.HandleTelemetry(ctx, ports.Telemetry{DeviceID: dev.ID, Metric: "temperature", Value: 21.5, Unit: "C"})
	state, gotDev, err := svc.Telemetry(ctx, "u1", "Lamp")
	if err != nil || state["temperature"].(float64) != 21.5 || gotDev.Status != "online" {
		t.Fatalf("telemetry: %v %+v %+v", err, state, gotDev)
	}
	svc.HandleTelemetry(ctx, ports.Telemetry{})

	if err := svc.Update(ctx, "u1", &domain.Device{ID: dev.ID, Room: "Study"}); err != nil {
		t.Fatal(err)
	}
	updated, _ := svc.Get(ctx, "u1", dev.ID)
	if updated.Room != "Study" || updated.Kind != domain.DeviceLight || len(updated.Capabilities) != 2 {
		t.Fatalf("partial update: %+v", updated)
	}
	if err := svc.Update(ctx, "u2", &domain.Device{ID: dev.ID, Name: "hack"}); !platform.IsForbidden(err) {
		t.Fatalf("update ownership: %v", err)
	}
	list, total, err := svc.List(ctx, "u1", ports.DeviceFilter{Kind: domain.DeviceLight}, platform.Page{Limit: 10})
	if err != nil || total != 1 || len(list) != 1 {
		t.Fatalf("list: %v %d", err, total)
	}
	if all, total, err := svc.ListAll(ctx, ports.DeviceFilter{}, platform.Page{Limit: 10}); err != nil || total != 1 || len(all) != 1 {
		t.Fatalf("list all: %v %d", err, total)
	}

	newToken, err := svc.RotateToken(ctx, "u1", dev.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Authenticate(ctx, token); err == nil {
		t.Fatal("old token still valid")
	}
	if _, err := svc.Authenticate(ctx, newToken); err != nil {
		t.Fatal(err)
	}
	if err := svc.Delete(ctx, "u2", dev.ID); !platform.IsForbidden(err) {
		t.Fatalf("delete ownership: %v", err)
	}
	if err := svc.Delete(ctx, "u1", dev.ID); err != nil {
		t.Fatal(err)
	}
	if n, _ := repos.Devices.Count(ctx); n != 0 {
		t.Fatalf("device not deleted: %d", n)
	}
}
