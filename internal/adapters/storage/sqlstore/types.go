package sqlstore

import (
	"database/sql"
	"encoding/json"
	"time"
)

// tsLayout is fixed-width UTC so TEXT timestamps sort chronologically in both
// SQLite and Postgres.
const tsLayout = "2006-01-02T15:04:05.000000000Z"

func ft(t time.Time) string { return t.UTC().Format(tsLayout) }

func ftp(t *time.Time) any {
	if t == nil {
		return nil
	}
	return ft(*t)
}

func parseTime(s string) time.Time {
	t, err := time.Parse(tsLayout, s)
	if err != nil {
		t, _ = time.Parse(time.RFC3339Nano, s)
	}
	return t
}

func ntp(s sql.NullString) *time.Time {
	if !s.Valid || s.String == "" {
		return nil
	}
	t := parseTime(s.String)
	return &t
}

func nstr(s sql.NullString) string {
	if s.Valid {
		return s.String
	}
	return ""
}

func jsonText(v any) string {
	if v == nil {
		return "{}"
	}
	raw, err := json.Marshal(v)
	if err != nil {
		return "{}"
	}
	return string(raw)
}

func jsonSliceText(v []string) string {
	if len(v) == 0 {
		return "[]"
	}
	raw, err := json.Marshal(v)
	if err != nil {
		return "[]"
	}
	return string(raw)
}

func decodeJSON(s string, v any) {
	if s == "" {
		return
	}
	_ = json.Unmarshal([]byte(s), v)
}

func decodeStrings(s string) []string {
	if s == "" {
		return nil
	}
	var out []string
	_ = json.Unmarshal([]byte(s), &out)
	return out
}

func decodeFloats(s string) []float32 {
	if s == "" {
		return nil
	}
	var out []float32
	_ = json.Unmarshal([]byte(s), &out)
	return out
}

func b2i(b bool) int {
	if b {
		return 1
	}
	return 0
}
