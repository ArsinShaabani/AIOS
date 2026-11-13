// Command wasm-echo is the reference AIOS WASM plugin. Build it with:
//
//	GOOS=wasip1 GOARCH=wasm go build -o echo.wasm ./examples/plugins/wasm-echo
//
// It exposes a single `echo` tool and runs fully sandboxed: no filesystem, no
// network, no environment.
package main

import (
	"fmt"

	"github.com/ArsinShaabani/AIOS/sdk/wasmplugin"
)

func main() {
	wasmplugin.Serve(func(req wasmplugin.Request) wasmplugin.Response {
		switch req.Tool {
		case "echo":
			text := wasmplugin.String(req.Arguments, "text", "")
			if text == "" {
				return wasmplugin.Response{Content: "text is required", IsError: true}
			}
			return wasmplugin.Response{
				Content: "echo: " + text,
				Data:    map[string]any{"length": len([]rune(text)), "user_id": req.UserID},
			}
		default:
			return wasmplugin.Response{Content: fmt.Sprintf("unknown tool %q", req.Tool), IsError: true}
		}
	})
}
