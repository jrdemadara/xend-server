package auth

import (
	"context"
	"fmt"

	"google.golang.org/api/idtoken"
)

type GoogleClaims struct {
	Subject       string
	Email         string
	EmailVerified bool
	Name          string
}

type GoogleVerifier interface {
	Verify(ctx context.Context, idToken string) (GoogleClaims, error)
}

type IDTokenGoogleVerifier struct {
	Audience string
}

func (v *IDTokenGoogleVerifier) Verify(ctx context.Context, idTokenRaw string) (GoogleClaims, error) {
	if v.Audience == "" {
		return GoogleClaims{}, fmt.Errorf("google audience not configured")
	}
	payload, err := idtoken.Validate(ctx, idTokenRaw, v.Audience)
	if err != nil {
		return GoogleClaims{}, err
	}

	email, _ := payload.Claims["email"].(string)
	name, _ := payload.Claims["name"].(string)
	sub, _ := payload.Claims["sub"].(string)
	verified, _ := payload.Claims["email_verified"].(bool)

	if sub == "" {
		return GoogleClaims{}, fmt.Errorf("google subject claim missing")
	}

	return GoogleClaims{
		Subject:       sub,
		Email:         email,
		EmailVerified: verified,
		Name:          name,
	}, nil
}
