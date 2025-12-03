package security

import "context"

type BlacklistTokenChecker interface {
	IsTokenBlacklisted(ctx context.Context, jti string) (bool, error)
}
