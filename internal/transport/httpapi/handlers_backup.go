package httpapi

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/ArsinShaabani/AIOS/internal/backup"
)

func (a *API) handleBackup(w http.ResponseWriter, r *http.Request) {
	includeData := r.URL.Query().Get("data") == "1"
	doc, err := backup.New(a.Repos.DB, a.Version).Export(r.Context(), includeData)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	name := "aios-backup-" + time.Now().UTC().Format("20060102-150405") + ".json"
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Disposition", `attachment; filename="`+name+`"`)
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	_ = enc.Encode(doc)
}

func (a *API) handleRestore(w http.ResponseWriter, r *http.Request) {
	var doc backup.Document
	if err := decodeJSON(w, r, &doc); err != nil {
		a.fail(w, r, err)
		return
	}
	if err := backup.New(a.Repos.DB, a.Version).Restore(r.Context(), &doc); err != nil {
		a.fail(w, r, err)
		return
	}
	user := userFrom(r.Context())
	a.Auditor.Log(r.Context(), user.ID, "user", "backup.restore", "backup", "", "success", clientIP(r), r.UserAgent(), nil)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "restored_at": time.Now().UTC(),
		"tables": len(doc.Tables)})
}
