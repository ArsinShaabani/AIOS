package plugins

import (
	"os"
	"testing"

	"github.com/ArsinShaabani/AIOS/internal/config"
	"github.com/ArsinShaabani/AIOS/internal/domain"
	"github.com/ArsinShaabani/AIOS/internal/tools"
)

func TestWASMModuleErrorBranches(t *testing.T) {
	repos := pluginRepos(t)
	registry := tools.NewRegistry()
	host := New(repos.Plugins, registry, config.PluginConfig{Enabled: true, AllowHTTP: true,
		AllowWASM: true, Dir: t.TempDir(), MaxWASMBytes: 1024, WASMTimeout: config.Duration(2e9)}, nil)

	// A stored record without a module path is rejected on reload.
	if err := repos.Plugins.Upsert(t.Context(), &domain.Plugin{ID: "no-path", Name: "no-path", Kind: "wasm",
		Enabled: true, Tools: []domain.PluginTool{{Name: "no_path_tool"}}}); err != nil {
		t.Fatal(err)
	}
	if err := host.Reload(t.Context()); err != nil {
		t.Fatal(err)
	}
	if _, ok := registry.Get("no_path_tool"); ok {
		t.Fatal("plugin without module path registered")
	}

	// Missing file.
	if err := repos.Plugins.Upsert(t.Context(), &domain.Plugin{ID: "missing-file", Name: "missing-file", Kind: "wasm",
		Endpoint: "/nonexistent/module.wasm", WasmSHA256: "abc", Enabled: true,
		Tools: []domain.PluginTool{{Name: "missing_tool"}}}); err != nil {
		t.Fatal(err)
	}
	_ = host.Reload(t.Context())
	if _, ok := registry.Get("missing_tool"); ok {
		t.Fatal("missing module registered")
	}

	// Stored module larger than the configured limit.
	dir := t.TempDir()
	path := dir + "/big.wasm"
	if err := os.WriteFile(path, make([]byte, 4096), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := repos.Plugins.Upsert(t.Context(), &domain.Plugin{ID: "big", Name: "big", Kind: "wasm",
		Endpoint: path, WasmSHA256: Digest(make([]byte, 4096)), Enabled: true,
		Tools: []domain.PluginTool{{Name: "big_tool"}}}); err != nil {
		t.Fatal(err)
	}
	_ = host.Reload(t.Context())
	if _, ok := registry.Get("big_tool"); ok {
		t.Fatal("oversized module registered")
	}

	// Uninstall of a WASM plugin whose file is already gone must not fail.
	if err := host.Uninstall(t.Context(), "missing-file"); err != nil {
		t.Fatalf("uninstall: %v", err)
	}
}
