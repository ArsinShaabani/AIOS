package brain

import (
	"context"
	"time"

	"github.com/ArsinShaabani/AIOS/internal/domain"
	"github.com/ArsinShaabani/AIOS/internal/platform"
	"github.com/ArsinShaabani/AIOS/internal/ports"
)

// executeTool runs one tool call, emits events and persists the tool message.
// The boolean is false when the client disconnected and generation must stop.
func (e *Engine) executeTool(ctx context.Context, st turnState, call domain.ToolCall) (string, bool) {
	if st.emit != nil {
		if err := st.emit(Event{Type: "tool_start", Tool: &ToolEvent{CallID: call.ID, Name: call.Name, Arguments: call.Arguments}}); err != nil {
			return "", false
		}
	}
	start := time.Now()
	result, err := e.Tools.Execute(ctx, call.Name, ports.ToolCallContext{
		UserID: st.user.ID, SessionID: st.session.ID, AgentID: st.agent.ID, Arguments: call.Arguments,
	})
	content := result.Content
	isError := result.IsError
	if err != nil {
		content = "ERROR: " + err.Error()
		isError = true
	}
	if content == "" {
		content = "(tool returned no content)"
	}
	content = platform.Truncate(content, 16000)
	if st.emit != nil {
		_ = st.emit(Event{Type: "tool_result", Tool: &ToolEvent{
			CallID: call.ID, Name: call.Name, Result: platform.Truncate(content, 2000),
			IsError: isError, LatencyMS: time.Since(start).Milliseconds(),
		}})
	}
	_ = e.Messages.Create(ctx, &domain.Message{
		ID: platform.NewID("msg"), SessionID: st.session.ID, UserID: st.user.ID, Role: domain.MsgTool,
		Content: content, Name: call.Name, ToolCallID: call.ID, CreatedAt: time.Now().UTC(),
	})
	_ = e.Sessions.Touch(ctx, st.session.ID, 1, time.Now().UTC())
	return content, true
}
