package domain

import (
	"encoding/json"
	"time"
)

// Agent is a specialised planner persona with a tool allow-list. Agents are
// data, not code, so operators can add them from the dashboard at runtime.
type Agent struct {
	ID           string    `json:"id"`
	OwnerID      string    `json:"owner_id,omitempty"`
	Name         string    `json:"name"`
	Description  string    `json:"description"`
	SystemPrompt string    `json:"system_prompt"`
	Tools        []string  `json:"tools"`
	Keywords     []string  `json:"keywords"`
	ModelHint    string    `json:"model_hint,omitempty"`
	Priority     int       `json:"priority"`
	Enabled      bool      `json:"enabled"`
	Builtin      bool      `json:"builtin"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// DeviceKind classifies hardware for capability defaults and UI grouping.
type DeviceKind string

const (
	DeviceRobot     DeviceKind = "robot"
	DeviceSensor    DeviceKind = "sensor"
	DeviceActuator  DeviceKind = "actuator"
	DeviceLight     DeviceKind = "light"
	DeviceESP32     DeviceKind = "esp32"
	DeviceRaspberry DeviceKind = "raspberry_pi"
	DeviceSTM32     DeviceKind = "stm32"
	DeviceROS2      DeviceKind = "ros2"
	DeviceArduino   DeviceKind = "arduino"
	DeviceGateway   DeviceKind = "gateway"
	DeviceAppliance DeviceKind = "appliance"
)

// DeviceProtocol is the transport used to reach a device.
type DeviceProtocol string

const (
	ProtoMQTT      DeviceProtocol = "mqtt"
	ProtoWebSocket DeviceProtocol = "ws"
	ProtoHTTP      DeviceProtocol = "http"
	ProtoTCP       DeviceProtocol = "tcp"
	ProtoRosbridge DeviceProtocol = "rosbridge"
)

// Capability describes a device function, including safety bounds for
// actuators (e.g. max velocity) enforced by the robotics layer.
type Capability struct {
	Name    string   `json:"name"`
	Type    string   `json:"type"` // switch | number | string | color | enum
	Unit    string   `json:"unit,omitempty"`
	Min     *float64 `json:"min,omitempty"`
	Max     *float64 `json:"max,omitempty"`
	Enum    []string `json:"enum,omitempty"`
	Command string   `json:"command,omitempty"`
}

// Device is a registered robot, sensor, actuator or smart-home node.
type Device struct {
	ID           string         `json:"id"`
	UserID       string         `json:"user_id"`
	Name         string         `json:"name"`
	Kind         DeviceKind     `json:"kind"`
	Protocol     DeviceProtocol `json:"protocol"`
	Hardware     string         `json:"hardware,omitempty"`
	Room         string         `json:"room,omitempty"`
	Status       string         `json:"status"` // online | offline | unknown
	Capabilities []Capability   `json:"capabilities"`
	Metadata     map[string]any `json:"metadata,omitempty"`
	TokenHash    string         `json:"-"`
	TokenPrefix  string         `json:"token_prefix,omitempty"`
	Firmware     string         `json:"firmware,omitempty"`
	LastSeenAt   *time.Time     `json:"last_seen_at,omitempty"`
	CreatedAt    time.Time      `json:"created_at"`
	UpdatedAt    time.Time      `json:"updated_at"`
}

// Capability returns a capability by name.
func (d *Device) Capability(name string) (Capability, bool) {
	for _, c := range d.Capabilities {
		if c.Name == name {
			return c, true
		}
	}
	return Capability{}, false
}

// HasCapability reports whether the device exposes name.
func (d *Device) HasCapability(name string) bool {
	_, ok := d.Capability(name)
	return ok
}

// PluginTool is a single function exposed by a plugin. Parameters is a JSON
// Schema object mirroring the LLM tool schema.
type PluginTool struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Parameters  json.RawMessage `json:"parameters,omitempty"`
	Method      string          `json:"method,omitempty"`
	Path        string          `json:"path,omitempty"`
}

// Plugin is an out-of-process extension (HTTP webhook or built-in adapter).
type Plugin struct {
	ID          string          `json:"id"`
	Name        string          `json:"name"`
	Version     string          `json:"version"`
	Description string          `json:"description"`
	Author      string          `json:"author,omitempty"`
	Homepage    string          `json:"homepage,omitempty"`
	Kind        string          `json:"kind"` // http | builtin
	Endpoint    string          `json:"endpoint,omitempty"`
	Permissions []string        `json:"permissions"`
	Tools       []PluginTool    `json:"tools"`
	Manifest    json.RawMessage `json:"manifest,omitempty"`
	Enabled     bool            `json:"enabled"`
	Verified    bool            `json:"verified"`
	InstalledAt time.Time       `json:"installed_at"`
	UpdatedAt   time.Time       `json:"updated_at"`
}

// Permission constants for plugins and API keys.
const (
	PermChat         = "chat"
	PermMemoryRead   = "memory:read"
	PermMemoryWrite  = "memory:write"
	PermDevicesRead  = "devices:read"
	PermDevicesWrite = "devices:write"
	PermInternet     = "internet"
	PermAdmin        = "admin"
)
