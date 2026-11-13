package voice

import (
	"context"
	"log/slog"
	"testing"

	"github.com/ArsinShaabani/AIOS/internal/adapters/storage/sqlstore"
	"github.com/ArsinShaabani/AIOS/internal/config"
	"github.com/ArsinShaabani/AIOS/internal/domain"
)

func profileStore(t *testing.T) (*ProfileStore, *sqlstore.Repositories) {
	t.Helper()
	db, err := sqlstore.Open(context.Background(), config.DatabaseConfig{
		Driver: "sqlite", DSN: "voice.db", AutoMigrate: true, MaxOpenConns: 1,
	}, t.TempDir(), slog.Default())
	if err != nil {
		t.Fatal(err)
	}
	repos := sqlstore.NewRepositories(db)
	t.Cleanup(func() { _ = db.Close() })
	return NewProfileStore(repos.Settings), repos
}

func TestProfileStore(t *testing.T) {
	store, _ := profileStore(t)
	ctx := context.Background()
	if p, err := store.Get(ctx, "u1"); err != nil || p != nil {
		t.Fatalf("absent profile: %v %+v", err, p)
	}
	if _, err := store.Set(ctx, "", Profile{Voice: "x"}); err == nil {
		t.Fatal("profile without user accepted")
	}
	for _, bad := range []Profile{
		{Voice: string(make([]byte, 100))},
		{Format: "flac"},
		{Speed: 4},
		{Speed: 0.1},
		{Language: "this-is-not-a-language-code"},
	} {
		if err := bad.Validate(); err == nil {
			t.Fatalf("invalid profile accepted: %+v", bad)
		}
	}
	saved, err := store.Set(ctx, "u1", Profile{Voice: "fa_IR-amir", Language: "fa", Format: "WAV", Speed: 1.25})
	if err != nil {
		t.Fatal(err)
	}
	if saved.Format != "wav" {
		t.Fatalf("format not normalised: %+v", saved)
	}
	got, err := store.Get(ctx, "u1")
	if err != nil || got == nil || got.Voice != "fa_IR-amir" || got.Speed != 1.25 {
		t.Fatalf("get: %v %+v", err, got)
	}
	cfg := config.VoiceConfig{TTS: config.TTSConfig{Voice: "default", Format: "wav", Speed: 1},
		STT: config.STTConfig{Language: "en"}}
	full := got.Normalize(cfg)
	if full.Language != "fa" || full.Voice != "fa_IR-amir" {
		t.Fatalf("normalise: %+v", full)
	}
	empty := Profile{}
	if e := empty.Normalize(cfg); e.Voice != "default" || e.Language != "en" || e.Speed != 1 {
		t.Fatalf("defaults: %+v", e)
	}
	if err := store.Delete(ctx, "u1"); err != nil {
		t.Fatal(err)
	}
	if p, _ := store.Get(ctx, "u1"); p != nil {
		t.Fatal("profile not deleted")
	}
	// A malformed stored value is ignored rather than breaking voice.
	_ = store.settings.Set(ctx, &domain.Setting{Scope: "user", ScopeID: "u1", Key: profileSettingKey,
		Value: []byte("{not-json")})
	if p, err := store.Get(ctx, "u1"); err != nil || p != nil {
		t.Fatalf("corrupt profile: %v %+v", err, p)
	}
}
