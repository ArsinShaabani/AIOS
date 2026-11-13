package voice

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/ArsinShaabani/AIOS/internal/config"
	"github.com/ArsinShaabani/AIOS/internal/domain"
	"github.com/ArsinShaabani/AIOS/internal/platform"
	"github.com/ArsinShaabani/AIOS/internal/ports"
)

const profileSettingKey = "voice.profile"

// Profile is a per-user voice preference layered on top of the deployment
// defaults. Zero values mean "inherit".
type Profile struct {
	Voice    string  `json:"voice,omitempty"`
	Language string  `json:"language,omitempty"`
	Format   string  `json:"format,omitempty"`
	Speed    float64 `json:"speed,omitempty"`
}

// DefaultProfile returns the deployment defaults as a profile.
func DefaultProfile(cfg config.VoiceConfig) Profile {
	return Profile{Voice: cfg.TTS.Voice, Language: cfg.STT.Language, Format: cfg.TTS.Format, Speed: cfg.TTS.Speed}
}

// Normalize fills empty fields from the deployment defaults.
func (p Profile) Normalize(cfg config.VoiceConfig) Profile {
	d := DefaultProfile(cfg)
	if p.Voice == "" {
		p.Voice = d.Voice
	}
	if p.Language == "" {
		p.Language = d.Language
	}
	if p.Format == "" {
		p.Format = d.Format
	}
	if p.Speed == 0 {
		p.Speed = d.Speed
	}
	if p.Speed == 0 {
		p.Speed = 1
	}
	return p
}

// Validate rejects unsafe or unsupported values.
func (p Profile) Validate() error {
	if len(strings.TrimSpace(p.Voice)) > 80 {
		return platform.Invalid("voice.profile_voice", "voice name is too long")
	}
	if len(strings.TrimSpace(p.Language)) > 16 {
		return platform.Invalid("voice.profile_language", "language must be an ISO code (max 16 chars)")
	}
	switch strings.ToLower(strings.TrimSpace(p.Format)) {
	case "", "wav", "mp3", "opus":
	default:
		return platform.Invalid("voice.profile_format", "format must be wav, mp3 or opus")
	}
	if p.Speed != 0 && (p.Speed < 0.5 || p.Speed > 2) {
		return platform.Invalid("voice.profile_speed", "speed must be within [0.5, 2]")
	}
	return nil
}

// ProfileStore persists profiles in per-user settings (JSON, no schema change).
type ProfileStore struct {
	settings ports.SettingRepository
}

// NewProfileStore builds the store.
func NewProfileStore(settings ports.SettingRepository) *ProfileStore {
	return &ProfileStore{settings: settings}
}

// Get returns the stored profile or (nil, nil) when the user never customised it.
func (s *ProfileStore) Get(ctx context.Context, userID string) (*Profile, error) {
	if s == nil || s.settings == nil || userID == "" {
		return nil, nil
	}
	row, err := s.settings.Get(ctx, "user", userID, profileSettingKey)
	if err != nil {
		if platform.IsNotFound(err) {
			return nil, nil
		}
		return nil, err
	}
	var p Profile
	if err := json.Unmarshal(row.Value, &p); err != nil {
		// A corrupt preference must never take voice down.
		return nil, nil
	}
	return &p, nil
}

// Set stores a profile for the user.
func (s *ProfileStore) Set(ctx context.Context, userID string, p Profile) (*Profile, error) {
	if userID == "" {
		return nil, platform.Unauthorized("voice.profile_user", "authentication required")
	}
	if err := p.Validate(); err != nil {
		return nil, err
	}
	p.Format = strings.ToLower(strings.TrimSpace(p.Format))
	raw, err := json.Marshal(p)
	if err != nil {
		return nil, platform.Wrap(platform.KindInternal, "voice.profile_encode", "cannot encode profile", err)
	}
	if err := s.settings.Set(ctx, &domain.Setting{Scope: "user", ScopeID: userID,
		Key: profileSettingKey, Value: raw, UpdatedBy: userID}); err != nil {
		return nil, err
	}
	return &p, nil
}

// Delete resets the user's voice preferences.
func (s *ProfileStore) Delete(ctx context.Context, userID string) error {
	if userID == "" {
		return platform.Unauthorized("voice.profile_user", "authentication required")
	}
	return s.settings.Delete(ctx, "user", userID, profileSettingKey)
}
