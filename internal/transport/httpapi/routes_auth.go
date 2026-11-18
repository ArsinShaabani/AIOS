package httpapi

import (
	"net/http"
)

func (a *API) registerAuthRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/v1/auth/login", a.handleLogin)
	mux.HandleFunc("POST /api/v1/auth/register", a.handleRegister)
	mux.HandleFunc("POST /api/v1/auth/refresh", a.handleRefresh)
	mux.HandleFunc("POST /api/v1/auth/logout", a.authed(a.handleLogout))
	mux.HandleFunc("GET /api/v1/auth/me", a.authed(a.handleMe))
	mux.HandleFunc("GET /api/v1/auth/api-keys", a.authed(a.handleListAPIKeys))
	mux.HandleFunc("POST /api/v1/auth/api-keys", a.authed(a.handleCreateAPIKey))
	mux.HandleFunc("DELETE /api/v1/auth/api-keys/{id}", a.authed(a.handleRevokeAPIKey))
}

func (a *API) registerChatRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/v1/chat", a.authed(a.handleChat))
	mux.HandleFunc("GET /api/v1/sessions", a.authed(a.handleListSessions))
	mux.HandleFunc("GET /api/v1/sessions/{id}", a.authed(a.handleGetSession))
	mux.HandleFunc("PATCH /api/v1/sessions/{id}", a.authed(a.handlePatchSession))
	mux.HandleFunc("DELETE /api/v1/sessions/{id}", a.authed(a.handleDeleteSession))
	mux.HandleFunc("GET /api/v1/sessions/{id}/messages", a.authed(a.handleSessionMessages))
}

func (a *API) registerDataRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/agents", a.authed(a.handleListAgents))
	mux.HandleFunc("GET /api/v1/agents/catalog", a.authed(a.handleAgentCatalog))
	mux.HandleFunc("POST /api/v1/agents/install", a.perm(securityPermAgentsWrite, a.handleInstallAgent))
	mux.HandleFunc("POST /api/v1/agents", a.perm(securityPermAgentsWrite, a.handleUpsertAgent))
	mux.HandleFunc("PUT /api/v1/agents/{id}", a.perm(securityPermAgentsWrite, a.handleUpsertAgent))
	mux.HandleFunc("DELETE /api/v1/agents/{id}", a.perm(securityPermAgentsWrite, a.handleDeleteAgent))
	mux.HandleFunc("GET /api/v1/personalities", a.authed(a.handleListPersonalities))
	mux.HandleFunc("POST /api/v1/personalities", a.authed(a.handleCreatePersonality))
	mux.HandleFunc("PUT /api/v1/personalities/{id}", a.authed(a.handleUpdatePersonality))
	mux.HandleFunc("DELETE /api/v1/personalities/{id}", a.authed(a.handleDeletePersonality))

	mux.HandleFunc("GET /api/v1/memories", a.authed(a.handleListMemories))
	mux.HandleFunc("POST /api/v1/memories", a.perm(securityPermMemoryWrite, a.handleCreateMemory))
	mux.HandleFunc("DELETE /api/v1/memories/{id}", a.perm(securityPermMemoryWrite, a.handleDeleteMemory))
	mux.HandleFunc("DELETE /api/v1/memories", a.perm(securityPermMemoryWrite, a.handleWipeMemories))

	// Document RAG.
	mux.HandleFunc("GET /api/v1/documents", a.perm(securityPermMemoryRead, a.handleListDocuments))
	mux.HandleFunc("POST /api/v1/documents", a.perm(securityPermMemoryWrite, a.handleIngestDocument))
	mux.HandleFunc("GET /api/v1/documents/search", a.perm(securityPermMemoryRead, a.handleSearchDocuments))
	mux.HandleFunc("GET /api/v1/documents/{id}", a.perm(securityPermMemoryRead, a.handleGetDocument))
	mux.HandleFunc("GET /api/v1/documents/{id}/chunks", a.perm(securityPermMemoryRead, a.handleDocumentChunks))
	mux.HandleFunc("DELETE /api/v1/documents/{id}", a.perm(securityPermMemoryWrite, a.handleDeleteDocument))

	mux.HandleFunc("GET /api/v1/devices", a.authed(a.handleListDevices))
	mux.HandleFunc("POST /api/v1/devices", a.perm(securityPermDevicesWrite, a.handleCreateDevice))
	mux.HandleFunc("GET /api/v1/devices/{id}", a.authed(a.handleGetDevice))
	mux.HandleFunc("PATCH /api/v1/devices/{id}", a.perm(securityPermDevicesWrite, a.handleUpdateDevice))
	mux.HandleFunc("DELETE /api/v1/devices/{id}", a.perm(securityPermDevicesWrite, a.handleDeleteDevice))
	mux.HandleFunc("POST /api/v1/devices/{id}/command", a.perm(securityPermDevicesWrite, a.handleDeviceCommand))
	mux.HandleFunc("GET /api/v1/devices/{id}/telemetry", a.authed(a.handleDeviceTelemetry))
	mux.HandleFunc("POST /api/v1/devices/{id}/rotate-token", a.perm(securityPermDevicesWrite, a.handleRotateDeviceToken))
	mux.HandleFunc("POST /api/v1/device/telemetry", a.deviceAuthed(a.handleDeviceIngest))
	mux.HandleFunc("POST /api/v1/device/heartbeat", a.deviceAuthed(a.handleDeviceHeartbeat))
	mux.HandleFunc("GET /api/v1/device/me", a.deviceAuthed(a.handleDeviceMe))
}

func (a *API) registerUserAIRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/me/providers", a.authed(a.handleListUserProviders))
	mux.HandleFunc("PUT /api/v1/me/providers/{name}", a.authed(a.handleUpsertUserProvider))
	mux.HandleFunc("DELETE /api/v1/me/providers/{name}", a.authed(a.handleDeleteUserProvider))
	mux.HandleFunc("POST /api/v1/me/providers/discover", a.authed(a.handleDiscoverUserProvider))
	mux.HandleFunc("POST /api/v1/me/providers/{name}/test", a.authed(a.handleTestUserProvider))
	mux.HandleFunc("GET /api/v1/me/models", a.authed(a.handleUserModels))

	// OpenAI-compatible tunnel for external apps and agents.
	mux.HandleFunc("POST /v1/chat/completions", a.handleTunnelChat)
	mux.HandleFunc("GET /v1/models", a.handleTunnelModels)
}
