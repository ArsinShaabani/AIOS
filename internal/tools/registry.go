// Package tools implements the built-in capabilities exposed to agents through
// the model's function-calling loop. Every tool is declared with a JSON schema,
// executes with a context deadline and reports structured errors.
package tools

import (
	"context"
	"encoding/json"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/ArsinShaabani/AIOS/internal/platform"
	"github.com/ArsinShaabani/AIOS/internal/ports"
)

// Registry is a thread-safe tool registry.
type Registry struct {
	mu    sync.RWMutex
	tools map[string]ports.Tool
}

var _ ports.ToolRegistry = (*Registry)(nil)

// NewRegistry builds an empty registry.
func NewRegistry() *Registry {
	return &Registry{tools: make(map[string]ports.Tool, 16)}
}

// Register validates and stores a tool.
func (r *Registry) Register(tool ports.Tool) error {
	if strings.TrimSpace(tool.Name) == "" {
		return platform.Invalid("tool.name", "tool name is required")
	}
	if tool.Executor == nil {
		return platform.Invalid("tool.executor", "tool executor is required")
	}
	if len(tool.Parameters) > 0 && !json.Valid(tool.Parameters) {
		return platform.Invalid("tool.schema", "tool parameters must be valid JSON: "+tool.Name)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.tools[tool.Name] = tool
	return nil
}

// Get resolves a tool by name.
func (r *Registry) Get(name string) (ports.Tool, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	t, ok := r.tools[name]
	return t, ok
}

// List returns the tools allowed by the given allow-list. An empty list means
// "no tools" (safer than accidentally exposing everything).
func (r *Registry) List(allowed []string) []ports.Tool {
	if len(allowed) == 0 {
		return nil
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]ports.Tool, 0, len(allowed))
	for _, name := range allowed {
		if t, ok := r.tools[name]; ok {
			out = append(out, t)
		}
	}
	return out
}

// Specs converts tools into provider tool definitions.
func (r *Registry) Specs(allowed []string) []ports.ToolSpec {
	list := r.List(allowed)
	specs := make([]ports.ToolSpec, 0, len(list))
	for _, t := range list {
		params := t.Parameters
		if len(params) == 0 {
			params = json.RawMessage(`{"type":"object","properties":{}}`)
		}
		specs = append(specs, ports.ToolSpec{Name: t.Name, Description: t.Description, Parameters: params})
	}
	return specs
}

// Names lists every registered tool name, sorted.
func (r *Registry) Names() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]string, 0, len(r.tools))
	for name := range r.tools {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

// Unregister removes a tool (plugin unload).
func (r *Registry) Unregister(name string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.tools, name)
}

// Execute runs a tool with a timeout, converting panics into tool errors.
func (r *Registry) Execute(ctx context.Context, name string, call ports.ToolCallContext) (ports.ToolResult, error) {
	t, ok := r.Get(name)
	if !ok {
		return ports.ToolResult{IsError: true, Content: "unknown tool: " + name}, platform.NotFound("tool.unknown", "unknown tool: "+name)
	}
	timeout := 20
	if t.Timeout > 0 {
		timeout = t.Timeout
	}
	cctx, cancel := context.WithTimeout(ctx, time.Duration(timeout)*time.Second)
	defer cancel()
	return t.Executor(cctx, call)
}

// Schema builds a JSON-schema object from property definitions.
func Schema(properties map[string]any, required ...string) json.RawMessage {
	obj := map[string]any{"type": "object", "properties": properties}
	if len(required) > 0 {
		obj["required"] = required
	}
	raw, _ := json.Marshal(obj)
	return raw
}
