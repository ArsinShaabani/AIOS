package sqlstore

import (
	"context"
	"time"

	"github.com/ArsinShaabani/AIOS/internal/domain"
)

// PersonalityRepo implements ports.PersonalityRepository.
type PersonalityRepo struct{ db *DB }

func NewPersonalityRepo(db *DB) *PersonalityRepo { return &PersonalityRepo{db: db} }

const personalityCols = `id, owner_id, name, description, tone, style, humor, emotional_level,
	response_length, custom_prompt, is_preset, created_at, updated_at`

func scanPersonality(row rowScanner) (*domain.Personality, error) {
	var (
		p       domain.Personality
		preset  int
		created string
		updated string
	)
	if err := row.Scan(&p.ID, &p.OwnerID, &p.Name, &p.Description, &p.Tone, &p.Style, &p.Humor,
		&p.EmotionalLevel, &p.ResponseLength, &p.CustomPrompt, &preset, &created, &updated); err != nil {
		return nil, err
	}
	p.IsPreset = preset == 1
	p.CreatedAt = parseTime(created)
	p.UpdatedAt = parseTime(updated)
	return &p, nil
}

// Create inserts a personality.
func (r *PersonalityRepo) Create(ctx context.Context, p *domain.Personality) error {
	now := time.Now().UTC()
	if p.CreatedAt.IsZero() {
		p.CreatedAt = now
	}
	p.UpdatedAt = now
	p.Clamp()
	_, err := r.db.ExecContext(ctx, r.db.Rebind(`INSERT INTO personalities (`+personalityCols+`)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`),
		p.ID, p.OwnerID, p.Name, p.Description, p.Tone, p.Style, p.Humor, p.EmotionalLevel,
		p.ResponseLength, p.CustomPrompt, b2i(p.IsPreset), ft(p.CreatedAt), ft(p.UpdatedAt))
	return mapErr("personality.create", err)
}

// Update rewrites a personality.
func (r *PersonalityRepo) Update(ctx context.Context, p *domain.Personality) error {
	p.UpdatedAt = time.Now().UTC()
	p.Clamp()
	res, err := r.db.ExecContext(ctx, r.db.Rebind(`UPDATE personalities SET name = ?, description = ?, tone = ?,
		style = ?, humor = ?, emotional_level = ?, response_length = ?, custom_prompt = ?, updated_at = ? WHERE id = ?`),
		p.Name, p.Description, p.Tone, p.Style, p.Humor, p.EmotionalLevel, p.ResponseLength, p.CustomPrompt, ft(p.UpdatedAt), p.ID)
	if err != nil {
		return mapErr("personality.update", err)
	}
	return ensureAffected(res, "personality.update")
}

// Get loads one personality.
func (r *PersonalityRepo) Get(ctx context.Context, id string) (*domain.Personality, error) {
	row := r.db.QueryRowContext(ctx, r.db.Rebind(`SELECT `+personalityCols+` FROM personalities WHERE id = ?`), id)
	p, err := scanPersonality(row)
	if err != nil {
		return nil, mapErr("personality.get", err)
	}
	return p, nil
}

// Delete removes a personality (presets are protected by the service layer).
func (r *PersonalityRepo) Delete(ctx context.Context, id string) error {
	res, err := r.db.ExecContext(ctx, r.db.Rebind(`DELETE FROM personalities WHERE id = ?`), id)
	if err != nil {
		return mapErr("personality.delete", err)
	}
	return ensureAffected(res, "personality.delete")
}

// ListAvailable returns presets plus the owner's personalities.
func (r *PersonalityRepo) ListAvailable(ctx context.Context, ownerID string) ([]domain.Personality, error) {
	rows, err := r.db.QueryContext(ctx, r.db.Rebind(`SELECT `+personalityCols+` FROM personalities
		WHERE owner_id = '' OR owner_id = ? ORDER BY is_preset DESC, name ASC`), ownerID)
	if err != nil {
		return nil, mapErr("personality.list", err)
	}
	defer rows.Close()
	out := make([]domain.Personality, 0, 8)
	for rows.Next() {
		p, err := scanPersonality(rows)
		if err != nil {
			return nil, mapErr("personality.list_scan", err)
		}
		out = append(out, *p)
	}
	return out, mapErr("personality.list_iter", rows.Err())
}

// Count counts personalities.
func (r *PersonalityRepo) Count(ctx context.Context) (int, error) {
	var n int
	err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM personalities`).Scan(&n)
	return n, mapErr("personality.count", err)
}
