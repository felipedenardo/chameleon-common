package security

import "context"

// TokenVersionChecker checks if the token version is valid for a given user.
type TokenVersionChecker interface {
	// GetUserTokenVersion retrieves the current token version for the user.
	// It returns the version number and any error encountered.
	GetUserTokenVersion(ctx context.Context, userID string) (int, error)
}
