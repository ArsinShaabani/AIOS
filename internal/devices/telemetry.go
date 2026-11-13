package devices

import (
	"context"
	"encoding/json"
	"time"

	"github.com/ArsinShaabani/AIOS/internal/domain"
	"github.com/ArsinShaabani/AIOS/internal/platform"
	"github.com/ArsinShaabani/AIOS/internal/ports"
)

// SendTo dispatches a raw command to a known device (used by the MQTT bridge and
// Home Assistant integration).
func (s *Service) SendTo(ctx context.Context, device domain.Device, capability string, value any) (ports.CommandResult, error) {
	capDef, ok := device.Capability(capability)
	if !ok {
		return ports.CommandResult{}, platform.Invalid("device.capability", "unknown capability "+capability)
	}
	normalised, err := validateValue(capDef, value)
	if err != nil {
		return ports.CommandResult{}, err
	}
	transport := s.transportFor(device.Protocol)
	if transport == nil {
		return ports.CommandResult{}, platform.Unavailable("device.transport", "no transport for "+string(device.Protocol))
	}
	return transport.Send(ctx, device, ports.Command{
		RequestID: platform.NewID("cmd"), DeviceID: device.ID, Capability: capability,
		Value: normalised, ExpiresAt: time.Now().Add(30 * time.Second).Unix(),
	})
}

// Telemetry returns the latest cached telemetry for a device.
func (s *Service) Telemetry(ctx context.Context, userID, ref string) (map[string]any, *domain.Device, error) {
	device, err := s.Resolve(ctx, userID, ref)
	if err != nil {
		return nil, nil, err
	}
	out := map[string]any{}
	if s.cache != nil {
		raw, ok, _ := s.cache.Get(ctx, "device:telemetry:"+device.ID)
		if ok {
			_ = json.Unmarshal(raw, &out)
		}
	}
	return out, device, nil
}

// HandleTelemetry merges an inbound device reading, updates presence and
// publishes an event for automations.
func (s *Service) HandleTelemetry(ctx context.Context, t ports.Telemetry) {
	if t.DeviceID == "" || t.Metric == "" {
		return
	}
	if s.cache != nil {
		key := "device:telemetry:" + t.DeviceID
		state := map[string]any{}
		if raw, ok, _ := s.cache.Get(ctx, key); ok {
			_ = json.Unmarshal(raw, &state)
		}
		state[t.Metric] = t.Value
		state["_updated_at"] = time.Now().UTC().Format(time.RFC3339)
		if raw, err := json.Marshal(state); err == nil {
			_ = s.cache.Set(ctx, key, raw, 24*time.Hour)
		}
	}
	s.Touch(ctx, t.DeviceID)
	if s.events != nil {
		s.events.Publish(ctx, ports.Event{Name: "device.telemetry", Payload: map[string]any{
			"device_id": t.DeviceID, "metric": t.Metric, "value": t.Value, "unit": t.Unit,
		}})
	}
}
