package queue

import (
	"context"
	"encoding/json"

	"github.com/hibiken/asynq"
)

type VerificationEmailEnqueuer struct {
	client *asynq.Client
}

func NewVerificationEmailEnqueuer(client *asynq.Client) *VerificationEmailEnqueuer {
	return &VerificationEmailEnqueuer{client: client}
}

func (q *VerificationEmailEnqueuer) EnqueueVerificationEmail(ctx context.Context, email, token string) error {
	payload, err := json.Marshal(SendVerificationEmailPayload{Email: email, Token: token})
	if err != nil {
		return err
	}
	task := asynq.NewTask(TaskTypeSendVerificationEmail, payload, asynq.MaxRetry(5))
	_, err = q.client.EnqueueContext(ctx, task)
	return err
}

func (q *VerificationEmailEnqueuer) EnqueueRelationshipInviteEmail(ctx context.Context, email, inviterDisplayName, inviterIdentifier, note string) error {
	payload, err := json.Marshal(SendRelationshipInviteEmailPayload{
		Email:              email,
		InviterDisplayName: inviterDisplayName,
		InviterIdentifier:  inviterIdentifier,
		Note:               note,
	})
	if err != nil {
		return err
	}
	task := asynq.NewTask(TaskTypeSendRelationshipInviteEmail, payload, asynq.MaxRetry(5))
	_, err = q.client.EnqueueContext(ctx, task)
	return err
}
