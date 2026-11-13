package httpapi

import (
	"context"

	"github.com/ArsinShaabani/AIOS/internal/domain"
)

func userFrom(ctx context.Context) *domain.User {
	if v, ok := ctx.Value(userKey).(*domain.User); ok {
		return v
	}
	return nil
}
