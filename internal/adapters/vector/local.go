// Package vector implements the VectorStore port: an in-process brute-force
// index for small footprints and a Qdrant adapter for larger deployments.
package vector

import (
	"context"
	"math"
	"sort"
	"sync"

	"github.com/ArsinShaabani/AIOS/internal/platform"
	"github.com/ArsinShaabani/AIOS/internal/ports"
)

// Local is a user-partitioned, in-memory cosine index. Memories are the source of
// truth in SQL; this index is warmed from them at boot.
type Local struct {
	mu     sync.RWMutex
	dim    int
	points map[string]*localPoint
}

type localPoint struct {
	userID  string
	vec     []float32
	payload map[string]any
}

var _ ports.VectorStore = (*Local)(nil)

// NewLocal creates an index for vectors of dimension dim.
func NewLocal(dim int) *Local {
	if dim <= 0 {
		dim = 384
	}
	return &Local{dim: dim, points: make(map[string]*localPoint, 256)}
}

// Upsert normalises and stores points (cosine == dot product).
func (l *Local) Upsert(_ context.Context, points []ports.VectorPoint) error {
	if len(points) == 0 {
		return nil
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	for _, p := range points {
		if len(p.Vector) != l.dim {
			return platform.Invalid("vector.dim", "embedding dimension mismatch")
		}
		vec := make([]float32, len(p.Vector))
		norm := float64(0)
		for _, v := range p.Vector {
			norm += float64(v) * float64(v)
		}
		norm = math.Sqrt(norm)
		if norm == 0 {
			norm = 1
		}
		for i, v := range p.Vector {
			vec[i] = float32(float64(v) / norm)
		}
		l.points[p.ID] = &localPoint{userID: p.UserID, vec: vec, payload: p.Payload}
	}
	return nil
}

// Delete removes points by ID.
func (l *Local) Delete(_ context.Context, ids []string) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	for _, id := range ids {
		delete(l.points, id)
	}
	return nil
}

// Search returns the topK most similar points for a user.
func (l *Local) Search(_ context.Context, userID string, vector []float32, topK int, minScore float64) ([]ports.VectorMatch, error) {
	if len(vector) != l.dim {
		return nil, platform.Invalid("vector.dim", "query dimension mismatch")
	}
	if topK <= 0 {
		topK = 5
	}
	norm := float64(0)
	for _, v := range vector {
		norm += float64(v) * float64(v)
	}
	norm = math.Sqrt(norm)
	if norm == 0 {
		return nil, nil
	}
	l.mu.RLock()
	defer l.mu.RUnlock()
	matches := make([]ports.VectorMatch, 0, topK*2)
	for id, p := range l.points {
		if userID != "" && p.userID != userID {
			continue
		}
		dot := float64(0)
		for i, v := range vector {
			dot += float64(v) * float64(p.vec[i])
		}
		score := dot / norm
		if score < minScore {
			continue
		}
		matches = append(matches, ports.VectorMatch{ID: id, Score: score, Payload: p.payload})
	}
	sort.Slice(matches, func(i, j int) bool { return matches[i].Score > matches[j].Score })
	if len(matches) > topK {
		matches = matches[:topK]
	}
	return matches, nil
}

// Count returns the number of indexed points.
func (l *Local) Count(context.Context) (int, error) {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return len(l.points), nil
}

// Ping always succeeds.
func (l *Local) Ping(context.Context) error { return nil }

// Len is an alias used by tests.
func (l *Local) Len() int {
	n, _ := l.Count(context.Background())
	return n
}
