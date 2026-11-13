package httpapi

import (
	"net/http"

	"github.com/ArsinShaabani/AIOS/internal/domain"
	"github.com/ArsinShaabani/AIOS/internal/platform"
	"github.com/ArsinShaabani/AIOS/internal/ports"
)

func (a *API) handleListDevices(w http.ResponseWriter, r *http.Request) {
	user := userFrom(r.Context())
	filter := ports.DeviceFilter{
		Kind:   domain.DeviceKind(r.URL.Query().Get("kind")),
		Status: r.URL.Query().Get("status"),
		Room:   r.URL.Query().Get("room"),
		Query:  r.URL.Query().Get("q"),
	}
	page := platform.Page{Limit: queryInt(r, "limit", 50), Offset: queryInt(r, "offset", 0)}
	p := principalFrom(r.Context())
	var (
		items []domain.Device
		total int
		err   error
	)
	if r.URL.Query().Get("all") == "true" && p.Can(securityPermUsersRead) {
		items, total, err = a.Devices.ListAll(r.Context(), filter, page)
	} else {
		items, total, err = a.Devices.List(r.Context(), user.ID, filter, page)
	}
	if err != nil {
		a.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, platform.NewPageResult(items, total, page))
}

func (a *API) handleCreateDevice(w http.ResponseWriter, r *http.Request) {
	var d domain.Device
	if err := decodeJSON(w, r, &d); err != nil {
		a.fail(w, r, err)
		return
	}
	user := userFrom(r.Context())
	device, token, err := a.Devices.Register(r.Context(), user.ID, &d)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{
		"device": device, "token": token,
		"note": "store this token now; it is not retrievable later",
	})
}

func (a *API) handleGetDevice(w http.ResponseWriter, r *http.Request) {
	user := userFrom(r.Context())
	device, err := a.Devices.Get(r.Context(), user.ID, r.PathValue("id"))
	if err != nil {
		a.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, device)
}

func (a *API) handleUpdateDevice(w http.ResponseWriter, r *http.Request) {
	user := userFrom(r.Context())
	var patch domain.Device
	if err := decodeJSON(w, r, &patch); err != nil {
		a.fail(w, r, err)
		return
	}
	patch.ID = r.PathValue("id")
	if err := a.Devices.Update(r.Context(), user.ID, &patch); err != nil {
		a.fail(w, r, err)
		return
	}
	device, err := a.Devices.Get(r.Context(), user.ID, patch.ID)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, device)
}

func (a *API) handleDeleteDevice(w http.ResponseWriter, r *http.Request) {
	user := userFrom(r.Context())
	if err := a.Devices.Delete(r.Context(), user.ID, r.PathValue("id")); err != nil {
		a.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

type commandRequest struct {
	Capability string `json:"capability"`
	Value      any    `json:"value"`
}

func (a *API) handleDeviceCommand(w http.ResponseWriter, r *http.Request) {
	var req commandRequest
	if err := decodeJSON(w, r, &req); err != nil {
		a.fail(w, r, err)
		return
	}
	if req.Capability == "" {
		a.fail(w, r, platform.Invalid("device.capability", "capability is required"))
		return
	}
	user := userFrom(r.Context())
	result, device, err := a.Devices.Command(r.Context(), user.ID, r.PathValue("id"), req.Capability, req.Value)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"result": result, "device": device})
}

func (a *API) handleDeviceTelemetry(w http.ResponseWriter, r *http.Request) {
	user := userFrom(r.Context())
	state, device, err := a.Devices.Telemetry(r.Context(), user.ID, r.PathValue("id"))
	if err != nil {
		a.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"device": device, "telemetry": state})
}

func (a *API) handleRotateDeviceToken(w http.ResponseWriter, r *http.Request) {
	user := userFrom(r.Context())
	token, err := a.Devices.RotateToken(r.Context(), user.ID, r.PathValue("id"))
	if err != nil {
		a.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"token": token, "note": "the previous token no longer works"})
}

type telemetryRequest struct {
	Metric string         `json:"metric"`
	Value  any            `json:"value"`
	Unit   string         `json:"unit"`
	Meta   map[string]any `json:"meta"`
	At     int64          `json:"at"`
}

func (a *API) handleDeviceIngest(w http.ResponseWriter, r *http.Request) {
	var req telemetryRequest
	if err := decodeJSON(w, r, &req); err != nil {
		a.fail(w, r, err)
		return
	}
	if req.Metric == "" {
		a.fail(w, r, platform.Invalid("telemetry.metric", "metric is required"))
		return
	}
	device := deviceFrom(r.Context())
	a.Devices.HandleTelemetry(r.Context(), ports.Telemetry{
		DeviceID: device.ID, Metric: req.Metric, Value: req.Value, Unit: req.Unit, Meta: req.Meta, At: req.At,
	})
	w.WriteHeader(http.StatusAccepted)
}

func (a *API) handleDeviceHeartbeat(w http.ResponseWriter, r *http.Request) {
	device := deviceFrom(r.Context())
	a.Devices.Touch(r.Context(), device.ID)
	writeJSON(w, http.StatusOK, map[string]any{"status": "online", "device_id": device.ID})
}

func (a *API) handleDeviceMe(w http.ResponseWriter, r *http.Request) {
	device := deviceFrom(r.Context())
	writeJSON(w, http.StatusOK, device)
}
