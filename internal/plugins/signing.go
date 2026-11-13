package plugins

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"sort"

	"github.com/ArsinShaabani/AIOS/internal/domain"
	"github.com/ArsinShaabani/AIOS/internal/platform"
)

// signable is the canonical, signed view of a plugin manifest. Only fields that
// affect behaviour are included; cosmetic metadata and IDs are not, so the same
// artifact can be re-installed under a different local ID.
type signable struct {
	Name        string              `json:"name"`
	Version     string              `json:"version"`
	Description string              `json:"description"`
	Author      string              `json:"author,omitempty"`
	Homepage    string              `json:"homepage,omitempty"`
	Kind        string              `json:"kind"`
	Endpoint    string              `json:"endpoint,omitempty"`
	Permissions []string            `json:"permissions"`
	Tools       []domain.PluginTool `json:"tools"`
	WasmSHA256  string              `json:"wasm_sha256,omitempty"`
}

// Digest returns the hex SHA-256 of the module bytes.
func Digest(wasm []byte) string {
	sum := sha256.Sum256(wasm)
	return hex.EncodeToString(sum[:])
}

// CanonicalBytes builds the exact byte sequence that is signed. It is stable
// across JSON encoders: sorted permissions, declared tool order, and a NUL
// separator before the raw module digest.
func CanonicalBytes(p *domain.Plugin, wasm []byte) ([]byte, error) {
	perms := make([]string, len(p.Permissions))
	copy(perms, p.Permissions)
	sort.Strings(perms)
	tools := make([]domain.PluginTool, len(p.Tools))
	copy(tools, p.Tools)
	for i := range tools {
		// Normalise defaults exactly like Host.validate does, otherwise the
		// signature would depend on whether validation ran first.
		if tools[i].Method == "" {
			tools[i].Method = "POST"
		}
	}
	kind := p.Kind
	if kind == "" {
		kind = "http"
	}
	version := p.Version
	if version == "" {
		version = "0.1.0"
	}
	digest := p.WasmSHA256
	if len(wasm) > 0 {
		digest = Digest(wasm)
	}
	// The endpoint is part of an HTTP plugin's behaviour and must be signed. For
	// WASM the host assigns a local file path, so it is intentionally excluded.
	endpoint := ""
	if kind == "http" {
		endpoint = p.Endpoint
	}
	raw, err := json.Marshal(signable{
		Name: p.Name, Version: version, Description: p.Description,
		Author: p.Author, Homepage: p.Homepage, Kind: kind, Endpoint: endpoint,
		Permissions: perms, Tools: tools, WasmSHA256: digest,
	})
	if err != nil {
		return nil, platform.Wrap(platform.KindInvalid, "plugin.canonical", "cannot encode manifest", err)
	}
	out := append(raw, 0)
	out = append(out, []byte(digest)...)
	return out, nil
}

// SignPlugin signs a manifest and its optional WASM module with Ed25519.
func SignPlugin(priv ed25519.PrivateKey, p *domain.Plugin, wasm []byte) (string, error) {
	if len(priv) != ed25519.PrivateKeySize {
		return "", platform.Invalid("plugin.key", "invalid Ed25519 private key")
	}
	payload, err := CanonicalBytes(p, wasm)
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(ed25519.Sign(priv, payload)), nil
}

// VerifyPlugin checks a signature against any of the trusted public keys.
func VerifyPlugin(p *domain.Plugin, wasm []byte, keys []ed25519.PublicKey) bool {
	if p.Signature == "" || len(keys) == 0 {
		return false
	}
	sig, err := base64.StdEncoding.DecodeString(p.Signature)
	if err != nil || len(sig) != ed25519.SignatureSize {
		return false
	}
	payload, err := CanonicalBytes(p, wasm)
	if err != nil {
		return false
	}
	for _, key := range keys {
		if len(key) == ed25519.PublicKeySize && ed25519.Verify(key, payload, sig) {
			return true
		}
	}
	return false
}

// GenerateKeyPair returns base64-encoded Ed25519 keys for operators.
func GenerateKeyPair() (privateB64, publicB64 string, err error) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return "", "", platform.Wrap(platform.KindInternal, "plugin.keygen", "cannot generate key", err)
	}
	return base64.StdEncoding.EncodeToString(priv), base64.StdEncoding.EncodeToString(pub), nil
}

// DecodePrivateKey parses a base64 Ed25519 private key (64 bytes).
func DecodePrivateKey(s string) (ed25519.PrivateKey, error) {
	raw, err := base64.StdEncoding.DecodeString(s)
	if err != nil || len(raw) != ed25519.PrivateKeySize {
		return nil, platform.Invalid("plugin.key", "invalid base64 Ed25519 private key")
	}
	return ed25519.PrivateKey(raw), nil
}

// DecodePublicKey parses a base64 Ed25519 public key (32 bytes).
func DecodePublicKey(s string) (ed25519.PublicKey, error) {
	raw, err := base64.StdEncoding.DecodeString(s)
	if err != nil || len(raw) != ed25519.PublicKeySize {
		return nil, platform.Invalid("plugin.key", "invalid base64 Ed25519 public key")
	}
	return ed25519.PublicKey(raw), nil
}

// TrustedKeys parses the configured public keys, skipping malformed entries.
func TrustedKeys(encoded []string) []ed25519.PublicKey {
	out := make([]ed25519.PublicKey, 0, len(encoded))
	for _, s := range encoded {
		if key, err := DecodePublicKey(s); err == nil {
			out = append(out, key)
		}
	}
	return out
}
