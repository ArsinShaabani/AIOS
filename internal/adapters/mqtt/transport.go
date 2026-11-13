package mqtt

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/ArsinShaabani/AIOS/internal/domain"
	"github.com/ArsinShaabani/AIOS/internal/platform"
	"github.com/ArsinShaabani/AIOS/internal/ports"
)

// Transport implements ports.DeviceTransport over MQTT.
//
// Topics (default prefix "aios"):
//
//	aios/devices/<device_id>/cmd        command JSON (server -> device)
//	aios/devices/<device_id>/cmd/ack    acknowledgement (device -> server)
//	aios/devices/<device_id>/telemetry  telemetry JSON (device -> server)
//	aios/devices/<device_id>/status     online/offline (LWT)
//
// Devices may override the command topic with metadata["command_topic"] and the
// telemetry topic with metadata["telemetry_topic"].
type Transport struct {
	client  *Client
	prefix  string
	handler func(ports.Telemetry)
}

var _ ports.DeviceTransport = (*Transport)(nil)

// NewTransport wraps an MQTT client.
func NewTransport(client *Client, prefix string) *Transport {
	if prefix == "" {
		prefix = "aios"
	}
	return &Transport{client: client, prefix: strings.Trim(prefix, "/")}
}

// Name implements ports.DeviceTransport.
func (t *Transport) Name() string { return "mqtt" }

// Supports implements ports.DeviceTransport.
func (t *Transport) Supports(protocol domain.DeviceProtocol) bool {
	return protocol == domain.ProtoMQTT || protocol == ""
}

// Send publishes a validated command envelope.
func (t *Transport) Send(ctx context.Context, device domain.Device, cmd ports.Command) (ports.CommandResult, error) {
	start := time.Now()
	topic := t.commandTopic(device)
	payload, err := json.Marshal(map[string]any{
		"request_id": cmd.RequestID,
		"device_id":  device.ID,
		"capability": cmd.Capability,
		"value":      cmd.Value,
		"issued_at":  time.Now().UTC().Unix(),
		"expires_at": cmd.ExpiresAt,
	})
	if err != nil {
		return ports.CommandResult{}, platform.Wrap(platform.KindInternal, "mqtt.encode", "cannot encode command", err)
	}
	if err := t.client.Publish(ctx, topic, payload, false); err != nil {
		return ports.CommandResult{LatencyMS: time.Since(start).Milliseconds()}, err
	}
	return ports.CommandResult{OK: true, Detail: "published to " + topic, LatencyMS: time.Since(start).Milliseconds()}, nil
}

// Subscribe listens for telemetry from every device.
func (t *Transport) Subscribe(ctx context.Context, handler func(ports.Telemetry)) error {
	t.handler = handler
	patterns := []string{
		t.prefix + "/devices/+/telemetry",
		t.prefix + "/+/telemetry",
	}
	for _, pattern := range patterns {
		pattern := pattern
		if err := t.client.Subscribe(ctx, pattern, func(topic string, payload []byte) {
			t.handleMessage(topic, payload)
		}); err != nil {
			return err
		}
	}
	// Presence topic: mark devices offline on LWT.
	return t.client.Subscribe(ctx, t.prefix+"/devices/+/status", func(topic string, payload []byte) {
		deviceID := topicSegment(topic, len(strings.Split(topic, "/"))-2)
		status := strings.TrimSpace(string(payload))
		if deviceID == "" {
			return
		}
		if t.handler != nil {
			t.handler(ports.Telemetry{DeviceID: deviceID, Metric: "status", Value: status, At: time.Now().Unix()})
		}
	})
}

func (t *Transport) handleMessage(topic string, payload []byte) {
	var msg struct {
		DeviceID string         `json:"device_id"`
		Metric   string         `json:"metric"`
		Value    any            `json:"value"`
		Unit     string         `json:"unit"`
		At       int64          `json:"at"`
		Meta     map[string]any `json:"meta"`
	}
	if err := json.Unmarshal(payload, &msg); err != nil {
		return
	}
	if msg.DeviceID == "" {
		parts := strings.Split(topic, "/")
		// aios/devices/<id>/telemetry
		if len(parts) >= 4 && parts[len(parts)-1] == "telemetry" {
			msg.DeviceID = parts[len(parts)-2]
		}
	}
	if msg.Metric == "" {
		msg.Metric = "value"
	}
	if msg.At == 0 {
		msg.At = time.Now().Unix()
	}
	if t.handler != nil {
		t.handler(ports.Telemetry{
			DeviceID: msg.DeviceID, Metric: msg.Metric, Value: msg.Value,
			Unit: msg.Unit, At: msg.At, Meta: msg.Meta,
		})
	}
}

func (t *Transport) commandTopic(device domain.Device) string {
	if device.Metadata != nil {
		if topic, ok := device.Metadata["command_topic"].(string); ok && topic != "" {
			return topic
		}
	}
	return fmt.Sprintf("%s/devices/%s/cmd", t.prefix, device.ID)
}

func (t *Transport) telemetryTopic(device domain.Device) string {
	if device.Metadata != nil {
		if topic, ok := device.Metadata["telemetry_topic"].(string); ok && topic != "" {
			return topic
		}
	}
	return fmt.Sprintf("%s/devices/%s/telemetry", t.prefix, device.ID)
}

// Ping checks broker connectivity.
func (t *Transport) Ping(context.Context) error {
	if !t.client.Connected() {
		return platform.Unavailable("mqtt.disconnected", "MQTT broker is not connected")
	}
	return nil
}

// Close disconnects the client.
func (t *Transport) Close() error {
	t.client.Close()
	return nil
}

// PublishRaw implements the tools.MQTTPublisher port.
func (t *Transport) PublishRaw(ctx context.Context, topic string, payload []byte, retain bool) error {
	return t.client.Publish(ctx, topic, payload, retain)
}

// Publish implements tools.MQTTPublisher.
func (t *Transport) Publish(ctx context.Context, topic string, payload []byte, retain bool) error {
	return t.client.Publish(ctx, topic, payload, retain)
}

func topicSegment(topic string, index int) string {
	parts := strings.Split(topic, "/")
	if index < 0 || index >= len(parts) {
		return ""
	}
	return parts[index]
}
