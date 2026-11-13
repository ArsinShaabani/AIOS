package security

import (
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/ArsinShaabani/AIOS/internal/config"
	"github.com/ArsinShaabani/AIOS/internal/domain"
	"github.com/ArsinShaabani/AIOS/internal/platform"
	"github.com/ArsinShaabani/AIOS/internal/ports"
)

// Principal is the authenticated identity travelling on a request context.
type Principal struct {
	UserID   string      `json:"user_id"`
	Role     domain.Role `json:"role"`
	Scopes   []string    `json:"scopes,omitempty"`
	Method   string      `json:"method"` // jwt | api_key | device
	DeviceID string      `json:"device_id,omitempty"`
}

// Can checks a permission for the principal.
func (p *Principal) Can(perm Permission) bool {
	if p == nil {
		return false
	}
	if p.Method == "device" {
		return perm == PermDevicesRead || perm == PermDevicesWrite
	}
	if len(p.Scopes) > 0 {
		for _, s := range p.Scopes {
			if s == string(PermAdmin) || s == string(perm) {
				return Can(p.Role, perm)
			}
		}
		return false
	}
	return Can(p.Role, perm)
}

// Claims is the JWT payload.
type Claims struct {
	Role     string   `json:"role"`
	Scopes   []string `json:"scopes,omitempty"`
	Type     string   `json:"typ"`
	DeviceID string   `json:"did,omitempty"`
	jwt.RegisteredClaims
}

// TokenPair is returned by login/refresh.
type TokenPair struct {
	AccessToken      string       `json:"access_token"`
	RefreshToken     string       `json:"refresh_token"`
	TokenType        string       `json:"token_type"`
	ExpiresIn        int          `json:"expires_in"`
	RefreshExpiresAt time.Time    `json:"refresh_expires_at"`
	User             *domain.User `json:"user,omitempty"`
}

// TokenService issues and verifies access/refresh tokens.
type TokenService struct {
	keyring    *Keyring
	issuer     string
	accessTTL  time.Duration
	refreshTTL time.Duration
	skew       time.Duration
	tokens     ports.TokenRepository
	users      ports.UserRepository
	now        func() time.Time
}

// NewTokenService builds the token service.
func NewTokenService(cfg config.SecurityConfig, keyring *Keyring, tokens ports.TokenRepository, users ports.UserRepository) *TokenService {
	return &TokenService{
		keyring:    keyring,
		issuer:     cfg.Issuer,
		accessTTL:  cfg.AccessTTL.Std(),
		refreshTTL: cfg.RefreshTTL.Std(),
		skew:       cfg.AllowedClockSkew.Std(),
		tokens:     tokens,
		users:      users,
		now:        time.Now,
	}
}

// SetClock overrides the clock (tests).
func (s *TokenService) SetClock(fn func() time.Time) { s.now = fn }

// IssueAccess signs a short-lived JWT for a user.
func (s *TokenService) IssueAccess(u *domain.User) (string, time.Time, error) {
	now := s.now().UTC()
	exp := now.Add(s.accessTTL)
	claims := Claims{
		Role: string(u.Role),
		Type: "access",
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   u.ID,
			Issuer:    s.issuer,
			IssuedAt:  jwt.NewNumericDate(now),
			NotBefore: jwt.NewNumericDate(now.Add(-s.skew)),
			ExpiresAt: jwt.NewNumericDate(exp),
			ID:        platform.NewID("jti"),
		},
	}
	signed, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(s.keyring.SigningKey())
	if err != nil {
		return "", time.Time{}, platform.Wrap(platform.KindInternal, "token.sign", "cannot sign access token", err)
	}
	return signed, exp, nil
}

// ParseAccess verifies a JWT and returns the principal.
func (s *TokenService) ParseAccess(raw string) (*Principal, error) {
	claims := &Claims{}
	parser := jwt.NewParser(
		jwt.WithValidMethods([]string{"HS256"}),
		jwt.WithIssuer(s.issuer),
		jwt.WithLeeway(s.skew),
		jwt.WithTimeFunc(s.now),
	)
	token, err := parser.ParseWithClaims(raw, claims, func(t *jwt.Token) (any, error) {
		return s.keyring.SigningKey(), nil
	})
	if err != nil || !token.Valid {
		return nil, platform.Unauthorized("token.invalid", "invalid or expired access token")
	}
	if claims.Type != "access" || claims.Subject == "" {
		return nil, platform.Unauthorized("token.invalid_type", "invalid token type")
	}
	return &Principal{UserID: claims.Subject, Role: domain.Role(claims.Role), Method: "jwt"}, nil
}

// Now returns the service clock.
func (s *TokenService) Now() time.Time { return s.now().UTC() }
