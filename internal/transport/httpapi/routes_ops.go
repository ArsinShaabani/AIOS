package httpapi

import (
	"net/http"

	"github.com/ArsinShaabani/AIOS/internal/security"
)

const (
	securityPermAgentsWrite   = security.PermAgentsWrite
	securityPermDevicesWrite  = security.PermDevicesWrite
	securityPermPluginsRead   = security.PermPluginsRead
	securityPermPluginsWrite  = security.PermPluginsWrite
	securityPermSettingsWrite = security.PermSettingsWrite
	securityPermModelsRead    = security.PermModelsRead
	securityPermUsersRead     = security.PermUsersRead
	securityPermUsersWrite    = security.PermUsersWrite
	securityPermLogsRead      = security.PermLogsRead
	securityPermQueueRead     = security.PermQueueRead
	securityPermQueueWrite    = security.PermQueueWrite
	securityPermMemoryRead    = security.PermMemoryRead
	securityPermMemoryWrite   = security.PermMemoryWrite
	securityPermAdmin         = security.PermAdmin
)

func (a *API) registerOpsRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/plugins", a.perm(securityPermPluginsRead, a.handleListPlugins))
	mux.HandleFunc("POST /api/v1/plugins", a.perm(securityPermPluginsWrite, a.handleInstallPlugin))
	mux.HandleFunc("PATCH /api/v1/plugins/{id}", a.perm(securityPermPluginsWrite, a.handleTogglePlugin))
	mux.HandleFunc("DELETE /api/v1/plugins/{id}", a.perm(securityPermPluginsWrite, a.handleDeletePlugin))
	mux.HandleFunc("GET /api/v1/settings", a.authed(a.handleListSettings))
	mux.HandleFunc("PUT /api/v1/settings/{key}", a.perm(securityPermSettingsWrite, a.handleSetSetting))
	mux.HandleFunc("DELETE /api/v1/settings/{key}", a.perm(securityPermSettingsWrite, a.handleDeleteSetting))
	mux.HandleFunc("GET /api/v1/models", a.perm(securityPermModelsRead, a.handleListModels))
	mux.HandleFunc("GET /api/v1/usage", a.authed(a.handleUsage))
	mux.HandleFunc("GET /api/v1/usage/daily", a.authed(a.handleUsageDaily))
	mux.HandleFunc("GET /api/v1/usage/providers", a.perm(securityPermUsersRead, a.handleUsageProviders))
	mux.HandleFunc("GET /api/v1/voice/config", a.authed(a.handleVoiceConfig))
	mux.HandleFunc("GET /api/v1/voice/profile", a.authed(a.handleGetVoiceProfile))
	mux.HandleFunc("PUT /api/v1/voice/profile", a.authed(a.handleSetVoiceProfile))
	mux.HandleFunc("DELETE /api/v1/voice/profile", a.authed(a.handleDeleteVoiceProfile))
	mux.HandleFunc("POST /api/v1/voice/transcribe", a.authed(a.handleTranscribe))
	mux.HandleFunc("POST /api/v1/voice/synthesize", a.authed(a.handleSynthesize))
	mux.HandleFunc("GET /api/v1/tasks", a.perm(securityPermQueueRead, a.handleListTasks))
	mux.HandleFunc("DELETE /api/v1/tasks/{id}", a.perm(securityPermQueueWrite, a.handleCancelTask))
}

func (a *API) registerAdminRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/admin/overview", a.perm(securityPermUsersRead, a.handleOverview))
	mux.HandleFunc("GET /api/v1/admin/users", a.perm(securityPermUsersRead, a.handleListUsers))
	mux.HandleFunc("POST /api/v1/admin/users", a.perm(securityPermUsersWrite, a.handleCreateUser))
	mux.HandleFunc("PATCH /api/v1/admin/users/{id}", a.perm(securityPermUsersWrite, a.handleUpdateUser))
	mux.HandleFunc("DELETE /api/v1/admin/users/{id}", a.perm(securityPermUsersWrite, a.handleDeleteUser))
	mux.HandleFunc("POST /api/v1/admin/users/{id}/reset-password", a.perm(securityPermUsersWrite, a.handleResetPassword))
	mux.HandleFunc("POST /api/v1/admin/users/{id}/revoke-tokens", a.perm(securityPermUsersWrite, a.handleRevokeUserTokens))
	mux.HandleFunc("GET /api/v1/admin/audit", a.perm(securityPermLogsRead, a.handleAudit))
	mux.HandleFunc("GET /api/v1/admin/system", a.perm(securityPermUsersRead, a.handleSystem))
	mux.HandleFunc("GET /api/v1/admin/queue", a.perm(securityPermQueueRead, a.handleQueue))
	mux.HandleFunc("GET /api/v1/admin/prompt/{sessionID}", a.perm(securityPermUsersRead, a.handlePromptPreview))
	mux.HandleFunc("GET /api/v1/admin/memories", a.perm(securityPermMemoryRead, a.handleAdminMemories))
	mux.HandleFunc("POST /api/v1/admin/maintenance/prune", a.perm(securityPermAdmin, a.handlePrune))
}

func (a *API) registerRealtimeRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/ws/chat", a.handleWSChat)
	mux.HandleFunc("GET /api/v1/ws/voice", a.handleWSVoice)
}
