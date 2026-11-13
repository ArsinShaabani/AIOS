package plugins

import (
	"crypto/ed25519"
	"testing"

	"github.com/ArsinShaabani/AIOS/internal/domain"
)

func TestSigningCanonicalizationAndRejection(t *testing.T) {
	privB64, pubB64, err := GenerateKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	priv, _ := DecodePrivateKey(privB64)
	pub, _ := DecodePublicKey(pubB64)
	if _, err := DecodePrivateKey("not-a-key"); err == nil {
		t.Fatal("invalid private key accepted")
	}
	if _, err := DecodePublicKey("not-a-key"); err == nil {
		t.Fatal("invalid public key accepted")
	}
	keys := TrustedKeys([]string{"bad", pubB64})
	if len(keys) != 1 {
		t.Fatalf("trusted keys: %d", len(keys))
	}

	p := &domain.Plugin{Name: "sign-test", Version: "1.0.0", Kind: "http", Endpoint: "http://x",
		Permissions: []string{"internet", "chat"},
		Tools:       []domain.PluginTool{{Name: "t", Description: "d"}}}
	sig, err := SignPlugin(priv, p, nil)
	if err != nil {
		t.Fatal(err)
	}
	p.Signature = sig
	if !VerifyPlugin(p, nil, keys) {
		t.Fatal("valid signature rejected")
	}
	// Permission order is canonicalised.
	reordered := *p
	reordered.Permissions = []string{"chat", "internet"}
	if !VerifyPlugin(&reordered, nil, keys) {
		t.Fatal("permission ordering should not break the signature")
	}
	// Any semantic change must invalidate it.
	for name, mutate := range map[string]func(*domain.Plugin){
		"name":        func(x *domain.Plugin) { x.Name = "evil" },
		"version":     func(x *domain.Plugin) { x.Version = "2.0.0" },
		"permissions": func(x *domain.Plugin) { x.Permissions = []string{"devices:write"} },
		"tools":       func(x *domain.Plugin) { x.Tools[0].Description = "changed" },
		"endpoint":    func(x *domain.Plugin) { x.Endpoint = "http://evil" },
	} {
		bad := *p
		bad.Permissions = append([]string(nil), p.Permissions...)
		bad.Tools = append([]domain.PluginTool(nil), p.Tools...)
		mutate(&bad)
		if VerifyPlugin(&bad, nil, keys) {
			t.Fatalf("%s change did not invalidate the signature", name)
		}
	}
	if VerifyPlugin(p, []byte("different module"), keys) {
		t.Fatal("module change did not invalidate the signature")
	}
	if VerifyPlugin(&domain.Plugin{Name: "x", Signature: "bad"}, nil, keys) {
		t.Fatal("malformed signature accepted")
	}
	if VerifyPlugin(p, nil, nil) {
		t.Fatal("signature accepted without trusted keys")
	}
	p.Signature = sig
	if VerifyPlugin(p, nil, []ed25519.PublicKey{}) {
		t.Fatal("empty key list accepted")
	}
	if string(keys[0]) != string(pub) {
		t.Fatal("decoded key mismatch")
	}
}
