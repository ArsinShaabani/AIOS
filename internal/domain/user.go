// Package domain holds AIOS' entities and value objects. It is pure Go with no
// transport, storage or framework imports so the same model can back REST,
// GraphQL, MQTT and voice front-ends.
package domain

import (
	"strings"
	"time"

	"github.com/ArsinShaabani/AIOS/internal/platform"
)

// Role is the RBAC role attached to a principal.
type Role string

const (
	RoleAdmin    Role = "admin"
	RoleOperator Role = "operator"
	RoleUser     Role = "user"
	RoleViewer   Role = "viewer"
	RoleDevice   Role = "device"
)

// Valid reports whether the role is known.
func (r Role) Valid() bool {
	switch r {
	case RoleAdmin, RoleOperator, RoleUser, RoleViewer, RoleDevice:
		return true
	}
	return false
}

// User is a human (or service) principal that owns sessions, devices and
// memories. PasswordHash is never serialised.
type User struct {
	ID            string         `json:"id"`
	Email         string         `json:"email"`
	DisplayName   string         `json:"display_name"`
	PasswordHash  string         `json:"-"`
	Role          Role           `json:"role"`
	Status        string         `json:"status"` // active | disabled
	PersonalityID string         `json:"personality_id,omitempty"`
	Locale        string         `json:"locale"`
	Timezone      string         `json:"timezone"`
	Metadata      map[string]any `json:"metadata,omitempty"`
	LastLoginAt   *time.Time     `json:"last_login_at,omitempty"`
	CreatedAt     time.Time      `json:"created_at"`
	UpdatedAt     time.Time      `json:"updated_at"`
}

// IsActive reports whether the account may authenticate.
func (u *User) IsActive() bool { return u != nil && u.Status == "active" }

// NormalizeEmail lower-cases and trims an email address.
func NormalizeEmail(email string) string { return strings.ToLower(strings.TrimSpace(email)) }

// Validate checks invariants before persistence.
func (u *User) Validate() error {
	if strings.TrimSpace(u.Email) == "" || !strings.Contains(u.Email, "@") {
		return platform.Invalid("user.email_invalid", "a valid email is required")
	}
	if !u.Role.Valid() {
		return platform.Invalid("user.role_invalid", "unknown role: "+string(u.Role))
	}
	if u.Status == "" {
		u.Status = "active"
	}
	if u.Status != "active" && u.Status != "disabled" {
		return platform.Invalid("user.status_invalid", "status must be active or disabled")
	}
	if u.Locale == "" {
		u.Locale = "en"
	}
	if u.Timezone == "" {
		u.Timezone = "UTC"
	}
	return nil
}

// Public returns a copy safe to send to clients.
func (u User) Public() User {
	u.PasswordHash = ""
	return u
}

// PublicPtr returns a sanitised copy for JSON responses.
func (u *User) PublicPtr() *User {
	if u == nil {
		return nil
	}
	cp := *u
	cp.PasswordHash = ""
	return &cp
}

// Channel identifies where a conversation originates. It drives formatting,
// streaming capability and default personality.
type Channel string

const (
	ChannelWeb    Channel = "web"
	ChannelAPI    Channel = "api"
	ChannelVoice  Channel = "voice"
	ChannelMQTT   Channel = "mqtt"
	ChannelDevice Channel = "device"
	ChannelCLI    Channel = "cli"
)

// Session is a conversation thread.
type Session struct {
	ID            string         `json:"id"`
	UserID        string         `json:"user_id"`
	Title         string         `json:"title"`
	PersonalityID string         `json:"personality_id,omitempty"`
	AgentID       string         `json:"agent_id,omitempty"`
	Channel       Channel        `json:"channel"`
	Metadata      map[string]any `json:"metadata,omitempty"`
	MessageCount  int            `json:"message_count"`
	CreatedAt     time.Time      `json:"created_at"`
	UpdatedAt     time.Time      `json:"updated_at"`
}
