package plugins

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ArsinShaabani/AIOS/internal/config"
	"github.com/ArsinShaabani/AIOS/internal/domain"
	"github.com/ArsinShaabani/AIOS/internal/ports"
	"github.com/ArsinShaabani/AIOS/internal/tools"
)

func TestSignedHTTPPlugin(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"content":"signed ok"}`)
	}))
	defer srv.Close()
	privB64, pubB64, _ := GenerateKeyPair()
	otherB64, _, _ := GenerateKeyPair()
	priv, _ := DecodePrivateKey(privB64)
	other, _ := DecodePrivateKey(otherB64)

	host := New(pluginRepos(t).Plugins, tools.NewRegistry(),
		config.PluginConfig{Enabled: true, AllowHTTP: true, Dir: t.TempDir(),
			RequireSignature: true, TrustedKeys: []string{pubB64}, Timeout: config.Duration(2e9)}, nil)

	p := &domain.Plugin{Name: "signed-http", Version: "1.0.0", Kind: "http",
		Endpoint: srv.URL, Enabled: true,
		Tools: []domain.PluginTool{{Name: "signed_tool", Description: "x", Path: "/tool"}}}
	sig, err := SignPlugin(priv, p, nil)
	if err != nil {
		t.Fatal(err)
	}
	p.Signature = sig
	installed, err := host.Install(t.Context(), p)
	if err != nil {
		t.Fatalf("signed install: %v", err)
	}
	if !installed.Verified {
		t.Fatal("signed plugin not marked verified")
	}
	tool, ok := host.registry.Get("signed_tool")
	if !ok {
		t.Fatal("tool not registered")
	}
	res, err := tool.Executor(t.Context(), ports.ToolCallContext{})
	if err != nil || res.Content != "signed ok" {
		t.Fatalf("tool: %v %+v", err, res)
	}

	// Endpoint tampering invalidates the signature.
	tampered := *p
	tampered.ID = ""
	tampered.Signature = sig
	tampered.Endpoint = srv.URL + "/evil"
	if _, err := host.Install(t.Context(), &tampered); err == nil {
		t.Fatal("tampered endpoint accepted")
	}
	// Untrusted key.
	bad := *p
	bad.ID = ""
	bad.Signature, _ = SignPlugin(other, &bad, nil)
	if _, err := host.Install(t.Context(), &bad); err == nil {
		t.Fatal("untrusted signature accepted")
	}
	// Signature without configured keys is rejected.
	open := New(pluginRepos(t).Plugins, tools.NewRegistry(),
		config.PluginConfig{Enabled: true, AllowHTTP: true, Dir: t.TempDir()}, nil)
	unsigned := *p
	unsigned.ID = ""
	if _, err := open.Install(t.Context(), &unsigned); err == nil {
		t.Fatal("signature without trusted keys accepted")
	}
	// But a plain unsigned plugin works when signatures are not required.
	unsigned.Signature = ""
	if _, err := open.Install(t.Context(), &unsigned); err != nil {
		t.Fatalf("unsigned plugin rejected without require_signature: %v", err)
	}
}
