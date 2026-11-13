package plugins

import (
	"os"
	"testing"
	"time"

	"github.com/ArsinShaabani/AIOS/internal/config"
	"github.com/ArsinShaabani/AIOS/internal/domain"
)

func TestWASMDiskTamperDetected(t *testing.T) {
	privB64, pubB64, _ := GenerateKeyPair()
	priv, _ := DecodePrivateKey(privB64)
	code := buildEchoGuest(t)
	host := wasmTestHost(t, func(c *config.PluginConfig) {
		c.RequireSignature = true
		c.TrustedKeys = []string{pubB64}
	})
	p := &domain.Plugin{Name: "wasm-echo", Kind: "wasm", Enabled: true,
		Tools: []domain.PluginTool{{Name: "disk_echo", Description: "e"}}}
	sig, _ := SignPlugin(priv, p, code)
	p.Signature = sig
	installed, err := host.InstallModule(t.Context(), p, code)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := host.registry.Get("disk_echo"); !ok {
		t.Fatal("tool not registered")
	}
	// Tamper with the stored module; Reload must refuse to register it.
	if err := os.WriteFile(installed.Endpoint, append(code, 0x00), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := host.Reload(t.Context()); err != nil {
		t.Fatal(err)
	}
	if _, ok := host.registry.Get("disk_echo"); ok {
		t.Fatal("tampered module was loaded")
	}
	// Missing module is also rejected rather than crashing the server.
	if err := os.Remove(installed.Endpoint); err != nil {
		t.Fatal(err)
	}
	if err := host.Reload(t.Context()); err != nil {
		t.Fatal(err)
	}
}

func TestWASMTimeout(t *testing.T) {
	code := buildEchoGuest(t)
	host := wasmTestHost(t, func(c *config.PluginConfig) {
		c.WASMTimeout = config.Duration(50 * time.Millisecond)
	})
	p := &domain.Plugin{Name: "slow", Kind: "wasm", Enabled: true,
		Tools: []domain.PluginTool{{Name: "slow_tool", Description: "s"}}}
	installed, err := host.InstallModule(t.Context(), p, code)
	if err != nil {
		t.Fatal(err)
	}
	if !installed.Enabled {
		t.Fatal("plugin not enabled")
	}
	// Corrupt the module after install to force a compile/runtime failure path.
	if err := os.WriteFile(installed.Endpoint, []byte("invalid wasm"), 0o600); err != nil {
		t.Fatal(err)
	}
	_ = host.Reload(t.Context())
	if _, ok := host.registry.Get("slow_tool"); ok {
		t.Fatal("invalid module registered")
	}
}
