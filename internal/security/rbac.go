package security

import (
	"github.com/ArsinShaabani/AIOS/internal/domain"
)

// Permission is a coarse-grained capability checked by transports.
type Permission string

const (
	PermChat          Permission = "chat"
	PermVoice         Permission = "voice"
	PermUsersRead     Permission = "users:read"
	PermUsersWrite    Permission = "users:write"
	PermDevicesRead   Permission = "devices:read"
	PermDevicesWrite  Permission = "devices:write"
	PermAgentsRead    Permission = "agents:read"
	PermAgentsWrite   Permission = "agents:write"
	PermPluginsRead   Permission = "plugins:read"
	PermPluginsWrite  Permission = "plugins:write"
	PermMemoryRead    Permission = "memory:read"
	PermMemoryWrite   Permission = "memory:write"
	PermLogsRead      Permission = "logs:read"
	PermModelsRead    Permission = "models:read"
	PermModelsWrite   Permission = "models:write"
	PermSettingsRead  Permission = "settings:read"
	PermSettingsWrite Permission = "settings:write"
	PermQueueRead     Permission = "queue:read"
	PermQueueWrite    Permission = "queue:write"
	PermAdmin         Permission = "admin"
)

var rolePermissions = map[domain.Role]map[Permission]struct{}{
	domain.RoleAdmin: setOf(
		PermChat, PermVoice, PermUsersRead, PermUsersWrite, PermDevicesRead, PermDevicesWrite,
		PermAgentsRead, PermAgentsWrite, PermPluginsRead, PermPluginsWrite, PermMemoryRead,
		PermMemoryWrite, PermLogsRead, PermModelsRead, PermModelsWrite, PermSettingsRead,
		PermSettingsWrite, PermQueueRead, PermQueueWrite, PermAdmin,
	),
	domain.RoleOperator: setOf(
		PermChat, PermVoice, PermUsersRead, PermDevicesRead, PermDevicesWrite, PermAgentsRead,
		PermAgentsWrite, PermPluginsRead, PermMemoryRead, PermMemoryWrite, PermLogsRead,
		PermModelsRead, PermQueueRead, PermQueueWrite,
	),
	domain.RoleUser: setOf(
		PermChat, PermVoice, PermDevicesRead, PermDevicesWrite, PermAgentsRead,
		PermPluginsRead, PermMemoryRead, PermMemoryWrite, PermModelsRead,
	),
	domain.RoleViewer: setOf(PermChat, PermDevicesRead, PermAgentsRead, PermPluginsRead, PermModelsRead),
	domain.RoleDevice: setOf(PermDevicesRead),
}

func setOf(perms ...Permission) map[Permission]struct{} {
	out := make(map[Permission]struct{}, len(perms))
	for _, p := range perms {
		out[p] = struct{}{}
	}
	return out
}

// Can reports whether a role holds a permission.
func Can(role domain.Role, perm Permission) bool {
	perms, ok := rolePermissions[role]
	if !ok {
		return false
	}
	if _, ok := perms[PermAdmin]; ok {
		return true
	}
	_, ok = perms[perm]
	return ok
}

// Permissions returns the sorted permission list for a role (UI use).
func Permissions(role domain.Role) []string {
	perms := rolePermissions[role]
	out := make([]string, 0, len(perms))
	for p := range perms {
		out = append(out, string(p))
	}
	return out
}
