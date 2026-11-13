package httpapi

import (
	"context"
	"io"
	"net/http"
	"strings"

	"github.com/ArsinShaabani/AIOS/internal/domain"
	"github.com/ArsinShaabani/AIOS/internal/platform"
	"github.com/ArsinShaabani/AIOS/internal/ports"
	"github.com/ArsinShaabani/AIOS/internal/voice"
)

func (a *API) handleVoiceConfig(w http.ResponseWriter, r *http.Request) {
	cfg := a.Config.Voice
	writeJSON(w, http.StatusOK, map[string]any{
		"enabled": cfg.Enabled, "sample_rate": cfg.SampleRate,
		"stt": cfg.STT.Provider, "tts": cfg.TTS.Provider,
		"voice": cfg.TTS.Voice, "format": cfg.TTS.Format,
		"wake_word": cfg.WakeWord, "vad": cfg.VAD,
		"interruptions": cfg.Interruptions, "max_session": cfg.MaxSession,
	})
}

func (a *API) voiceProfile(ctx context.Context, userID string) (voice.Profile, bool) {
	defaults := voice.DefaultProfile(a.Config.Voice)
	if a.VoiceProfiles == nil {
		return defaults, false
	}
	stored, err := a.VoiceProfiles.Get(ctx, userID)
	if err != nil || stored == nil {
		return defaults, false
	}
	return stored.Normalize(a.Config.Voice), true
}

func (a *API) handleGetVoiceProfile(w http.ResponseWriter, r *http.Request) {
	user := userFrom(r.Context())
	profile, customised := a.voiceProfile(r.Context(), user.ID)
	writeJSON(w, http.StatusOK, map[string]any{"profile": profile, "customised": customised})
}

func (a *API) handleSetVoiceProfile(w http.ResponseWriter, r *http.Request) {
	var p voice.Profile
	if err := decodeJSON(w, r, &p); err != nil {
		a.fail(w, r, err)
		return
	}
	if err := p.Validate(); err != nil {
		a.fail(w, r, err)
		return
	}
	user := userFrom(r.Context())
	stored, err := a.VoiceProfiles.Set(r.Context(), user.ID, p)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	a.Auditor.Log(r.Context(), user.ID, "user", "voice.profile_update", "user", user.ID, "success",
		clientIP(r), r.UserAgent(), map[string]any{"voice": stored.Voice, "language": stored.Language, "speed": stored.Speed})
	writeJSON(w, http.StatusOK, map[string]any{"profile": stored.Normalize(a.Config.Voice), "customised": true})
}

func (a *API) handleDeleteVoiceProfile(w http.ResponseWriter, r *http.Request) {
	user := userFrom(r.Context())
	if err := a.VoiceProfiles.Delete(r.Context(), user.ID); err != nil {
		a.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *API) handleTranscribe(w http.ResponseWriter, r *http.Request) {
	if !a.Voice.Enabled() {
		a.fail(w, r, platform.Unavailable("voice.disabled", "voice is disabled"))
		return
	}
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		a.fail(w, r, platform.Invalid("voice.form", "expected multipart/form-data with an audio file"))
		return
	}
	file, header, err := r.FormFile("audio")
	if err != nil {
		a.fail(w, r, platform.Invalid("voice.file", "audio file is required"))
		return
	}
	defer file.Close()
	if header.Size > 32<<20 {
		a.fail(w, r, platform.Invalid("voice.size", "audio must be under 32 MB"))
		return
	}
	raw, err := io.ReadAll(io.LimitReader(file, 32<<20))
	if err != nil {
		a.fail(w, r, err)
		return
	}
	audio := ports.Audio{Data: raw, Format: "wav", SampleRate: a.Config.Voice.SampleRate}
	if samples, rate, err := voice.DecodeWAV(raw); err == nil {
		if rate != a.Config.Voice.SampleRate {
			samples = voice.Resample(samples, rate, a.Config.Voice.SampleRate)
		}
		audio.Data = voice.EncodeWAV(samples, a.Config.Voice.SampleRate)
		audio.DurationMS = voice.DurationMS(len(samples), a.Config.Voice.SampleRate)
	}
	profile, _ := a.voiceProfile(r.Context(), userFrom(r.Context()).ID)
	tr, err := a.Voice.TranscribeLanguage(r.Context(), audio, profile.Language)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, tr)
}

type synthesizeRequest struct {
	Text   string `json:"text"`
	Voice  string `json:"voice"`
	Format string `json:"format"`
}

func (a *API) handleSynthesize(w http.ResponseWriter, r *http.Request) {
	if !a.Voice.Enabled() {
		a.fail(w, r, platform.Unavailable("voice.disabled", "voice is disabled"))
		return
	}
	var req synthesizeRequest
	if err := decodeJSON(w, r, &req); err != nil {
		a.fail(w, r, err)
		return
	}
	if strings.TrimSpace(req.Text) == "" {
		a.fail(w, r, platform.Invalid("voice.text", "text is required"))
		return
	}
	if len([]rune(req.Text)) > 4000 {
		a.fail(w, r, platform.Invalid("voice.text_long", "text must be under 4000 characters"))
		return
	}
	profile, _ := a.voiceProfile(r.Context(), userFrom(r.Context()).ID)
	if req.Voice == "" {
		req.Voice = profile.Voice
	}
	if req.Format == "" {
		req.Format = profile.Format
	}
	audio, err := a.Voice.SynthesizeOptions(r.Context(), req.Text, ports.VoiceOptions{
		Voice: req.Voice, Format: req.Format, Speed: profile.Speed,
	})
	if err != nil {
		a.fail(w, r, err)
		return
	}
	contentType := "audio/wav"
	switch audio.Format {
	case "mp3":
		contentType = "audio/mpeg"
	case "opus":
		contentType = "audio/ogg"
	}
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Length", itoa(len(audio.Data)))
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(audio.Data)
}

func (a *API) handleListTasks(w http.ResponseWriter, r *http.Request) {
	status := r.URL.Query().Get("status")
	page := platform.Page{Limit: queryInt(r, "limit", 50), Offset: queryInt(r, "offset", 0)}
	items, total, err := a.Scheduler.Tasks(r.Context(), status, page)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, platform.NewPageResult(items, total, page))
}

func (a *API) handleCancelTask(w http.ResponseWriter, r *http.Request) {
	user := userFrom(r.Context())
	p := principalFrom(r.Context())
	privileged := p.Role == domain.RoleAdmin || p.Role == domain.RoleOperator
	if err := a.Scheduler.Cancel(r.Context(), user.ID, r.PathValue("id"), privileged); err != nil {
		a.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}
