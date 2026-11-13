package tools

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ArsinShaabani/AIOS/internal/config"
	"github.com/ArsinShaabani/AIOS/internal/ports"
)

func TestHomeAssistantTool(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/api/states":
			fmt.Fprint(w, `[{"entity_id":"light.k","state":"on","attributes":{"friendly_name":"Kitchen","unit_of_measurement":null},"last_changed":"now"}]`)
		case strings.HasPrefix(r.URL.Path, "/api/states/"):
			fmt.Fprint(w, `{"entity_id":"light.k","state":"on"}`)
		case strings.HasPrefix(r.URL.Path, "/api/services/"):
			fmt.Fprint(w, `[{"entity_id":"light.k"}]`)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()
	ha := NewHomeAssistant(config.HomeAssistantConfig{Enabled: true, BaseURL: srv.URL, Token: "tok"})
	reg := NewRegistry()
	if err := RegisterHomeAssistantTool(reg, ha); err != nil {
		t.Fatal(err)
	}
	res, _ := reg.Execute(context.Background(), "home_assistant", ports.ToolCallContext{Arguments: map[string]any{"action": "list_states"}})
	if res.IsError || !strings.Contains(res.Content, "Kitchen") {
		t.Fatalf("list states: %+v", res)
	}
	res, _ = reg.Execute(context.Background(), "home_assistant", ports.ToolCallContext{Arguments: map[string]any{"action": "get_state", "entity_id": "light.k"}})
	if res.IsError || !strings.Contains(res.Content, "light.k") {
		t.Fatalf("get state: %+v", res)
	}
	res, _ = reg.Execute(context.Background(), "home_assistant", ports.ToolCallContext{Arguments: map[string]any{
		"action": "call_service", "service": "light.turn_on", "entity_id": "light.k", "data": map[string]any{"brightness": 30},
	}})
	if res.IsError || !strings.Contains(res.Content, `"ok":true`) {
		t.Fatalf("call service: %+v", res)
	}
	if _, err := ha.Call(context.Background(), "call_service", "light.k", "badservice", nil); err == nil {
		t.Fatal("invalid service accepted")
	}
	if _, err := ha.Call(context.Background(), "unknown", "", "", nil); err == nil {
		t.Fatal("unknown action accepted")
	}
	empty := NewHomeAssistant(config.HomeAssistantConfig{})
	if _, err := empty.Call(context.Background(), "list_states", "", "", nil); err == nil {
		t.Fatal("unconfigured HA must fail")
	}
}

func TestEmailAndMQTTTools(t *testing.T) {
	mail := &fakeEmail{}
	reg := NewRegistry()
	if err := RegisterEmailTool(reg, mail); err != nil {
		t.Fatal(err)
	}
	res, _ := reg.Execute(context.Background(), "send_email", ports.ToolCallContext{Arguments: map[string]any{
		"to": "a@b.c", "subject": "hi", "body": "text",
	}})
	if res.IsError || mail.sent != "a@b.c|hi|text" {
		t.Fatalf("email: %+v", res)
	}
	if res, _ := reg.Execute(context.Background(), "send_email", ports.ToolCallContext{}); !res.IsError {
		t.Fatal("missing email fields must fail")
	}
	pub := &fakePublisher{}
	if err := RegisterMQTTTool(reg, pub); err != nil {
		t.Fatal(err)
	}
	res, _ = reg.Execute(context.Background(), "mqtt_publish", ports.ToolCallContext{Arguments: map[string]any{"topic": "home/x", "payload": "on"}})
	if res.IsError || pub.topic != "home/x" || pub.payload != "on" {
		t.Fatalf("mqtt: %+v", res)
	}
	if res, _ := reg.Execute(context.Background(), "mqtt_publish", ports.ToolCallContext{Arguments: map[string]any{"topic": "home/+", "payload": "x"}}); !res.IsError {
		t.Fatal("wildcard topic must be rejected")
	}
	pub.err = errors.New("broker down")
	if res, _ := reg.Execute(context.Background(), "mqtt_publish", ports.ToolCallContext{Arguments: map[string]any{"topic": "home/x", "payload": "on"}}); !res.IsError {
		t.Fatal("publish error must surface")
	}
}
