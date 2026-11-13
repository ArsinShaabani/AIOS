package plugins

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/ArsinShaabani/AIOS/internal/config"
	"github.com/ArsinShaabani/AIOS/internal/domain"
	"github.com/ArsinShaabani/AIOS/internal/ports"
	"github.com/ArsinShaabani/AIOS/internal/tools"
)

func buildEchoGuest(t *testing.T) []byte {
	t.Helper()
	_, file, _, _ := runtime.Caller(0)
	root := filepath.Join(filepath.Dir(file), "..", "..")
	out := filepath.Join(t.TempDir(), "echo.wasm")
	cmd := exec.Command("go", "build", "-trimpath", "-o", out, "./examples/plugins/wasm-echo")
	cmd.Dir = root
	cmd.Env = append(os.Environ(), "GOOS=wasip1", "GOARCH=wasm", "CGO_ENABLED=0")
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build guest: %v\n%s", err, output)
	}
	code, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	return code
}

func wasmTestHost(t *testing.T, mutate func(*config.PluginConfig)) *Host {
	t.Helper()
	repos := pluginRepos(t)
	registry := tools.NewRegistry()
	cfg := config.PluginConfig{
		Enabled: true, AllowHTTP: true, AllowWASM: true, Dir: t.TempDir(),
		Timeout: config.Duration(2e9), MaxResponseBytes: 1 << 20,
		MaxWASMBytes: 16 << 20, WASMTimeout: config.Duration(30e9), WASMMemoryPages: 256,
	}
	if mutate != nil {
		mutate(&cfg)
	}
	return New(repos.Plugins, registry, cfg, nil)
}

func TestWASMPluginEndToEnd(t *testing.T) {
	privB64, pubB64, err := GenerateKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	priv, err := DecodePrivateKey(privB64)
	if err != nil {
		t.Fatal(err)
	}
	code := buildEchoGuest(t)
	host := wasmTestHost(t, func(c *config.PluginConfig) {
		c.RequireSignature = true
		c.TrustedKeys = []string{pubB64}
	})
	registry := host.registry
	p := &domain.Plugin{
		Name: "wasm-echo", Version: "1.0.0", Kind: "wasm", Enabled: true,
		Tools: []domain.PluginTool{
			{Name: "echo", Description: "echo text", Parameters: []byte(`{"type":"object","properties":{"text":{"type":"string"}}}`)},
		},
	}
	sig, err := SignPlugin(priv, p, code)
	if err != nil {
		t.Fatal(err)
	}
	p.Signature = sig
	installed, err := host.InstallModule(t.Context(), p, code)
	if err != nil {
		t.Fatalf("install wasm: %v", err)
	}
	if !installed.Verified || installed.WasmSHA256 == "" || installed.WasmSize == 0 {
		t.Fatalf("install metadata: %+v", installed)
	}
	if _, err := os.Stat(installed.Endpoint); err != nil {
		t.Fatalf("module not stored: %v", err)
	}
	tool, ok := registry.Get("echo")
	if !ok {
		t.Fatal("wasm tool not registered")
	}
	res, err := tool.Executor(t.Context(), ports.ToolCallContext{UserID: "u1", Arguments: map[string]any{"text": "hello"}})
	if err != nil || res.IsError || res.Content != "echo: hello" {
		t.Fatalf("execute wasm: %v %+v", err, res)
	}
	if res.Data["length"].(float64) != 5 {
		t.Fatalf("guest data: %+v", res.Data)
	}
	// Unknown tool is handled by the guest, not the host.
	res, _ = tool.Executor(t.Context(), ports.ToolCallContext{Arguments: map[string]any{"text": ""}})
	if !res.IsError {
		t.Fatalf("empty argument should produce a tool error: %+v", res)
	}

	if err := host.Uninstall(t.Context(), installed.ID); err != nil {
		t.Fatal(err)
	}
	if _, ok := registry.Get("echo"); ok {
		t.Fatal("tool still registered after uninstall")
	}
	if _, err := os.Stat(installed.Endpoint); !os.IsNotExist(err) {
		t.Fatal("wasm module not removed")
	}
}

func TestWASMSignatureAndTamperRejection(t *testing.T) {
	privB64, pubB64, _ := GenerateKeyPair()
	priv, _ := DecodePrivateKey(privB64)
	otherPrivB64, _, _ := GenerateKeyPair()
	otherPriv, _ := DecodePrivateKey(otherPrivB64)
	code := buildEchoGuest(t)
	host := wasmTestHost(t, func(c *config.PluginConfig) {
		c.RequireSignature = true
		c.TrustedKeys = []string{pubB64}
	})
	base := func() *domain.Plugin {
		return &domain.Plugin{Name: "wasm-echo", Kind: "wasm", Enabled: false,
			Tools: []domain.PluginTool{{Name: "echo", Description: "e"}}}
	}
	// Unsigned.
	if _, err := host.InstallModule(t.Context(), base(), code); err == nil {
		t.Fatal("unsigned wasm accepted")
	}
	// Signed by an untrusted key.
	p := base()
	sig, _ := SignPlugin(otherPriv, p, code)
	p.Signature = sig
	if _, err := host.InstallModule(t.Context(), p, code); err == nil {
		t.Fatal("untrusted signature accepted")
	}
	// Signature for different bytes.
	p = base()
	sig, _ = SignPlugin(priv, p, code)
	p.Signature = sig
	tampered := append([]byte(nil), code...)
	tampered[len(tampered)-1] ^= 0xff
	if _, err := host.InstallModule(t.Context(), p, tampered); err == nil {
		t.Fatal("tampered module accepted")
	}
	// Valid signature.
	p = base()
	sig, _ = SignPlugin(priv, p, code)
	p.Signature = sig
	if _, err := host.InstallModule(t.Context(), p, code); err != nil {
		t.Fatalf("valid signature rejected: %v", err)
	}
}

func TestWASMDisabledAndSizeLimits(t *testing.T) {
	host := wasmTestHost(t, func(c *config.PluginConfig) { c.AllowWASM = false })
	p := &domain.Plugin{Name: "w", Kind: "wasm", Tools: []domain.PluginTool{{Name: "t"}}}
	if _, err := host.InstallModule(t.Context(), p, []byte("x")); err == nil {
		t.Fatal("wasm accepted while disabled")
	}
	host2 := wasmTestHost(t, func(c *config.PluginConfig) { c.MaxWASMBytes = 8 })
	if _, err := host2.InstallModule(t.Context(), p, make([]byte, 32)); err == nil {
		t.Fatal("oversized module accepted")
	}
	host3 := wasmTestHost(t, nil)
	if _, err := host3.InstallModule(t.Context(), p, nil); err == nil {
		t.Fatal("missing module accepted")
	}
	// Invalid wasm bytes pass validation but fail at load/compile.
	if _, err := host3.InstallModule(t.Context(), p, []byte("not wasm")); err != nil {
		t.Fatalf("storing invalid bytes should be allowed for later verification: %v", err)
	}
}
