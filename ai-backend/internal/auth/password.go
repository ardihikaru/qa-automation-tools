package auth

import (
	"errors"

	"golang.org/x/crypto/bcrypt"
)

// bcryptCost matches the cost used by the taskmgr reference app.
const bcryptCost = 12

// ErrInvalidCredentials is returned by Check when the password does not match.
// Callers should not surface the underlying bcrypt error to users.
var ErrInvalidCredentials = errors.New("invalid credentials")

// Hash returns a bcrypt hash of plain at cost 12.
func Hash(plain string) (string, error) {
	h, err := bcrypt.GenerateFromPassword([]byte(plain), bcryptCost)
	if err != nil {
		return "", err
	}
	return string(h), nil
}

// Check compares hash and plain. Returns ErrInvalidCredentials on mismatch.
func Check(hash, plain string) error {
	if err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(plain)); err != nil {
		return ErrInvalidCredentials
	}
	return nil
}
