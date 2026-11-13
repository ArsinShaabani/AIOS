package e2e

import (
	"bytes"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"testing"
	"time"

	"github.com/ArsinShaabani/AIOS/internal/voice"
)

func TestVoiceREST(t *testing.T) {
	e := newEnv(t)
	if status, cfg := e.do("GET", "/api/v1/voice/config", e.token, nil); status != 200 || cfg["enabled"] != true {
		t.Fatalf("voice config: %d %v", status, cfg)
	}
	status, audio := e.do("POST", "/api/v1/voice/synthesize", e.token, map[string]any{"text": "Hello from AIOS"})
	_ = audio
	// synthesize returns audio, not JSON; use a raw request below.
	req, _ := http.NewRequest("POST", e.server.URL+"/api/v1/voice/synthesize", bytes.NewBufferString(`{"text":"Hello from AIOS"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+e.token)
	resp, err := e.client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 200 || len(raw) < 44 || resp.Header.Get("Content-Type") != "audio/wav" {
		t.Fatalf("synthesize: %d len=%d type=%s", resp.StatusCode, len(raw), resp.Header.Get("Content-Type"))
	}
	_ = status

	// Transcribe a real WAV built by the voice package.
	wav := voice.EncodeWAV(voice.Beep(250, 16000, 440), 16000)
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	part, _ := mw.CreateFormFile("audio", "sample.wav")
	_, _ = part.Write(wav)
	_ = mw.Close()
	req, _ = http.NewRequest("POST", e.server.URL+"/api/v1/voice/transcribe", &body)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Set("Authorization", "Bearer "+e.token)
	resp, err = e.client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	transcript, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 200 || !bytes.Contains(transcript, []byte("hello aios")) {
		t.Fatalf("transcribe: %d %s", resp.StatusCode, transcript)
	}

	if status, _ := e.do("POST", "/api/v1/voice/synthesize", e.token, map[string]any{"text": ""}); status != 400 {
		t.Fatalf("empty text must be 400, got %d", status)
	}
	// Missing multipart file.
	req, _ = http.NewRequest("POST", e.server.URL+"/api/v1/voice/transcribe", bytes.NewBufferString("garbage"))
	req.Header.Set("Content-Type", "multipart/form-data; boundary=xxx")
	req.Header.Set("Authorization", "Bearer "+e.token)
	resp, _ = e.client.Do(req)
	resp.Body.Close()
	if resp.StatusCode != 400 {
		t.Fatalf("invalid multipart must be 400, got %d", resp.StatusCode)
	}
}

func TestGraphQLAll(t *testing.T) {
	e := newEnv(t)
	e.do("POST", "/api/v1/chat", e.token, map[string]any{"message": "hello"})
	if status, mem := e.do("POST", "/api/v1/memories", e.token, map[string]any{"content": "graphql fact"}); status != 201 {
		t.Fatalf("seed memory: %v", mem)
	}
	query := `{"query":"query { me { email role } sessions(limit: 5) { title messageCount } agents { name enabled tools } memories(query: \"graphql\", limit: 3) { id kind content } devices { id } providers { name healthy } }"}`
	status, result := e.do("POST", "/graphql", e.token, json.RawMessage(query))
	if status != 200 || result["data"] == nil {
		t.Fatalf("graphql query: %d %v", status, result)
	}
	overview := `{"query":"query { overview { users devices memories } }"}`
	if status, result := e.do("POST", "/graphql", e.token, json.RawMessage(overview)); status != 200 {
		t.Fatalf("graphql overview: %d %v", status, result)
	}
	mutation := `{"query":"mutation { sendMessage(message: \"hi\") { content provider } }"}`
	if status, result := e.do("POST", "/graphql", e.token, json.RawMessage(mutation)); status != 200 {
		t.Fatalf("graphql mutation: %d %v", status, result)
	}
	create := `{"query":"mutation { createMemory(content: \"from graphql\") { id content } }"}`
	status, result = e.do("POST", "/graphql", e.token, json.RawMessage(create))
	if status != 200 {
		t.Fatalf("graphql createMemory: %d %v", status, result)
	}
	data := result["data"].(map[string]any)["createMemory"].(map[string]any)
	id := data["id"].(string)
	del := `{"query":"mutation { deleteMemory(id: \"` + id + `\") }"}`
	if status, result := e.do("POST", "/graphql", e.token, json.RawMessage(del)); status != 200 || result["data"].(map[string]any)["deleteMemory"] != true {
		t.Fatalf("graphql deleteMemory: %d %v", status, result)
	}
	if status, _ := e.do("POST", "/graphql", e.token, json.RawMessage(`{"query":"query { nope }"}`)); status != 400 {
		t.Fatalf("invalid graphql must be 400, got %d", status)
	}
	if status, _ := e.do("POST", "/graphql", e.token, map[string]any{"query": ""}); status != 400 {
		t.Fatalf("empty graphql must be 400, got %d", status)
	}
}

// ensure time is used if the file is edited down
var _ = time.Second
