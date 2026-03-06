package notify

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"

	"golang.org/x/oauth2/google"
)

type PushMessage struct {
	Title string
	Body  string
	Data  map[string]string
}

type PushNotifier interface {
	SendToTokens(ctx context.Context, tokens []string, msg PushMessage) error
}

type NoopPushNotifier struct{}

func (NoopPushNotifier) SendToTokens(context.Context, []string, PushMessage) error { return nil }

type FCMNotifier struct {
	projectID   string
	credentials []byte
}

type serviceAccount struct {
	ProjectID string `json:"project_id"`
}

func NewFCMNotifier(credentialsPath string) (*FCMNotifier, error) {
	if credentialsPath == "" {
		return nil, fmt.Errorf("fcm is not configured")
	}
	creds, err := os.ReadFile(credentialsPath)
	if err != nil {
		return nil, err
	}

	var sa serviceAccount
	if err := json.Unmarshal(creds, &sa); err != nil {
		return nil, fmt.Errorf("parse firebase credentials: %w", err)
	}
	if sa.ProjectID == "" {
		return nil, fmt.Errorf("firebase project_id is missing in credentials json")
	}

	return &FCMNotifier{
		projectID:   sa.ProjectID,
		credentials: creds,
	}, nil
}

func (n *FCMNotifier) SendToTokens(ctx context.Context, tokens []string, msg PushMessage) error {
	if len(tokens) == 0 {
		return nil
	}

	cfg, err := google.JWTConfigFromJSON(n.credentials, "https://www.googleapis.com/auth/firebase.messaging")
	if err != nil {
		return err
	}
	client := cfg.Client(ctx)

	var sendErr error
	for _, token := range tokens {
		if token == "" {
			continue
		}
		if err := n.sendSingle(ctx, client, token, msg); err != nil {
			sendErr = err
		}
	}
	return sendErr
}

func (n *FCMNotifier) sendSingle(ctx context.Context, client *http.Client, token string, msg PushMessage) error {
	endpoint := fmt.Sprintf("https://fcm.googleapis.com/v1/projects/%s/messages:send", n.projectID)
	payload := map[string]any{
		"message": map[string]any{
			"token": token,
			"notification": map[string]string{
				"title": msg.Title,
				"body":  msg.Body,
			},
			"data": msg.Data,
		},
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return nil
	}
	rawBody, _ := io.ReadAll(resp.Body)
	return fmt.Errorf("fcm send failed: status=%d body=%s", resp.StatusCode, string(rawBody))
}
