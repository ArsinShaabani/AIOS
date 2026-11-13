package e2e

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func wsURL(e *env, path string) string {
	return strings.Replace(e.server.URL, "http://", "ws://", 1) + path + "?token=" + e.token
}

func readUntil(t *testing.T, conn *websocket.Conn, want string, limit int) []map[string]any {
	t.Helper()
	var seen []map[string]any
	for i := 0; i < limit; i++ {
		_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
		_, raw, err := conn.ReadMessage()
		if err != nil {
			t.Fatalf("read: %v (events so far %v)", err, seen)
		}
		var msg map[string]any
		if err := jsonUnmarshal(raw, &msg); err != nil {
			continue
		}
		seen = append(seen, msg)
		if msg["type"] == want {
			return seen
		}
	}
	t.Fatalf("did not receive %q (events %v)", want, seen)
	return seen
}

func TestWebSocketChat(t *testing.T) {
	e := newEnv(t)
	conn, resp, err := websocket.DefaultDialer.Dial(wsURL(e, "/api/v1/ws/chat"), nil)
	if err != nil {
		status := 0
		if resp != nil {
			status = resp.StatusCode
		}
		t.Fatalf("dial: %v (status %d)", err, status)
	}
	defer conn.Close()
	readUntil(t, conn, "ready", 3)
	if err := conn.WriteJSON(map[string]any{"type": "message", "message": "hello via websocket"}); err != nil {
		t.Fatal(err)
	}
	events := readUntil(t, conn, "done", 40)
	var delta, session bool
	for _, ev := range events {
		switch ev["type"] {
		case "delta":
			delta = true
		case "session":
			session = true
		}
	}
	if !delta || !session {
		t.Fatalf("missing events: %v", events)
	}
	if err := conn.WriteJSON(map[string]any{"type": "ping"}); err != nil {
		t.Fatal(err)
	}
	readUntil(t, conn, "pong", 3)
	if err := conn.WriteJSON(map[string]any{"type": "cancel"}); err != nil {
		t.Fatal(err)
	}
	readUntil(t, conn, "cancelled", 3)
}

func TestWebSocketVoice(t *testing.T) {
	e := newEnv(t)
	url := strings.Replace(e.server.URL, "http://", "ws://", 1) + "/api/v1/ws/voice?token=" + e.token
	conn, resp, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		status := 0
		if resp != nil {
			status = resp.StatusCode
		}
		t.Fatalf("dial voice: %v (status %d)", err, status)
	}
	defer conn.Close()
	readUntil(t, conn, "ready", 3)
	if err := conn.WriteJSON(map[string]any{"type": "text", "message": "say hello"}); err != nil {
		t.Fatal(err)
	}
	sawReply, sawAudio, sawEnd := false, false, false
	for i := 0; i < 30; i++ {
		_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
		mt, raw, err := conn.ReadMessage()
		if err != nil {
			t.Fatalf("voice read: %v (reply=%v audio=%v end=%v)", err, sawReply, sawAudio, sawEnd)
		}
		if mt == websocket.BinaryMessage {
			if len(raw) > 44 {
				sawAudio = true
			}
			continue
		}
		var msg map[string]any
		if jsonUnmarshal(raw, &msg) != nil {
			continue
		}
		switch msg["type"] {
		case "reply":
			sawReply = true
		case "audio_end":
			sawEnd = true
		case "error":
			t.Fatalf("voice error: %v", msg)
		}
		if sawEnd {
			break
		}
	}
	if !sawReply || !sawAudio || !sawEnd {
		t.Fatalf("voice loop incomplete reply=%v audio=%v end=%v", sawReply, sawAudio, sawEnd)
	}
	if err := conn.WriteJSON(map[string]any{"type": "interrupt"}); err != nil {
		t.Fatal(err)
	}
	readUntil(t, conn, "cancelled", 5)
}

// jsonUnmarshal is kept tiny so the tests read naturally.
func jsonUnmarshal(raw []byte, v any) error { return json.Unmarshal(raw, v) }
