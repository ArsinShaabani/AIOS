package app

import (
	"context"
	"time"

	"github.com/ArsinShaabani/AIOS/internal/domain"
	"github.com/ArsinShaabani/AIOS/internal/platform"
	"github.com/ArsinShaabani/AIOS/internal/security"
)

// Seed creates the built-in personalities, agents and the first administrator.
// It is idempotent and safe to run on every boot.
func (a *App) Seed(ctx context.Context) error {
	if err := a.Personality.Seed(ctx); err != nil {
		return err
	}
	if err := a.Agents.Seed(ctx); err != nil {
		return err
	}
	count, err := a.Repos.Users.Count(ctx)
	if err != nil {
		return err
	}
	if count > 0 {
		return nil
	}
	email := domain.NormalizeEmail(a.Config.Security.Bootstrap.Email)
	if email == "" {
		email = "admin@aios.local"
	}
	password := a.Config.Security.Bootstrap.Password
	generated := false
	if password == "" {
		password = platform.MustRandomToken(18)
		generated = true
	}
	hash, err := security.HashPassword(password, a.Config.Security.BcryptCost)
	if err != nil {
		return err
	}
	admin := &domain.User{
		ID: platform.NewID("usr"), Email: email, DisplayName: a.Config.Security.Bootstrap.Name,
		PasswordHash: hash, Role: domain.RoleAdmin, Status: "active",
		PersonalityID: "per_balanced", Locale: "en", Timezone: a.Config.Scheduler.Timezone,
		CreatedAt: time.Now().UTC(),
	}
	if admin.DisplayName == "" {
		admin.DisplayName = "Administrator"
	}
	if err := a.Repos.Users.Create(ctx, admin); err != nil {
		return err
	}
	if generated {
		a.Log.Warn("bootstrap administrator created with a generated password — change it after first login",
			"email", email, "password", password)
	} else {
		a.Log.Info("bootstrap administrator created", "email", email)
	}
	a.Auditor.Record(ctx, domain.AuditEvent{
		ActorType: "system", Action: "bootstrap.admin_created", Resource: "user",
		ResourceID: admin.ID, Status: "success", CreatedAt: time.Now().UTC(),
	})
	return nil
}
