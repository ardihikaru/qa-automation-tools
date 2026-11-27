package auth

import (
	"context"
	"time"
)

// User is the authenticated principal stored on the request context.
// Mirrors the columns the handlers care about — never the password hash.
//
// While impersonating, User is the *target* and Impersonator is the real,
// authenticated user. A target identity never exists without Impersonator.
type User struct {
	ID           string `json:"id"`
	Username     string `json:"username"`
	FullName     string `json:"fullName,omitempty"`
	Email        string `json:"email,omitempty"`
	WhatsApp     string `json:"whatsApp,omitempty"`
	OdooID       string `json:"odooID,omitempty"`
	Role         string `json:"role"`
	IsLead       bool   `json:"isLead"`
	Active       bool   `json:"active"`
	MustChangePW bool   `json:"mustChangePassword"`
	// Language is the display language, "en" or "id" (KNOWLEDGE_TRANSLATION.md).
	Language string `json:"language"`

	// Impersonator is set only while this session acts as another user.
	Impersonator *Actor `json:"impersonator,omitempty"`

	// SessionToken is the cookie token of the real session (never serialised).
	SessionToken string `json:"-"`
	// SessionExpires is the real session's expiry (never serialised).
	SessionExpires time.Time `json:"-"`
}

// Actor identifies the real user behind an impersonated session.
type Actor struct {
	ID       string `json:"id"`
	Username string `json:"username"`
	FullName string `json:"fullName,omitempty"`
	Role     string `json:"role"`
}

// ImpersonatorRoles are the real roles allowed to impersonate. Shared by
// CanImpersonate and ValidateSession so the two never drift apart.
var ImpersonatorRoles = map[string]bool{"Admin": true, "Service": true}

// CanImpersonate reports whether u may start an impersonation. A session that
// is already impersonating can never start another (no nesting).
func CanImpersonate(u *User) bool {
	return u != nil && u.Impersonator == nil && ImpersonatorRoles[u.Role]
}

// RequireRealUser reports whether u is acting as itself. Every endpoint that
// changes a user account or its credentials must refuse when this is false.
func RequireRealUser(u *User) bool {
	return u != nil && u.Impersonator == nil
}

type ctxKey int

const userCtxKey ctxKey = 1

// SetUser stores the user on the context.
func SetUser(ctx context.Context, u *User) context.Context {
	return context.WithValue(ctx, userCtxKey, u)
}

// GetUser returns the user attached to ctx, or (nil, false) if absent.
func GetUser(ctx context.Context) (*User, bool) {
	u, ok := ctx.Value(userCtxKey).(*User)
	return u, ok && u != nil
}
