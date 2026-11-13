package sqlstore

import (
	"context"
	"database/sql"
	"time"

	"github.com/ArsinShaabani/AIOS/internal/domain"
)

// DeviceRepo implements ports.DeviceRepository.
type DeviceRepo struct{ db *DB }

func NewDeviceRepo(db *DB) *DeviceRepo { return &DeviceRepo{db: db} }

const deviceCols = `id, user_id, name, kind, protocol, hardware, room, status, capabilities, metadata,
	token_hash, token_prefix, firmware, last_seen_at, created_at, updated_at`

func scanDevice(row rowScanner) (*domain.Device, error) {
	var (
		d        domain.Device
		caps     string
		meta     string
		lastSeen sql.NullString
		created  string
		updated  string
	)
	if err := row.Scan(&d.ID, &d.UserID, &d.Name, &d.Kind, &d.Protocol, &d.Hardware, &d.Room, &d.Status,
		&caps, &meta, &d.TokenHash, &d.TokenPrefix, &d.Firmware, &lastSeen, &created, &updated); err != nil {
		return nil, err
	}
	decodeJSON(caps, &d.Capabilities)
	decodeJSON(meta, &d.Metadata)
	d.LastSeenAt = ntp(lastSeen)
	d.CreatedAt = parseTime(created)
	d.UpdatedAt = parseTime(updated)
	return &d, nil
}

// Create inserts a device.
func (r *DeviceRepo) Create(ctx context.Context, d *domain.Device) error {
	now := time.Now().UTC()
	if d.CreatedAt.IsZero() {
		d.CreatedAt = now
	}
	d.UpdatedAt = now
	if d.Status == "" {
		d.Status = "unknown"
	}
	if d.Kind == "" {
		d.Kind = domain.DeviceSensor
	}
	if d.Protocol == "" {
		d.Protocol = domain.ProtoMQTT
	}
	_, err := r.db.ExecContext(ctx, r.db.Rebind(`INSERT INTO devices (`+deviceCols+`)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`),
		d.ID, d.UserID, d.Name, string(d.Kind), string(d.Protocol), d.Hardware, d.Room, d.Status,
		jsonText(d.Capabilities), jsonText(d.Metadata), d.TokenHash, d.TokenPrefix, d.Firmware,
		ftp(d.LastSeenAt), ft(d.CreatedAt), ft(d.UpdatedAt))
	return mapErr("device.create", err)
}

// Update rewrites a device.
func (r *DeviceRepo) Update(ctx context.Context, d *domain.Device) error {
	d.UpdatedAt = time.Now().UTC()
	res, err := r.db.ExecContext(ctx, r.db.Rebind(`UPDATE devices SET name = ?, kind = ?, protocol = ?, hardware = ?,
		room = ?, capabilities = ?, metadata = ?, token_hash = ?, token_prefix = ?, firmware = ?, updated_at = ?
		WHERE id = ?`),
		d.Name, string(d.Kind), string(d.Protocol), d.Hardware, d.Room, jsonText(d.Capabilities),
		jsonText(d.Metadata), d.TokenHash, d.TokenPrefix, d.Firmware, ft(d.UpdatedAt), d.ID)
	if err != nil {
		return mapErr("device.update", err)
	}
	return ensureAffected(res, "device.update")
}

// Get loads one device.
func (r *DeviceRepo) Get(ctx context.Context, id string) (*domain.Device, error) {
	row := r.db.QueryRowContext(ctx, r.db.Rebind(`SELECT `+deviceCols+` FROM devices WHERE id = ?`), id)
	d, err := scanDevice(row)
	if err != nil {
		return nil, mapErr("device.get", err)
	}
	return d, nil
}

// GetByTokenPrefix finds a device during token authentication.
func (r *DeviceRepo) GetByTokenPrefix(ctx context.Context, prefix string) (*domain.Device, error) {
	row := r.db.QueryRowContext(ctx, r.db.Rebind(`SELECT `+deviceCols+` FROM devices WHERE token_prefix = ?`), prefix)
	d, err := scanDevice(row)
	if err != nil {
		return nil, mapErr("device.get_by_token", err)
	}
	return d, nil
}

// SetStatus updates presence and the last-seen timestamp.
func (r *DeviceRepo) SetStatus(ctx context.Context, id, status string, at time.Time) error {
	res, err := r.db.ExecContext(ctx, r.db.Rebind(`UPDATE devices SET status = ?, last_seen_at = ?, updated_at = ? WHERE id = ?`),
		status, ft(at), ft(at), id)
	if err != nil {
		return mapErr("device.status", err)
	}
	return ensureAffected(res, "device.status")
}

// Delete removes a device.
func (r *DeviceRepo) Delete(ctx context.Context, id string) error {
	res, err := r.db.ExecContext(ctx, r.db.Rebind(`DELETE FROM devices WHERE id = ?`), id)
	if err != nil {
		return mapErr("device.delete", err)
	}
	return ensureAffected(res, "device.delete")
}
