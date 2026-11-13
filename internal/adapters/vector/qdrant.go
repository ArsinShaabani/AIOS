package vector

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/ArsinShaabani/AIOS/internal/adapters/httpx"
	"github.com/ArsinShaabani/AIOS/internal/config"
	"github.com/ArsinShaabani/AIOS/internal/platform"
	"github.com/ArsinShaabani/AIOS/internal/ports"
)

// Qdrant implements the VectorStore port against Qdrant's REST API. It is only
// needed when a deployment outgrows the local index.
type Qdrant struct {
	baseURL    string
	apiKey     string
	collection string
	dim        int
	client     *http.Client
	once       sync.Once
	initErr    error
}

var _ ports.VectorStore = (*Qdrant)(nil)

// NewQdrant builds a Qdrant-backed store.
func NewQdrant(cfg config.VectorConfig) *Qdrant {
	timeout := cfg.Timeout.Std()
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	return &Qdrant{
		baseURL:    trimSlash(cfg.QdrantURL),
		apiKey:     cfg.QdrantAPIKey,
		collection: cfg.Collection,
		dim:        cfg.Dim,
		client:     httpx.NewClient(timeout),
	}
}

func trimSlash(s string) string {
	for len(s) > 0 && s[len(s)-1] == '/' {
		s = s[:len(s)-1]
	}
	return s
}

func (q *Qdrant) headers() map[string]string {
	h := map[string]string{}
	if q.apiKey != "" {
		h["api-key"] = q.apiKey
	}
	return h
}

// pointID maps an arbitrary AIOS id to a Qdrant-compatible UUID deterministically.
func pointID(ref string) string {
	return uuid.NewMD5(uuid.NameSpaceOID, []byte(ref)).String()
}

func (q *Qdrant) ensure(ctx context.Context) error {
	q.once.Do(func() {
		var info struct {
			Result map[string]any `json:"result"`
		}
		err := httpx.DoJSON(ctx, q.client, http.MethodGet, q.baseURL+"/collections/"+q.collection, q.headers(), nil, &info, 1<<20)
		if err == nil {
			return
		}
		body := map[string]any{
			"vectors": map[string]any{"size": q.dim, "distance": "Cosine"},
		}
		if err := httpx.DoJSON(ctx, q.client, http.MethodPut, q.baseURL+"/collections/"+q.collection, q.headers(), body, nil, 1<<20); err != nil {
			q.initErr = platform.Wrap(platform.KindUnavailable, "qdrant.collection", "cannot create collection", err)
		}
	})
	return q.initErr
}

// Upsert stores points.
func (q *Qdrant) Upsert(ctx context.Context, points []ports.VectorPoint) error {
	if len(points) == 0 {
		return nil
	}
	if err := q.ensure(ctx); err != nil {
		return err
	}
	body := make([]map[string]any, 0, len(points))
	for _, p := range points {
		payload := map[string]any{"ref": p.ID, "user_id": p.UserID}
		for k, v := range p.Payload {
			payload[k] = v
		}
		body = append(body, map[string]any{
			"id":      pointID(p.ID),
			"vector":  p.Vector,
			"payload": payload,
		})
	}
	req := map[string]any{"points": body}
	if err := httpx.DoJSON(ctx, q.client, http.MethodPut, q.baseURL+"/collections/"+q.collection+"/points?wait=true", q.headers(), req, nil, 1<<20); err != nil {
		return platform.Wrap(platform.KindUnavailable, "qdrant.upsert", "qdrant upsert failed", err)
	}
	return nil
}

// Delete removes points by reference id.
func (q *Qdrant) Delete(ctx context.Context, ids []string) error {
	if len(ids) == 0 {
		return nil
	}
	if err := q.ensure(ctx); err != nil {
		return err
	}
	pointIDs := make([]string, len(ids))
	for i, id := range ids {
		pointIDs[i] = pointID(id)
	}
	req := map[string]any{"points": pointIDs}
	if err := httpx.DoJSON(ctx, q.client, http.MethodPost, q.baseURL+"/collections/"+q.collection+"/points/delete?wait=true", q.headers(), req, nil, 1<<20); err != nil {
		return platform.Wrap(platform.KindUnavailable, "qdrant.delete", "qdrant delete failed", err)
	}
	return nil
}

// Search runs a filtered similarity query.
func (q *Qdrant) Search(ctx context.Context, userID string, vector []float32, topK int, minScore float64) ([]ports.VectorMatch, error) {
	if err := q.ensure(ctx); err != nil {
		return nil, err
	}
	if topK <= 0 {
		topK = 5
	}
	req := map[string]any{
		"vector":          vector,
		"limit":           topK,
		"with_payload":    true,
		"score_threshold": minScore,
	}
	if userID != "" {
		req["filter"] = map[string]any{
			"must": []map[string]any{{"key": "user_id", "match": map[string]any{"value": userID}}},
		}
	}
	var out struct {
		Result []struct {
			ID      any            `json:"id"`
			Score   float64        `json:"score"`
			Payload map[string]any `json:"payload"`
		} `json:"result"`
	}
	if err := httpx.DoJSON(ctx, q.client, http.MethodPost, q.baseURL+"/collections/"+q.collection+"/points/search", q.headers(), req, &out, 4<<20); err != nil {
		return nil, platform.Wrap(platform.KindUnavailable, "qdrant.search", "qdrant search failed", err)
	}
	matches := make([]ports.VectorMatch, 0, len(out.Result))
	for _, r := range out.Result {
		ref := fmt.Sprint(r.ID)
		if v, ok := r.Payload["ref"].(string); ok && v != "" {
			ref = v
		}
		matches = append(matches, ports.VectorMatch{ID: ref, Score: r.Score, Payload: r.Payload})
	}
	return matches, nil
}

// Count returns the collection's point count.
func (q *Qdrant) Count(ctx context.Context) (int, error) {
	var out struct {
		Result struct {
			PointsCount int `json:"points_count"`
		} `json:"result"`
	}
	if err := httpx.DoJSON(ctx, q.client, http.MethodGet, q.baseURL+"/collections/"+q.collection, q.headers(), nil, &out, 1<<20); err != nil {
		return 0, platform.Wrap(platform.KindUnavailable, "qdrant.count", "qdrant count failed", err)
	}
	return out.Result.PointsCount, nil
}

// Ping checks connectivity.
func (q *Qdrant) Ping(ctx context.Context) error {
	return httpx.DoJSON(ctx, q.client, http.MethodGet, q.baseURL+"/collections", q.headers(), nil, nil, 1<<20)
}

// New builds the configured vector store.
func New(cfg config.VectorConfig) ports.VectorStore {
	if cfg.Driver == "qdrant" {
		return NewQdrant(cfg)
	}
	return NewLocal(cfg.Dim)
}
