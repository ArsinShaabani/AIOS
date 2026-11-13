package app

import (
	"context"
	"encoding/json"
	"time"

	"github.com/ArsinShaabani/AIOS/internal/brain"
	"github.com/ArsinShaabani/AIOS/internal/domain"
	"github.com/ArsinShaabani/AIOS/internal/memory"
	"github.com/ArsinShaabani/AIOS/internal/platform"
	"github.com/ArsinShaabani/AIOS/internal/ports"
)

// registerJobs binds queue handlers. Every handler is idempotent enough to
// survive at-least-once delivery.
func (a *App) registerJobs() {
	a.Queue.Register("memory.extract", func(ctx context.Context, job ports.Job) error {
		var payload struct {
			UserID    string `json:"user_id"`
			SessionID string `json:"session_id"`
			UserText  string `json:"user_text"`
		}
		if err := json.Unmarshal(job.Payload, &payload); err != nil || payload.UserID == "" {
			return nil
		}
		found := memory.Extract(payload.UserText)
		if len(found) == 0 {
			return nil
		}
		_, err := a.Memory.RememberExtracted(ctx, payload.UserID, payload.SessionID, "conversation", found)
		return err
	})

	a.Queue.Register("reminder", func(ctx context.Context, job ports.Job) error {
		var payload struct {
			UserID string `json:"user_id"`
			Prompt string `json:"prompt"`
			TaskID string `json:"task_id"`
		}
		if err := json.Unmarshal(job.Payload, &payload); err != nil || payload.UserID == "" {
			return nil
		}
		_, err := a.Memory.Remember(ctx, &domain.Memory{
			UserID: payload.UserID, Tier: domain.TierLong, Kind: domain.KindEpisode,
			Content: "Reminder: " + payload.Prompt, Summary: platform.Truncate(payload.Prompt, 160),
			Importance: 0.8, Source: "reminder", Tags: []string{"reminder"},
		})
		a.Bus.Publish(ctx, ports.Event{Name: "reminder.fired", UserID: payload.UserID, Payload: map[string]any{
			"prompt": payload.Prompt, "task_id": payload.TaskID,
		}})
		if a.MQTT != nil {
			topic := a.Config.MQTT.TopicPrefix + "/users/" + payload.UserID + "/notifications"
			_ = a.MQTT.Publish(ctx, topic, []byte(`{"type":"reminder","text":`+quoteJSON(payload.Prompt)+`}`), false)
		}
		return err
	})

	a.Queue.Register("automation.prompt", func(ctx context.Context, job ports.Job) error {
		var payload struct {
			UserID  string `json:"user_id"`
			Prompt  string `json:"prompt"`
			AgentID string `json:"agent_id"`
			Deliver string `json:"deliver"`
		}
		if err := json.Unmarshal(job.Payload, &payload); err != nil || payload.UserID == "" || payload.Prompt == "" {
			return nil
		}
		runCtx, cancel := context.WithTimeout(ctx, 3*time.Minute)
		defer cancel()
		out, err := a.Brain.Chat(runCtx, brain.ChatInput{
			UserID: payload.UserID, Message: payload.Prompt, AgentID: payload.AgentID,
			Channel: domain.ChannelAPI,
		}, nil)
		if err != nil {
			return err
		}
		if payload.Deliver == "mqtt" && a.MQTT != nil {
			topic := a.Config.MQTT.TopicPrefix + "/users/" + payload.UserID + "/notifications"
			return a.MQTT.Publish(ctx, topic, []byte(`{"type":"automation","text":`+quoteJSON(out.Content)+`}`), false)
		}
		_, err = a.Memory.Remember(ctx, &domain.Memory{
			UserID: payload.UserID, Kind: domain.KindEpisode, Content: out.Content,
			Summary: platform.Truncate(out.Content, 160), Source: "automation", Importance: 0.6,
		})
		return err
	})

	a.Queue.Register("memory.maintenance", func(ctx context.Context, job ports.Job) error {
		expired, old, err := a.Memory.Prune(ctx)
		if err != nil {
			return err
		}
		auditDeleted, _ := a.Repos.Audit.DeleteBefore(ctx, time.Now().UTC().AddDate(0, 0, -90))
		tasksDeleted, _ := a.Repos.Tasks.DeleteBefore(ctx, time.Now().UTC().AddDate(0, 0, -30))
		a.Log.Info("maintenance completed", "memories_expired", expired, "memories_old", old,
			"audit_deleted", auditDeleted, "tasks_deleted", tasksDeleted)
		return nil
	})
}

func quoteJSON(s string) string {
	raw, _ := json.Marshal(s)
	return string(raw)
}

func (a *App) startMQTTBridge(ctx context.Context) {
	if a.MQTTTransport != nil {
		if err := a.MQTTTransport.Subscribe(ctx, func(t ports.Telemetry) {
			a.Devices.HandleTelemetry(ctx, t)
		}); err != nil {
			a.Log.Warn("cannot subscribe to device telemetry", "error", err)
		} else {
			a.Log.Info("MQTT device bridge active", "prefix", a.Config.MQTT.TopicPrefix)
		}
	}
	if a.MQTTBridge != nil {
		if err := a.MQTTBridge.Start(ctx); err != nil {
			a.Log.Warn("cannot start MQTT chat bridge", "error", err)
		}
	} else if a.Config.MQTT.Bridge.Enabled {
		a.Log.Warn("MQTT chat bridge requested but the broker is not connected")
	}
}
