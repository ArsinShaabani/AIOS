package devices

import (
	"context"
	"testing"

	"github.com/ArsinShaabani/AIOS/internal/domain"
	"github.com/ArsinShaabani/AIOS/internal/platform"
)

func TestRegisterAuthenticateResolve(t *testing.T) {
	svc, _, _ := testDeviceService(t)
	ctx := context.Background()
	if _, _, err := svc.Register(ctx, "", &domain.Device{Name: "x"}); !platform.IsUnauthorized(err) {
		t.Fatalf("anonymous register: %v", err)
	}
	if _, _, err := svc.Register(ctx, "u1", &domain.Device{}); !platform.IsInvalid(err) {
		t.Fatalf("empty name: %v", err)
	}
	if _, _, err := svc.Register(ctx, "u1", &domain.Device{Name: "x", Capabilities: []domain.Capability{{Name: "a"}, {Name: "a"}}}); !platform.IsInvalid(err) {
		t.Fatalf("duplicate capability: %v", err)
	}
	dev, token, err := svc.Register(ctx, "u1", &domain.Device{
		Name: "Kitchen lamp", Kind: domain.DeviceLight, Room: "Kitchen",
		Capabilities: []domain.Capability{{Name: "power", Type: "switch"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if token == "" || dev.ID == "" || dev.Status != "offline" || dev.TokenHash == "" {
		t.Fatalf("register: %+v", dev)
	}
	auth, err := svc.Authenticate(ctx, token)
	if err != nil || auth.ID != dev.ID {
		t.Fatalf("authenticate: %v", err)
	}
	if _, err := svc.Authenticate(ctx, "dev_wrong"); !platform.IsUnauthorized(err) {
		t.Fatalf("wrong token: %v", err)
	}
	if _, err := svc.Authenticate(ctx, ""); !platform.IsUnauthorized(err) {
		t.Fatalf("empty token: %v", err)
	}
	if got, err := svc.Resolve(ctx, "u1", dev.ID); err != nil || got.ID != dev.ID {
		t.Fatalf("resolve id: %v", err)
	}
	if got, err := svc.Resolve(ctx, "u1", "kitchen lamp"); err != nil || got.ID != dev.ID {
		t.Fatalf("resolve name: %v", err)
	}
	if got, err := svc.Resolve(ctx, "u1", "Kitchen"); err != nil || got.ID != dev.ID {
		t.Fatalf("resolve room: %v", err)
	}
	if _, err := svc.Resolve(ctx, "u1", "missing"); !platform.IsNotFound(err) {
		t.Fatalf("missing resolve: %v", err)
	}
	if _, err := svc.Resolve(ctx, "u2", dev.ID); !platform.IsNotFound(err) {
		t.Fatalf("ownership: %v", err)
	}
	if _, err := svc.Resolve(ctx, "u1", ""); !platform.IsInvalid(err) {
		t.Fatalf("empty ref: %v", err)
	}
}
