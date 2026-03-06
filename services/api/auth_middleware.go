package api

import (
	"context"
	"net/http"
	"strings"

	"xend.chat/m/internal/auth"
	"xend.chat/m/pkg/httputil"
)

type authContextKey string

const claimsContextKey authContextKey = "access_claims"

func requireAuth(tokens *auth.TokenManager, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		raw := strings.TrimSpace(r.Header.Get("Authorization"))
		if raw == "" || !strings.HasPrefix(strings.ToLower(raw), "bearer ") {
			httputil.Error(w, http.StatusUnauthorized, "missing_token", "authorization bearer token is required")
			return
		}
		token := strings.TrimSpace(raw[7:])
		claims, err := tokens.ParseAccessToken(token)
		if err != nil {
			httputil.Error(w, http.StatusUnauthorized, "invalid_token", "access token is invalid")
			return
		}
		ctx := context.WithValue(r.Context(), claimsContextKey, claims)
		next(w, r.WithContext(ctx))
	}
}

func requireAuthMiddleware(tokens *auth.TokenManager) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			raw := strings.TrimSpace(r.Header.Get("Authorization"))
			if raw == "" || !strings.HasPrefix(strings.ToLower(raw), "bearer ") {
				httputil.Error(w, http.StatusUnauthorized, "missing_token", "authorization bearer token is required")
				return
			}
			token := strings.TrimSpace(raw[7:])
			claims, err := tokens.ParseAccessToken(token)
			if err != nil {
				httputil.Error(w, http.StatusUnauthorized, "invalid_token", "access token is invalid")
				return
			}
			ctx := context.WithValue(r.Context(), claimsContextKey, claims)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func claimsFromContext(ctx context.Context) (auth.AccessClaims, bool) {
	v := ctx.Value(claimsContextKey)
	claims, ok := v.(auth.AccessClaims)
	return claims, ok
}
