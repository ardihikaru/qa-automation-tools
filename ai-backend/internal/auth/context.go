package auth

import "context"

// User is the authenticated principal stored on the request context.
// Mirrors the columns the handlers care about — never the password hash.
type User struct {
	ID               string `json:"id"`
	Username         string `json:"username"`
	FullName         string `json:"fullName,omitempty"`
	Email            string `json:"email,omitempty"`
	WhatsApp         string `json:"whatsApp,omitempty"`
	OdooID           string `json:"odooID,omitempty"`
	Role             string `json:"role"`
	IsLead           bool   `json:"isLead"`
	Active           bool   `json:"active"`
	MustChangePW     bool   `json:"mustChangePassword"`
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
