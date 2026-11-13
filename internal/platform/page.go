package platform

// Page is the standard pagination request used by every list endpoint.
// Offset pagination is used for SQLite/Postgres portability; Admin APIs also
// expose cursor fields for future streaming sources.
type Page struct {
	Limit  int    `json:"limit" form:"limit"`
	Offset int    `json:"offset" form:"offset"`
	Cursor string `json:"cursor,omitempty" form:"cursor"`
}

// Normalize clamps pagination to safe bounds. Defaults are deliberately small
// so a weak server never builds a 100k-row response by accident.
func (p Page) Normalize() Page {
	if p.Limit <= 0 {
		p.Limit = 25
	}
	if p.Limit > 200 {
		p.Limit = 200
	}
	if p.Offset < 0 {
		p.Offset = 0
	}
	return p
}

// PageResult wraps a page of rows with the total row count (when cheap).
type PageResult[T any] struct {
	Items  []T `json:"items"`
	Total  int `json:"total"`
	Limit  int `json:"limit"`
	Offset int `json:"offset"`
}

// NewPageResult builds a page result, guaranteeing a non-nil slice so JSON
// clients always receive [] instead of null.
func NewPageResult[T any](items []T, total int, p Page) PageResult[T] {
	if items == nil {
		items = []T{}
	}
	p = p.Normalize()
	return PageResult[T]{Items: items, Total: total, Limit: p.Limit, Offset: p.Offset}
}
