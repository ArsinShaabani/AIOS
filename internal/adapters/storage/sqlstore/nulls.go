package sqlstore

import (
	"database/sql"
	"time"
)

// sqlNullString helps scan nullable TEXT timestamps and convert them to
// *time.Time without repeating sql.NullString handling everywhere.
type sqlNullString struct{ sql.NullString }

func (s *sqlNullString) Scan(v any) error { return s.NullString.Scan(v) }

// Ptr returns a *time.Time or nil.
func (s sqlNullString) Ptr() *time.Time {
	if !s.Valid || s.String == "" {
		return nil
	}
	t := parseTime(s.String)
	return &t
}
