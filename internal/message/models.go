package message

import (
	"errors"
	"time"
)

type Record struct {
	MessageID        string
	ConversationID   string
	SenderUserID     string
	SenderDeviceID   string
	ClientMessageID  string
	MessageType      string
	Ciphertext       string
	ReplyToMessageID *string
	SenderTimestamp  *time.Time
	CreatedAt        time.Time
	ReceiptUserID    *string
	ReceiptStatus    *string
	DeliveredAt      *time.Time
	ReadAt           *time.Time
}

type SendRequest struct {
	ClientMessageID  string  `json:"client_message_id"`
	MessageType      string  `json:"message_type"`
	Ciphertext       string  `json:"ciphertext"`
	ReplyToMessageID *string `json:"reply_to_message_id"`
	SenderTimestamp  *int64  `json:"sender_timestamp"`
}

var ErrNotFound = errors.New("message not found")
