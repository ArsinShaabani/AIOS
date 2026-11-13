package devices

import (
	"context"
	"strconv"
	"strings"
	"time"

	"github.com/ArsinShaabani/AIOS/internal/domain"
	"github.com/ArsinShaabani/AIOS/internal/platform"
	"github.com/ArsinShaabani/AIOS/internal/ports"
)

// Command validates and dispatches a capability command.
func (s *Service) Command(ctx context.Context, userID, ref, capability string, value any) (ports.CommandResult, *domain.Device, error) {
	device, err := s.Resolve(ctx, userID, ref)
	if err != nil {
		return ports.CommandResult{}, nil, err
	}
	capDef, ok := device.Capability(capability)
	if !ok {
		return ports.CommandResult{}, device, platform.Invalid("device.capability",
			"device "+device.Name+" has no capability "+capability)
	}
	normalised, err := validateValue(capDef, value)
	if err != nil {
		return ports.CommandResult{}, device, err
	}
	transport := s.transportFor(device.Protocol)
	if transport == nil {
		return ports.CommandResult{}, device, platform.Unavailable("device.transport",
			"no transport is configured for protocol "+string(device.Protocol))
	}
	start := time.Now()
	result, err := transport.Send(ctx, *device, ports.Command{
		RequestID:  platform.NewID("cmd"),
		DeviceID:   device.ID,
		Capability: capability,
		Value:      normalised,
		UserID:     userID,
		ExpiresAt:  time.Now().Add(30 * time.Second).Unix(),
	})
	if result.LatencyMS == 0 {
		result.LatencyMS = time.Since(start).Milliseconds()
	}
	status := "success"
	if err != nil || !result.OK {
		status = "failure"
	}
	if s.audit != nil {
		s.audit.Log(ctx, userID, "user", "device.command", "device", device.ID, status, "", "",
			map[string]any{"capability": capability, "value": normalised, "detail": platform.Truncate(result.Detail, 200)})
	}
	if err != nil {
		return result, device, err
	}
	if s.events != nil {
		s.events.Publish(ctx, ports.Event{Name: "device.command", UserID: userID, Payload: map[string]any{
			"device_id": device.ID, "capability": capability, "value": normalised, "ok": result.OK,
		}})
	}
	s.Touch(ctx, device.ID)
	return result, device, nil
}

func (s *Service) transportFor(protocol domain.DeviceProtocol) ports.DeviceTransport {
	for _, t := range s.transports {
		if t != nil && t.Supports(protocol) {
			return t
		}
	}
	return nil
}

// validateValue coerces and bounds a capability value. This is the safety layer
// for robotics: a model (or a malicious plugin) cannot exceed declared limits.
func validateValue(cap domain.Capability, value any) (any, error) {
	switch cap.Type {
	case "switch", "bool", "boolean":
		switch v := value.(type) {
		case bool:
			return v, nil
		case string:
			switch strings.ToLower(strings.TrimSpace(v)) {
			case "on", "true", "1", "yes", "open":
				return true, nil
			case "off", "false", "0", "no", "close":
				return false, nil
			}
		case float64:
			return v != 0, nil
		case int:
			return v != 0, nil
		}
		return nil, platform.Invalid("device.value", "expected a boolean for capability "+cap.Name)
	case "number":
		var f float64
		switch v := value.(type) {
		case float64:
			f = v
		case int:
			f = float64(v)
		case int64:
			f = float64(v)
		case string:
			parsed, err := strconv.ParseFloat(strings.TrimSpace(v), 64)
			if err != nil {
				return nil, platform.Invalid("device.value", "expected a number for capability "+cap.Name)
			}
			f = parsed
		default:
			return nil, platform.Invalid("device.value", "expected a number for capability "+cap.Name)
		}
		if cap.Min != nil && f < *cap.Min {
			return nil, platform.Invalid("device.range", "value below the safety minimum for "+cap.Name)
		}
		if cap.Max != nil && f > *cap.Max {
			return nil, platform.Invalid("device.range", "value above the safety maximum for "+cap.Name)
		}
		return f, nil
	case "enum":
		str, ok := value.(string)
		if !ok {
			return nil, platform.Invalid("device.value", "expected one of the allowed strings for "+cap.Name)
		}
		for _, allowed := range cap.Enum {
			if strings.EqualFold(allowed, str) {
				return allowed, nil
			}
		}
		return nil, platform.Invalid("device.enum", "value is not in the allowed set for "+cap.Name)
	default:
		if value == nil {
			return nil, platform.Invalid("device.value", "value is required for capability "+cap.Name)
		}
		return value, nil
	}
}
