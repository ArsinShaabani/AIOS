package sqlstore

import (
	"context"
	"testing"
	"time"

	"github.com/ArsinShaabani/AIOS/internal/domain"
	"github.com/ArsinShaabani/AIOS/internal/platform"
	"github.com/ArsinShaabani/AIOS/internal/ports"
)

func TestStatsOverviewAndSettings(t *testing.T) {
	ctx := context.Background()
	r := newTestStore(t)
	u := &domain.User{ID: "u1", Email: "a@b.c", PasswordHash: "x", Role: domain.RoleAdmin}
	if err := r.Users.Create(ctx, u); err != nil {
		t.Fatal(err)
	}
	s := &domain.Session{ID: "s1", UserID: "u1", Channel: domain.ChannelAPI}
	if err := r.Sessions.Create(ctx, s); err != nil {
		t.Fatal(err)
	}
	if err := r.Messages.Create(ctx, &domain.Message{ID: "m1", SessionID: "s1", Role: domain.MsgUser, Content: "hi"}); err != nil {
		t.Fatal(err)
	}
	if err := r.Devices.Create(ctx, &domain.Device{ID: "d1", UserID: "u1", Name: "x"}); err != nil {
		t.Fatal(err)
	}
	if err := r.Tasks.Create(ctx, &domain.Task{ID: "t1", Kind: "reminder", RunAt: time.Now().Add(time.Hour)}); err != nil {
		t.Fatal(err)
	}
	overview, err := r.Stats.Overview(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if overview.Users != 1 || overview.Sessions != 1 || overview.Messages != 1 || overview.Devices != 1 || overview.PendingTasks != 1 {
		t.Fatalf("overview: %+v", overview)
	}
	if err := r.Settings.Delete(ctx, "global", "", "missing"); err != nil {
		t.Fatalf("delete missing setting should be a no-op: %v", err)
	}
	if list, err := r.Settings.List(ctx, "global", ""); err != nil || len(list) != 0 {
		t.Fatalf("settings list: %v %+v", err, list)
	}
}

func TestUserAndSessionFilters(t *testing.T) {
	ctx := context.Background()
	r := newTestStore(t)
	for i, role := range []domain.Role{domain.RoleAdmin, domain.RoleUser, domain.RoleViewer} {
		u := &domain.User{ID: "u" + string(rune('1'+i)), Email: "u" + string(rune('1'+i)) + "@x.c",
			PasswordHash: "x", Role: role, DisplayName: "User"}
		if err := r.Users.Create(ctx, u); err != nil {
			t.Fatal(err)
		}
	}
	if list, total, err := r.Users.List(ctx, platform.Page{Limit: 2}, ports.UserFilter{Role: domain.RoleUser}); err != nil || total != 1 || len(list) != 1 {
		t.Fatalf("role filter: %v %d", err, total)
	}
	if list, total, err := r.Users.List(ctx, platform.Page{Limit: 10}, ports.UserFilter{Query: "u2"}); err != nil || total != 1 || len(list) != 1 {
		t.Fatalf("query filter: %v %d", err, total)
	}
	if _, total, err := r.Users.List(ctx, platform.Page{Limit: 10}, ports.UserFilter{Status: "active"}); err != nil || total != 3 {
		t.Fatalf("status filter: %v %d", err, total)
	}
	if n, err := r.Users.Count(ctx); err != nil || n != 3 {
		t.Fatalf("user count: %v %d", err, n)
	}
	s := &domain.Session{ID: "s1", UserID: "u1", Channel: domain.ChannelWeb}
	if err := r.Sessions.Create(ctx, s); err != nil {
		t.Fatal(err)
	}
	if list, total, err := r.Sessions.ListByUser(ctx, "u1", platform.Page{Limit: 5}); err != nil || total != 1 || len(list) != 1 {
		t.Fatalf("session list: %v %d", err, total)
	}
	if n, _ := r.Sessions.Count(ctx); n != 1 {
		t.Fatalf("session count: %d", n)
	}
	if n, _ := r.Messages.CountMessages(ctx); n != 0 {
		t.Fatalf("message count: %d", n)
	}
}
