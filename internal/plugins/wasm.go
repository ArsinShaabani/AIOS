package plugins

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/imports/wasi_snapshot_preview1"

	"github.com/ArsinShaabani/AIOS/internal/domain"
	"github.com/ArsinShaabani/AIOS/internal/platform"
	"github.com/ArsinShaabani/AIOS/internal/ports"
)

// wasmRequest is the JSON document written to the guest's stdin. The guest is a
// WASI command; it reads one JSON object and writes one JSON object to stdout.
type wasmRequest struct {
	Tool      string         `json:"tool"`
	Arguments map[string]any `json:"arguments"`
	UserID    string         `json:"user_id"`
	SessionID string         `json:"session_id"`
	AgentID   string         `json:"agent_id"`
	Plugin    string         `json:"plugin"`
}

type wasmResponse struct {
	Content string         `json:"content"`
	Data    map[string]any `json:"data,omitempty"`
	IsError bool           `json:"is_error,omitempty"`
}

// wasmExecutor runs a tool inside a fresh WASI sandbox: no filesystem pre-opens,
// no environment, no sockets, bounded memory and a hard deadline.
func (h *Host) wasmExecutor(p *domain.Plugin, t domain.PluginTool, code []byte) ports.ToolExecutor {
	return func(ctx context.Context, call ports.ToolCallContext) (ports.ToolResult, error) {
		req, err := json.Marshal(wasmRequest{
			Tool: t.Name, Arguments: call.Arguments, UserID: call.UserID,
			SessionID: call.SessionID, AgentID: call.AgentID, Plugin: p.Name,
		})
		if err != nil {
			return ports.ToolResult{IsError: true, Content: "cannot encode WASM request"}, nil
		}
		out, stderr, err := h.runWASM(ctx, code, req)
		if err != nil {
			return ports.ToolResult{IsError: true, Content: "WASM plugin failed: " + err.Error()}, nil
		}
		var resp wasmResponse
		if err := json.Unmarshal(out, &resp); err != nil {
			// A plain-text stdout is a valid, simplified response.
			text := strings.TrimSpace(string(out))
			if text == "" {
				return ports.ToolResult{IsError: true, Content: "WASM plugin returned no output: " + platform.Truncate(stderr, 300)}, nil
			}
			return ports.ToolResult{Content: platform.Truncate(text, int(h.maxResponseBytes()))}, nil
		}
		return ports.ToolResult{Content: platform.Truncate(resp.Content, int(h.maxResponseBytes())), Data: resp.Data, IsError: resp.IsError}, nil
	}
}

func (h *Host) runWASM(ctx context.Context, code, input []byte) ([]byte, string, error) {
	timeout := h.cfg.WASMTimeout.Std()
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	pages := h.cfg.WASMMemoryPages
	if pages == 0 {
		pages = 256
	}
	runtimeCfg := wazero.NewRuntimeConfig().
		WithMemoryLimitPages(pages).
		WithCloseOnContextDone(true)
	rt := wazero.NewRuntimeWithConfig(runCtx, runtimeCfg)
	defer func() { _ = rt.Close(context.Background()) }()

	if _, err := wasi_snapshot_preview1.Instantiate(runCtx, rt); err != nil {
		return nil, "", fmt.Errorf("cannot initialise WASI: %w", err)
	}
	compiled, err := rt.CompileModule(runCtx, code)
	if err != nil {
		return nil, "", fmt.Errorf("invalid WASM module: %w", err)
	}
	stdout := &boundedBuffer{limit: h.maxResponseBytes()}
	stderr := &boundedBuffer{limit: 8 << 10}
	moduleCfg := wazero.NewModuleConfig().
		WithName("plugin").
		WithStdin(bytes.NewReader(input)).
		WithStdout(stdout).
		WithStderr(stderr)
	_, err = rt.InstantiateModule(runCtx, compiled, moduleCfg)
	if err != nil {
		if errors.Is(runCtx.Err(), context.DeadlineExceeded) {
			return nil, stderr.String(), platform.Wrap(platform.KindTimeout, "plugin.wasm_timeout", "WASM plugin timed out", err)
		}
		if stdout.overflow || stderr.overflow {
			return nil, stderr.String(), platform.Invalid("plugin.wasm_output", "WASM plugin exceeded the output limit")
		}
		return nil, stderr.String(), platform.Wrap(platform.KindUnavailable, "plugin.wasm_run", "WASM plugin exited with an error: "+platform.Truncate(stderr.String(), 300), err)
	}
	if stdout.overflow {
		return nil, stderr.String(), platform.Invalid("plugin.wasm_output", "WASM plugin exceeded the output limit")
	}
	return stdout.Bytes(), stderr.String(), nil
}

func (h *Host) maxResponseBytes() int64 {
	if h.cfg.MaxResponseBytes > 0 {
		return h.cfg.MaxResponseBytes
	}
	return 512 << 10
}

// boundedBuffer writes up to limit bytes and records overflow instead of
// allowing a hostile guest to exhaust host memory.
type boundedBuffer struct {
	buf      bytes.Buffer
	limit    int64
	overflow bool
}

func (b *boundedBuffer) Write(p []byte) (int, error) {
	remaining := b.limit - int64(b.buf.Len())
	if remaining <= 0 {
		b.overflow = true
		return len(p), nil
	}
	if int64(len(p)) > remaining {
		b.overflow = true
		p = p[:remaining]
	}
	b.buf.Write(p)
	return len(p), nil
}

func (b *boundedBuffer) Bytes() []byte  { return b.buf.Bytes() }
func (b *boundedBuffer) String() string { return b.buf.String() }
