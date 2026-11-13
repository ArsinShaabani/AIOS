package app

import (
	"context"

	"github.com/ArsinShaabani/AIOS/internal/devices"
	"github.com/ArsinShaabani/AIOS/internal/domain"
	"github.com/ArsinShaabani/AIOS/internal/memory"
	"github.com/ArsinShaabani/AIOS/internal/platform"
)

// deviceAPI adapts devices.Service to the narrow tools.DeviceAPI contract.
type deviceAPI struct{ svc *devices.Service }

func (d deviceAPI) List(ctx context.Context, userID string) ([]domain.Device, error) {
	items, _, err := d.svc.List(ctx, userID, deviceFilter{}, platform.Page{Limit: 200})
	return items, err
}

func (d deviceAPI) Resolve(ctx context.Context, userID, ref string) (*domain.Device, error) {
	return d.svc.Resolve(ctx, userID, ref)
}

func (d deviceAPI) Command(ctx context.Context, userID, ref, capability string, value any) (commandResult, error) {
	result, _, err := d.svc.Command(ctx, userID, ref, capability, value)
	return commandResult{OK: result.OK, Detail: result.Detail, LatencyMS: result.LatencyMS}, err
}

func (d deviceAPI) Telemetry(ctx context.Context, userID, ref string) (map[string]any, error) {
	state, _, err := d.svc.Telemetry(ctx, userID, ref)
	return state, err
}

// memoryAPI adapts memory.Service to tools.MemoryAPI.
type memoryAPI struct{ svc *memory.Service }

func (m memoryAPI) Search(ctx context.Context, userID, query string) ([]domain.Memory, error) {
	return m.svc.Recall(ctx, userID, query, 10)
}

func (m memoryAPI) Save(ctx context.Context, userID, content, kind string) (*domain.Memory, error) {
	k := domain.KindFact
	switch kind {
	case "preference":
		k = domain.KindPreference
	case "note", "episode":
		k = domain.KindEpisode
	}
	return m.svc.Remember(ctx, &domain.Memory{UserID: userID, Content: content, Kind: k, Source: "tool"})
}
