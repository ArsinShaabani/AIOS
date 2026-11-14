package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	llmadapter "github.com/ArsinShaabani/AIOS/internal/adapters/llm"
	"github.com/ArsinShaabani/AIOS/internal/config"
	"github.com/ArsinShaabani/AIOS/internal/ports"
)

// runLLMTest diagnoses the model layer: it builds every configured provider,
// pings each one, then sends a real completion to the default route. It prints
// the provider that actually answered, so a silent fallback to the template
// model cannot hide a broken backend.
func runLLMTest(args []string) {
	fs := flag.NewFlagSet("llm", flag.ExitOnError)
	path := fs.String("config", os.Getenv("AIOS_CONFIG"), "path to config.yaml")
	prompt := fs.String("prompt", "Say hello in one short sentence.", "test prompt")
	timeout := fs.Duration("timeout", 120*time.Second, "per-request timeout")
	// Support both `aios llm test` and `aios llm`.
	if len(args) > 0 && (args[0] == "test" || args[0] == "diagnose") {
		args = args[1:]
	}
	if err := fs.Parse(args); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	if *path == "" {
		if _, err := os.Stat("config.yaml"); err == nil {
			*path = "config.yaml"
		}
	}
	cfg, err := config.Load(*path)
	if err != nil {
		fmt.Fprintln(os.Stderr, "config:", err)
		os.Exit(1)
	}
	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()

	fmt.Printf("config:           %s\n", *path)
	fmt.Printf("default provider: %s\n", cfg.LLM.DefaultProvider)
	fmt.Printf("default model:    %s\n\n", cfg.LLM.DefaultModel)

	providers, errs := llmadapter.BuildAll(cfg.LLM.Providers)
	for _, e := range errs {
		fmt.Printf("  ✗ skipped: %v\n", e)
	}

	names := make([]string, 0, len(providers))
	for name := range providers {
		names = append(names, name)
	}
	sort.Strings(names)

	failed := false
	for _, name := range names {
		p := providers[name]
		caps := p.Capabilities()
		fmt.Printf("provider %s (models: %s)\n", name, strings.Join(p.Models(), ", "))
		if caps.Embeddings {
			fmt.Printf("  embeddings: yes\n")
		}
		pingErr := pingProvider(ctx, p)
		if pingErr != nil {
			fmt.Printf("  ✗ ping failed: %v\n", pingErr)
			failed = true
		} else {
			fmt.Printf("  ✓ ping ok\n")
		}
		started := time.Now()
		resp, chatErr := p.Chat(ctx, ports.ChatRequest{
			Model:       firstModel(cfg, name, p),
			Messages:    []ports.ChatMessage{{Role: "user", Content: *prompt}},
			MaxTokens:   64,
			Temperature: 0.1,
		})
		if chatErr != nil {
			fmt.Printf("  ✗ chat failed: %v\n\n", chatErr)
			failed = true
			continue
		}
		elapsed := time.Since(started)
		content := strings.TrimSpace(resp.Content)
		if len(content) > 120 {
			content = content[:120] + "…"
		}
		speed := ""
		if out := resp.Usage.CompletionTokens; out > 0 && elapsed > 0 {
			speed = fmt.Sprintf(" (%.1f tok/s, %d tokens in %.1fs)", float64(out)/elapsed.Seconds(), out, elapsed.Seconds())
		} else if elapsed > 0 {
			speed = fmt.Sprintf(" (%.1fs)", elapsed.Seconds())
		}
		fmt.Printf("  ✓ chat ok%s: %q\n\n", speed, content)
	}

	if failed {
		fmt.Fprintln(os.Stderr, "one or more providers failed — see the errors above")
		fmt.Fprintln(os.Stderr, "hints: ollama serve / systemctl status ollama; ollama list; check base_url and the model name")
		os.Exit(1)
	}
	fmt.Println("model layer OK")
}

func firstModel(cfg config.Config, name string, p ports.Provider) string {
	for _, pc := range cfg.LLM.Providers {
		if pc.Name != name {
			continue
		}
		if pc.DefaultModel != "" {
			return pc.DefaultModel
		}
	}
	if models := p.Models(); len(models) > 0 {
		return models[0]
	}
	return cfg.LLM.DefaultModel
}

func pingProvider(ctx context.Context, p ports.Provider) error {
	type pinger interface {
		Ping(context.Context) error
	}
	if pp, ok := p.(pinger); ok {
		return pp.Ping(ctx)
	}
	return nil
}
