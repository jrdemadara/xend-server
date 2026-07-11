package auth

import "context"

type accessClaimsContextKey string

const claimsContextKey accessClaimsContextKey = "access_claims"

func WithAccessClaims(ctx context.Context, claims AccessClaims) context.Context {
	return context.WithValue(ctx, claimsContextKey, claims)
}

func AccessClaimsFromContext(ctx context.Context) (AccessClaims, bool) {
	v := ctx.Value(claimsContextKey)
	claims, ok := v.(AccessClaims)
	return claims, ok
}
