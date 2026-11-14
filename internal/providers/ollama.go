package providers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// OllamaModel is a locally installed model.
type OllamaModel struct {
	Name       string `json:"name"`
	Size       int64  `json:"size"`
	ModifiedAt string `json:"modified_at"`
	Family     string `json:"family,omitempty"`
	Params     string `json:"parameters,omitempty"`
}

// PullStatus tracks an asynchronous model download.
type PullStatus struct {
	Model    string    `json:"model"`
	Status   string    `json:"status"`
	Error    string    `json:"error,omitempty"`
	Done     bool      `json:"done"`
	Started  time.Time `json:"started_at"`
	Finished time.Time `json:"finished_at,omitempty"`
}

// OllamaBaseURL resolves the local Ollama endpoint from active providers.
func (s *Service) OllamaBaseURL() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, p := range s.llm.Providers {
		if p.Kind == "ollama" && p.BaseURL != "" {
			return strings.TrimRight(p.BaseURL, "/")
		}
	}
	return "http://127.0.0.1:11434"
}

func (s *Service) ollamaClient() *http.Client { return &http.Client{Timeout: 15 * time.Second} }

// OllamaModels lists installed models via /api/tags.
func (s *Service) OllamaModels(ctx context.Context) ([]OllamaModel, error) {
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, s.OllamaBaseURL()+"/api/tags", nil)
	resp, err := s.ollamaClient().Do(req)
	if err != nil {
		return nil, fmt.Errorf("ollama unreachable at %s: %w", s.OllamaBaseURL(), err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return nil, fmt.Errorf("ollama returned %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var payload struct {
		Models []struct {
			Name       string `json:"name"`
			Size       int64  `json:"size"`
			ModifiedAt string `json:"modified_at"`
			Details    struct {
				Family        string `json:"family"`
				ParameterSize string `json:"parameter_size"`
			} `json:"details"`
		} `json:"models"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, err
	}
	out := make([]OllamaModel, 0, len(payload.Models))
	for _, m := range payload.Models {
		out = append(out, OllamaModel{Name: m.Name, Size: m.Size, ModifiedAt: m.ModifiedAt,
			Family: m.Details.Family, Params: m.Details.ParameterSize})
	}
	return out, nil
}

// PullModel downloads a model in the background; poll PullStatuses.
func (s *Service) PullModel(model string) (*PullStatus, error) {
	model = strings.TrimSpace(model)
	if model == "" {
		return nil, fmt.Errorf("model name is required")
	}
	s.mu.Lock()
	if st, ok := s.pulls[model]; ok && !st.Done {
		s.mu.Unlock()
		return st, nil
	}
	st := &PullStatus{Model: model, Status: "downloading", Started: time.Now()}
	s.pulls[model] = st
	s.mu.Unlock()

	go func() {
		body, _ := json.Marshal(map[string]any{"model": model, "stream": false})
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
		defer cancel()
		req, _ := http.NewRequestWithContext(ctx, http.MethodPost, s.OllamaBaseURL()+"/api/pull", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		resp, err := (&http.Client{}).Do(req)
		s.mu.Lock()
		defer s.mu.Unlock()
		st.Done, st.Finished = true, time.Now()
		if err != nil {
			st.Status, st.Error = "failed", err.Error()
			return
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			msg, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
			st.Status, st.Error = "failed", fmt.Sprintf("ollama %d: %s", resp.StatusCode, strings.TrimSpace(string(msg)))
			return
		}
		st.Status = "done"
	}()
	return st, nil
}

// PullStatuses returns a snapshot of download states.
func (s *Service) PullStatuses() []PullStatus {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]PullStatus, 0, len(s.pulls))
	for _, st := range s.pulls {
		out = append(out, *st)
	}
	return out
}

// DeleteOllamaModel removes a downloaded model.
func (s *Service) DeleteOllamaModel(ctx context.Context, model string) error {
	body, _ := json.Marshal(map[string]any{"model": model})
	req, _ := http.NewRequestWithContext(ctx, http.MethodDelete, s.OllamaBaseURL()+"/api/delete", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := s.ollamaClient().Do(req)
	if err != nil {
		return fmt.Errorf("ollama unreachable: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("ollama %d: %s", resp.StatusCode, strings.TrimSpace(string(msg)))
	}
	return nil
}
