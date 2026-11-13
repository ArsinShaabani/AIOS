package main

import (
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/ArsinShaabani/AIOS/internal/domain"
	"github.com/ArsinShaabani/AIOS/internal/plugins"
)

// runPluginKeygen implements `aios plugin-keygen`.
func runPluginKeygen(args []string) {
	fs := flag.NewFlagSet("plugin-keygen", flag.ExitOnError)
	out := fs.String("out", "", "directory to write plugin-signing.key and plugin-signing.pub")
	_ = fs.Parse(args)
	priv, pub, err := plugins.GenerateKeyPair()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if *out == "" {
		fmt.Printf("private_key: %s\npublic_key:  %s\n", priv, pub)
		return
	}
	if err := os.MkdirAll(*out, 0o700); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	keyPath := filepath.Join(*out, "plugin-signing.key")
	pubPath := filepath.Join(*out, "plugin-signing.pub")
	if err := os.WriteFile(keyPath, []byte(priv+"\n"), 0o600); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if err := os.WriteFile(pubPath, []byte(pub+"\n"), 0o644); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Printf("wrote %s (0600) and %s\n", keyPath, pubPath)
	fmt.Printf("trusted_keys config value:\n  - %s\n", pub)
}

// runPluginSign implements `aios plugin-sign`: it canonicalises a manifest plus
// the optional WASM module, signs it, and writes a marketplace-ready manifest.
func runPluginSign(args []string) {
	fs := flag.NewFlagSet("plugin-sign", flag.ExitOnError)
	key := fs.String("key", "", "base64 Ed25519 private key or path to plugin-signing.key")
	manifestPath := fs.String("manifest", "", "plugin manifest JSON (name, version, kind, tools, permissions)")
	wasmPath := fs.String("wasm", "", "optional WASM module to sign and hash")
	out := fs.String("out", "", "output path for the signed manifest (default stdout)")
	_ = fs.Parse(args)
	if *manifestPath == "" {
		fmt.Fprintln(os.Stderr, "plugin-sign: -manifest is required")
		os.Exit(2)
	}
	keyValue := strings.TrimSpace(*key)
	if keyValue == "" {
		fmt.Fprintln(os.Stderr, "plugin-sign: -key is required")
		os.Exit(2)
	}
	if data, err := os.ReadFile(keyValue); err == nil {
		keyValue = strings.TrimSpace(string(data))
	}
	priv, err := plugins.DecodePrivateKey(keyValue)
	if err != nil {
		fmt.Fprintln(os.Stderr, "plugin-sign:", err)
		os.Exit(1)
	}
	raw, err := os.ReadFile(*manifestPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "plugin-sign:", err)
		os.Exit(1)
	}
	var p domain.Plugin
	if err := json.Unmarshal(raw, &p); err != nil {
		fmt.Fprintln(os.Stderr, "plugin-sign: invalid manifest:", err)
		os.Exit(1)
	}
	if p.Kind == "" {
		p.Kind = "http"
	}
	var wasm []byte
	if *wasmPath != "" {
		wasm, err = os.ReadFile(*wasmPath)
		if err != nil {
			fmt.Fprintln(os.Stderr, "plugin-sign:", err)
			os.Exit(1)
		}
		p.WasmSHA256 = plugins.Digest(wasm)
		p.WasmSize = int64(len(wasm))
	}
	sig, err := plugins.SignPlugin(priv, &p, wasm)
	if err != nil {
		fmt.Fprintln(os.Stderr, "plugin-sign:", err)
		os.Exit(1)
	}
	p.Signature = sig
	signed, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	signed = append(signed, '\n')
	if *out == "" {
		_, _ = os.Stdout.Write(signed)
		return
	}
	if err := os.WriteFile(*out, signed, 0o644); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Printf("signed manifest written to %s\n", *out)
	if len(wasm) > 0 {
		encoded := base64.StdEncoding.EncodeToString(wasm)
		fmt.Printf("install with: POST /api/v1/plugins and add \"wasm_module\": \"%s…\" (%d bytes base64)\n",
			encoded[:min(32, len(encoded))], len(encoded))
	}
}
