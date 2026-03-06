package auth

import "context"

type VerificationMailer interface {
	SendVerificationEmail(ctx context.Context, toEmail, token string) error
}

type VerificationEmailEnqueuer interface {
	EnqueueVerificationEmail(ctx context.Context, email, token string) error
}
