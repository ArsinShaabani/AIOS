package devices

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/ArsinShaabani/AIOS/internal/adapters/cache"
	"github.com/ArsinShaabani/AIOS/internal/adapters/storage/sqlstore"
	"github.com/ArsinShaabani/AIOS/internal/config"
	"github.com/ArsinShaabani/AIOS/internal/domain"
	"github.com/ArsinShaabani/AIOS/internal/ports"
)

type fakeTransport struct {
	result ports.CommandResult
	err    error
	sent   *ports.Command
}

func (f *fakeTransport) Name() string { return "fake" }
func (f *fakeTransport) Supports(p domain.DeviceProtocol) bool {
	return p == domain.ProtoMQTT || p == ""
}
func (f *fakeTransport) Send(ctx context.Context, d domain.Device, cmd ports.Command) (ports.CommandResult, error) {
	c := cmd
	f.sent = &c
	return f.result, f.err
}
func (f *fakeTransport) Subscribe(context.Context, func(ports.Telemetry)) error { return nil }
func (f *fakeTransport) Ping(context.Context) error                             { return nil }
func (f *fakeTransport) Close() error                                           { return nil }

func testDeviceService(t *testing.T) (*Service, *sqlstore.Repositories, *fakeTransport) {
	t.Helper()
	db, err := sqlstore.Open(context.Background(), config.DatabaseConfig{
		Driver: "sqlite", DSN: "devices.db", AutoMigrate: true, MaxOpenConns: 1,
	}, t.TempDir(), slog.Default())
	if err != nil {
		t.Fatal(err)
	}
	repos := sqlstore.NewRepositories(db)
	t.Cleanup(func() { _ = db.Close() })
	tr := &fakeTransport{result: ports.CommandResult{OK: true, Detail: "ack"}}
	svc := New(repos.Devices, []ports.DeviceTransport{tr}, cache.NewMemory(100, time.Minute), nil, nil, 8, slog.Default())
	return svc, repos, tr
}

func floatPtr(f float64) *float64 { return &f }
