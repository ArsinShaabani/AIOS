// Package agents owns the agent definitions and routing heuristics that choose
// the best specialist for a request. Agents are data, seeded on first boot and
// editable through the API.
package agents

import (
	"context"
	"log/slog"
	"strings"
	"sync"

	"github.com/ArsinShaabani/AIOS/internal/domain"
	"github.com/ArsinShaabani/AIOS/internal/ports"
)

// Registry loads agents from storage and routes requests to them.
type Registry struct {
	repo   ports.AgentRepository
	log    *slog.Logger
	mu     sync.RWMutex
	cache  []domain.Agent
	loaded bool
}

// New builds the registry.
func New(repo ports.AgentRepository, log *slog.Logger) *Registry {
	if log == nil {
		log = slog.Default()
	}
	return &Registry{repo: repo, log: log}
}

// Reload refreshes the in-memory view.
func (r *Registry) Reload(ctx context.Context) error {
	rows, err := r.repo.List(ctx, true)
	if err != nil {
		return err
	}
	r.mu.Lock()
	r.cache = rows
	r.loaded = true
	r.mu.Unlock()
	return nil
}

// List returns agents (from cache when warm).
func (r *Registry) List(ctx context.Context, enabledOnly bool) ([]domain.Agent, error) {
	if enabledOnly {
		if rows, ok := r.cached(); ok {
			return rows, nil
		}
	}
	return r.repo.List(ctx, enabledOnly)
}

func (r *Registry) cached() ([]domain.Agent, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if !r.loaded {
		return nil, false
	}
	out := make([]domain.Agent, len(r.cache))
	copy(out, r.cache)
	return out, true
}

// Get loads a single agent.
func (r *Registry) Get(ctx context.Context, id string) (*domain.Agent, error) {
	if rows, ok := r.cached(); ok {
		for i := range rows {
			if rows[i].ID == id {
				agent := rows[i]
				return &agent, nil
			}
		}
	}
	return r.repo.Get(ctx, id)
}

// Select picks the best agent for an intent and message. Explicit IDs win;
// otherwise keyword scoring with priority tie-breaks, falling back to general.
func (r *Registry) Select(ctx context.Context, explicit, intent, text string) *domain.Agent {
	rows, ok := r.cached()
	if !ok {
		rows, _ = r.repo.List(ctx, true)
	}
	if explicit != "" {
		for i := range rows {
			if rows[i].ID == explicit || strings.EqualFold(rows[i].Name, explicit) {
				agent := rows[i]
				return &agent
			}
		}
	}
	lower := strings.ToLower(text)
	best := -1
	bestScore := 0
	for i := range rows {
		a := rows[i]
		score := 0
		for _, kw := range a.Keywords {
			kw = strings.ToLower(strings.TrimSpace(kw))
			if kw == "" {
				continue
			}
			if strings.Contains(lower, kw) {
				score += 3
				if strings.HasPrefix(lower, kw) {
					score += 2
				}
			}
		}
		if intent != "" {
			for _, kw := range a.Keywords {
				if strings.EqualFold(kw, intent) {
					score += 4
				}
			}
		}
		if a.Name != "" && strings.Contains(lower, strings.ToLower(a.Name)) {
			score += 2
		}
		if score == 0 {
			continue // never route to a specialist on priority alone
		}
		// Priority only breaks ties between genuine keyword matches.
		if score > bestScore || (score == bestScore && best >= 0 && a.Priority > rows[best].Priority) {
			bestScore = score
			best = i
		}
	}
	if best >= 0 && bestScore > 0 {
		agent := rows[best]
		return &agent
	}
	for i := range rows {
		if rows[i].ID == "agent_general" {
			agent := rows[i]
			return &agent
		}
	}
	if len(rows) > 0 {
		agent := rows[0]
		return &agent
	}
	return &domain.Agent{ID: "agent_general", Name: "General", Enabled: true,
		SystemPrompt: "You are a helpful personal assistant.", Tools: []string{}}
}

// Seed inserts built-in agents and refreshes code-owned prompts.
func (r *Registry) Seed(ctx context.Context) error {
	for _, a := range Builtins() {
		a := a
		if err := r.repo.Upsert(ctx, &a); err != nil {
			return err
		}
	}
	return r.Reload(ctx)
}
