package plugins

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/ArsinShaabani/AIOS/internal/adapters/storage/sqlstore"
	"github.com/ArsinShaabani/AIOS/internal/config"
	"github.com/ArsinShaabani/AIOS/internal/domain"
	"github.com/ArsinShaabani/AIOS/internal/ports"
	"github.com/ArsinShaabani/AIOS/internal/tools"
)

func pluginRepos(t *testing.T) *sqlstore.Repositories {
	t.Helper()
	db, err := sqlstore.Open(context.Background(), config.DatabaseConfig{
		Driver: "sqlite", DSN: "plugins.db", AutoMigrate: true, MaxOpenConns: 1,
	}, t.TempDir(), nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return sqlstore.NewRepositories(db)
}

func TestHostInstallExecuteAndUninstall(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/tools/ok":
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{"content":"done","data":{"n":2}}`)
		case "/tools/text":
			fmt.Fprint(w, "plain answer")
		case "/tools/fail":
			w.WriteHeader(http.StatusInternalServerError)
			fmt.Fprint(w, "boom")
		case "/tools/get":
			if r.URL.Query().Get("city") != "Tehran" {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			fmt.Fprint(w, `{"content":"GET ok"}`)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	repos := pluginRepos(t)
	registry := tools.NewRegistry()
	cfg := config.PluginConfig{Enabled: true, AllowHTTP: true, Timeout: config.Duration(2 * time.Second), MaxResponseBytes: 1 << 20}
	host := New(repos.Plugins, registry, cfg, nil)
	plugin := &domain.Plugin{
		Name: "unit", Version: "1.0.0", Endpoint: srv.URL, Enabled: true,
		Permissions: []string{"internet"},
		Tools: []domain.PluginTool{
			{Name: "unit_ok", Description: "ok", Method: http.MethodPost, Path: "/tools/ok"},
			{Name: "unit_text", Description: "text", Method: http.MethodPost, Path: "/tools/text"},
			{Name: "unit_fail", Description: "fail", Method: http.MethodPost, Path: "/tools/fail"},
			{Name: "unit_get", Description: "get", Method: http.MethodGet, Path: "/tools/get"},
		},
	}
	installed, err := host.Install(context.Background(), plugin)
	if err != nil {
		t.Fatal(err)
	}
	if installed.ID == "" || len(installed.Manifest) == 0 {
		t.Fatalf("install: %+v", installed)
	}
	tool, ok := registry.Get("unit_ok")
	if !ok {
		t.Fatal("tool not registered")
	}
	res, err := tool.Executor(context.Background(), ports.ToolCallContext{UserID: "u1", Arguments: map[string]any{}})
	if err != nil || res.Content != "done" || res.Data["n"].(float64) != 2 {
		t.Fatalf("execute: %v %+v", err, res)
	}
	textTool, _ := registry.Get("unit_text")
	if res, _ := textTool.Executor(context.Background(), ports.ToolCallContext{}); res.Content != "plain answer" {
		t.Fatalf("text response: %+v", res)
	}
	failTool, _ := registry.Get("unit_fail")
	if res, _ := failTool.Executor(context.Background(), ports.ToolCallContext{}); !res.IsError {
		t.Fatalf("failure not surfaced: %+v", res)
	}
	getTool, _ := registry.Get("unit_get")
	if res, _ := getTool.Executor(context.Background(), ports.ToolCallContext{Arguments: map[string]any{"city": "Tehran"}}); res.Content != "GET ok" {
		t.Fatalf("GET tool: %+v", res)
	}
	if list, err := host.List(context.Background()); err != nil || len(list) != 1 {
		t.Fatalf("list: %v %v", err, list)
	}
	if err := host.SetEnabled(context.Background(), installed.ID, false); err != nil {
		t.Fatal(err)
	}
	if _, ok := registry.Get("unit_ok"); ok {
		t.Fatal("tool still registered after disable")
	}
	if err := host.SetEnabled(context.Background(), installed.ID, true); err != nil {
		t.Fatal(err)
	}
	if _, ok := registry.Get("unit_ok"); !ok {
		t.Fatal("tool not restored after enable")
	}
	if err := host.Uninstall(context.Background(), installed.ID); err != nil {
		t.Fatal(err)
	}
	if _, ok := registry.Get("unit_ok"); ok {
		t.Fatal("tool still registered after uninstall")
	}
}
