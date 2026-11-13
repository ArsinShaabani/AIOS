package plugins

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ArsinShaabani/AIOS/internal/config"
	"github.com/ArsinShaabani/AIOS/internal/domain"
	"github.com/ArsinShaabani/AIOS/internal/platform"
	"github.com/ArsinShaabani/AIOS/internal/tools"
)

func TestHostValidationAndDisabled(t *testing.T) {
	repos := pluginRepos(t)
	registry := tools.NewRegistry()
	disabled := New(repos.Plugins, registry, config.PluginConfig{Enabled: false}, nil)
	if _, err := disabled.Install(context.Background(), &domain.Plugin{Name: "x"}); err == nil {
		t.Fatal("install while disabled accepted")
	}

	host := New(repos.Plugins, registry, config.PluginConfig{Enabled: true, AllowHTTP: true,
		AllowedHosts: []string{"allowed.test"}, Timeout: config.Duration(time.Second)}, nil)
	valid := func() *domain.Plugin {
		return &domain.Plugin{Name: "ok", Endpoint: "http://allowed.test/hook",
			Tools: []domain.PluginTool{{Name: "ok_tool", Description: "d"}}}
	}
	cases := []struct {
		name   string
		mutate func(*domain.Plugin)
	}{
		{"empty name", func(p *domain.Plugin) { p.Name = "" }},
		{"long name", func(p *domain.Plugin) { p.Name = strings.Repeat("x", 100) }},
		{"bad kind", func(p *domain.Plugin) { p.Kind = "wasm" }},
		{"bad endpoint", func(p *domain.Plugin) { p.Endpoint = "ftp://allowed.test/x" }},
		{"host not allowed", func(p *domain.Plugin) { p.Endpoint = "http://evil.test/x" }},
		{"no tools", func(p *domain.Plugin) { p.Tools = nil }},
		{"bad tool name", func(p *domain.Plugin) { p.Tools[0].Name = "Bad Name" }},
		{"bad method", func(p *domain.Plugin) { p.Tools[0].Method = http.MethodDelete }},
		{"bad schema", func(p *domain.Plugin) { p.Tools[0].Parameters = json.RawMessage("{nope") }},
		{"bad permission", func(p *domain.Plugin) { p.Permissions = []string{"admin"} }},
		{"unknown permission", func(p *domain.Plugin) { p.Permissions = []string{"root"} }},
	}
	for _, c := range cases {
		p := valid()
		c.mutate(p)
		if _, err := host.Install(context.Background(), p); err == nil {
			t.Errorf("%s: accepted", c.name)
		}
	}
	p := valid()
	p.Enabled = false
	if _, err := host.Install(context.Background(), p); err != nil {
		t.Fatalf("valid plugin rejected: %v", err)
	}
}

func TestLoadManifestDir(t *testing.T) {
	dir := t.TempDir()
	manifest := map[string]any{
		"name": "disk", "version": "1.0.0", "endpoint": "http://127.0.0.1:9",
		"permissions": []string{"internet"},
		"tools":       []map[string]any{{"name": "disk_tool", "description": "x"}},
	}
	raw, _ := json.Marshal(manifest)
	if err := os.WriteFile(filepath.Join(dir, "disk.json"), raw, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "broken.json"), []byte("{nope"), 0o600); err != nil {
		t.Fatal(err)
	}
	count := 0
	n, err := LoadManifestDir(context.Background(), dir, func(p *domain.Plugin) error {
		count++
		if p.Name != "disk" || !p.Enabled {
			return platform.Invalid("bad", "unexpected manifest")
		}
		return nil
	})
	if err != nil || n != 1 || count != 1 {
		t.Fatalf("load dir: %v n=%d count=%d", err, n, count)
	}
	if n, err := LoadManifestDir(context.Background(), filepath.Join(dir, "missing"), func(*domain.Plugin) error { return nil }); err != nil || n != 0 {
		t.Fatalf("missing dir: %v %d", err, n)
	}
}
