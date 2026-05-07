package auth

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

type MessageRecord struct {
	MessageID       string
	ConversationID  string
	SenderUserID    string
	SenderDeviceID  string
	ClientMessageID string
	MessageType     string
	Ciphertext      string
	ReplyToMessageID *string
	SenderTimestamp *time.Time
	CreatedAt       time.Time
	ReceiptUserID   *string
	ReceiptStatus   *string
	DeliveredAt     *time.Time
	ReadAt          *time.Time
}

func (r *Repository) CreateConversationMessage(
	ctx context.Context,
	userID string,
	deviceID string,
	conversationID string,
	clientMessageID string,
	messageType string,
	ciphertext string,
	senderTimestamp *time.Time,
	replyToMessageID *string,
) (MessageRecord, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return MessageRecord{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	message, err := r.insertConversationMessageTx(ctx, tx, userID, deviceID, conversationID, clientMessageID, messageType, ciphertext, senderTimestamp, replyToMessageID)
	if err == nil {
		recipients, recipientErr := r.listConversationRecipientDevicesTx(ctx, tx, conversationID, userID)
		if recipientErr != nil {
			return MessageRecord{}, recipientErr
		}
		if receiptErr := r.insertMessageReceiptsTx(ctx, tx, message.MessageID, recipients); receiptErr != nil {
			return MessageRecord{}, receiptErr
		}
		if commitErr := tx.Commit(ctx); commitErr != nil {
			return MessageRecord{}, commitErr
		}
		return message, nil
	}
	if !isMessageUniqueViolation(err) {
		return MessageRecord{}, err
	}
	return r.GetConversationMessageByClientID(ctx, deviceID, clientMessageID)
}

type conversationRecipientDevice struct {
	UserID   string
	DeviceID string
}

func (r *Repository) insertConversationMessageTx(
	ctx context.Context,
	tx pgx.Tx,
	userID string,
	deviceID string,
	conversationID string,
	clientMessageID string,
	messageType string,
	ciphertext string,
	senderTimestamp *time.Time,
	replyToMessageID *string,
) (MessageRecord, error) {
	var item MessageRecord
	err := tx.QueryRow(ctx, `
		INSERT INTO messages (
			conversation_id,
			sender_user_id,
			sender_device_id,
			client_message_id,
			message_type,
			ciphertext,
			sender_timestamp,
			reply_to_message_id
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		RETURNING id, conversation_id, sender_user_id, sender_device_id, client_message_id, message_type, ciphertext, reply_to_message_id, sender_timestamp, created_at
	`, conversationID, userID, deviceID, clientMessageID, messageType, ciphertext, senderTimestamp, replyToMessageID).Scan(
		&item.MessageID,
		&item.ConversationID,
		&item.SenderUserID,
		&item.SenderDeviceID,
		&item.ClientMessageID,
		&item.MessageType,
		&item.Ciphertext,
		&item.ReplyToMessageID,
		&item.SenderTimestamp,
		&item.CreatedAt,
	)
	if err != nil {
		return MessageRecord{}, err
	}
	return item, nil
}

func (r *Repository) listConversationRecipientDevicesTx(ctx context.Context, tx pgx.Tx, conversationID, excludeUserID string) ([]conversationRecipientDevice, error) {
	rows, err := tx.Query(ctx, `
		SELECT DISTINCT d.user_id, d.id
		FROM conversations c
		JOIN relationship_space_members rsm
		  ON rsm.relationship_space_id = c.relationship_space_id
		JOIN devices d
		  ON d.user_id = rsm.user_id
		WHERE c.id = $1
		  AND rsm.membership_status = 'active'
		  AND rsm.user_id <> $2
		  AND d.is_active = TRUE
		  AND d.revoked_at IS NULL
	`, conversationID, excludeUserID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]conversationRecipientDevice, 0, 2)
	for rows.Next() {
		var item conversationRecipientDevice
		if err := rows.Scan(&item.UserID, &item.DeviceID); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return items, nil
}

func (r *Repository) insertMessageReceiptsTx(ctx context.Context, tx pgx.Tx, messageID string, recipients []conversationRecipientDevice) error {
	for _, recipient := range recipients {
		if _, err := tx.Exec(ctx, `
			INSERT INTO message_receipts (
				message_id,
				recipient_user_id,
				recipient_device_id,
				sent_at,
				status
			)
			VALUES ($1, $2, $3, now(), 'sent')
			ON CONFLICT (message_id, recipient_device_id) DO NOTHING
		`, messageID, recipient.UserID, recipient.DeviceID); err != nil {
			return err
		}
	}
	return nil
}

func (r *Repository) GetConversationMessageByClientID(ctx context.Context, deviceID, clientMessageID string) (MessageRecord, error) {
	var item MessageRecord
	err := r.db.QueryRow(ctx, `
		SELECT id,
		       conversation_id,
		       sender_user_id,
		       sender_device_id,
		       client_message_id,
		       message_type,
		       ciphertext,
		       reply_to_message_id,
		       sender_timestamp,
		       created_at
		FROM messages
		WHERE sender_device_id = $1
		  AND client_message_id = $2
		LIMIT 1
	`, deviceID, clientMessageID).Scan(
		&item.MessageID,
		&item.ConversationID,
		&item.SenderUserID,
		&item.SenderDeviceID,
		&item.ClientMessageID,
		&item.MessageType,
		&item.Ciphertext,
		&item.ReplyToMessageID,
		&item.SenderTimestamp,
		&item.CreatedAt,
	)
	if err != nil {
		return MessageRecord{}, err
	}
	return item, nil
}

func (r *Repository) ListConversationMessages(ctx context.Context, userID, conversationID string, limit int, before *time.Time) ([]MessageRecord, error) {
	if limit <= 0 || limit > 200 {
		limit = 100
	}

	rows, err := r.db.Query(ctx, `
		SELECT m.id,
		       m.conversation_id,
		       m.sender_user_id,
		       m.sender_device_id,
		       m.client_message_id,
		       m.message_type,
		       m.ciphertext,
		       m.reply_to_message_id,
		       m.sender_timestamp,
		       m.created_at,
		       receipt.recipient_user_id,
		       receipt.status,
		       receipt.delivered_at,
		       receipt.read_at
		FROM messages m
		JOIN conversations c
		  ON c.id = m.conversation_id
		JOIN relationship_space_members rsm
		  ON rsm.relationship_space_id = c.relationship_space_id
		LEFT JOIN LATERAL (
			SELECT mr.recipient_user_id,
			       mr.status,
			       mr.delivered_at,
			       mr.read_at
			FROM message_receipts mr
			WHERE mr.message_id = m.id
			  AND (
				(m.sender_user_id = $2 AND mr.recipient_user_id <> $2)
				OR
				(m.sender_user_id <> $2 AND mr.recipient_user_id = $2)
			  )
			ORDER BY mr.created_at DESC
			LIMIT 1
		) receipt ON TRUE
		WHERE m.conversation_id = $1
		  AND rsm.user_id = $2
		  AND rsm.membership_status = 'active'
		  AND ($3::timestamptz IS NULL OR m.created_at < $3)
		ORDER BY m.created_at ASC
		LIMIT $4
	`, conversationID, userID, before, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]MessageRecord, 0)
	for rows.Next() {
		var item MessageRecord
		if err := rows.Scan(
			&item.MessageID,
			&item.ConversationID,
			&item.SenderUserID,
			&item.SenderDeviceID,
			&item.ClientMessageID,
			&item.MessageType,
			&item.Ciphertext,
			&item.ReplyToMessageID,
			&item.SenderTimestamp,
			&item.CreatedAt,
			&item.ReceiptUserID,
			&item.ReceiptStatus,
			&item.DeliveredAt,
			&item.ReadAt,
		); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return items, nil
}

func (r *Repository) ListMessagesForUserSince(ctx context.Context, userID string, since *time.Time, limit int) ([]MessageRecord, error) {
	if limit <= 0 || limit > 500 {
		limit = 200
	}

	rows, err := r.db.Query(ctx, `
		SELECT m.id,
		       m.conversation_id,
		       m.sender_user_id,
		       m.sender_device_id,
		       m.client_message_id,
		       m.message_type,
		       m.ciphertext,
		       m.reply_to_message_id,
		       m.sender_timestamp,
		       m.created_at,
		       receipt.recipient_user_id,
		       receipt.status,
		       receipt.delivered_at,
		       receipt.read_at
		FROM messages m
		JOIN conversations c
		  ON c.id = m.conversation_id
		JOIN relationship_space_members rsm
		  ON rsm.relationship_space_id = c.relationship_space_id
		LEFT JOIN LATERAL (
			SELECT mr.recipient_user_id,
			       mr.status,
			       mr.delivered_at,
			       mr.read_at
			FROM message_receipts mr
			WHERE mr.message_id = m.id
			  AND (
				(m.sender_user_id = $1 AND mr.recipient_user_id <> $1)
				OR
				(m.sender_user_id <> $1 AND mr.recipient_user_id = $1)
			  )
			ORDER BY mr.created_at DESC
			LIMIT 1
		) receipt ON TRUE
		WHERE rsm.user_id = $1
		  AND rsm.membership_status = 'active'
		  AND ($2::timestamptz IS NULL OR m.created_at > $2)
		ORDER BY m.created_at ASC
		LIMIT $3
	`, userID, since, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]MessageRecord, 0)
	for rows.Next() {
		var item MessageRecord
		if err := rows.Scan(
			&item.MessageID,
			&item.ConversationID,
			&item.SenderUserID,
			&item.SenderDeviceID,
			&item.ClientMessageID,
			&item.MessageType,
			&item.Ciphertext,
			&item.ReplyToMessageID,
			&item.SenderTimestamp,
			&item.CreatedAt,
			&item.ReceiptUserID,
			&item.ReceiptStatus,
			&item.DeliveredAt,
			&item.ReadAt,
		); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return items, nil
}

func (r *Repository) ListConversationRecipientUserIDs(ctx context.Context, conversationID, excludeUserID string) ([]string, error) {
	rows, err := r.db.Query(ctx, `
		SELECT DISTINCT rsm.user_id
		FROM conversations c
		JOIN relationship_space_members rsm
		  ON rsm.relationship_space_id = c.relationship_space_id
		WHERE c.id = $1
		  AND rsm.membership_status = 'active'
		  AND rsm.user_id <> $2
	`, conversationID, excludeUserID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]string, 0)
	for rows.Next() {
		var userID string
		if err := rows.Scan(&userID); err != nil {
			return nil, err
		}
		items = append(items, userID)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return items, nil
}

func (r *Repository) ListRelatedUserIDs(ctx context.Context, userID string) ([]string, error) {
	rows, err := r.db.Query(ctx, `
		SELECT DISTINCT other.user_id
		FROM relationship_space_members me
		JOIN relationship_space_members other
		  ON other.relationship_space_id = me.relationship_space_id
		WHERE me.user_id = $1
		  AND me.membership_status = 'active'
		  AND other.membership_status = 'active'
		  AND other.user_id <> $1
	`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]string, 0, 4)
	for rows.Next() {
		var relatedUserID string
		if err := rows.Scan(&relatedUserID); err != nil {
			return nil, err
		}
		items = append(items, relatedUserID)
	}
	return items, rows.Err()
}

func (r *Repository) MarkMessagesDeliveredForDevice(ctx context.Context, userID, deviceID string) ([]string, error) {
	rows, err := r.db.Query(ctx, `
		WITH updated AS (
			UPDATE message_receipts mr
			SET status = 'delivered',
			    delivered_at = COALESCE(mr.delivered_at, now())
			FROM messages m
			JOIN conversations c
			  ON c.id = m.conversation_id
			JOIN relationship_space_members rsm
			  ON rsm.relationship_space_id = c.relationship_space_id
			WHERE mr.message_id = m.id
			  AND mr.recipient_user_id = $1
			  AND mr.recipient_device_id = $2
			  AND rsm.user_id = $1
			  AND rsm.membership_status = 'active'
			  AND mr.status = 'sent'
			RETURNING m.sender_user_id
		)
		SELECT DISTINCT sender_user_id FROM updated
	`, userID, deviceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	senderIDs := make([]string, 0, 2)
	for rows.Next() {
		var senderID string
		if err := rows.Scan(&senderID); err != nil {
			return nil, err
		}
		senderIDs = append(senderIDs, senderID)
	}
	return senderIDs, rows.Err()
}

func (r *Repository) MarkConversationMessagesRead(ctx context.Context, userID, deviceID, conversationID string) ([]string, error) {
	rows, err := r.db.Query(ctx, `
		WITH updated AS (
			UPDATE message_receipts mr
			SET status = 'read',
			    delivered_at = COALESCE(mr.delivered_at, now()),
			    read_at = COALESCE(mr.read_at, now())
			FROM messages m
			JOIN conversations c
			  ON c.id = m.conversation_id
			JOIN relationship_space_members rsm
			  ON rsm.relationship_space_id = c.relationship_space_id
			WHERE mr.message_id = m.id
			  AND m.conversation_id = $3
			  AND mr.recipient_user_id = $1
			  AND mr.recipient_device_id = $2
			  AND rsm.user_id = $1
			  AND rsm.membership_status = 'active'
			  AND mr.status IN ('sent', 'delivered')
			RETURNING m.sender_user_id
		)
		SELECT DISTINCT sender_user_id FROM updated
	`, userID, deviceID, conversationID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	senderIDs := make([]string, 0, 2)
	for rows.Next() {
		var senderID string
		if err := rows.Scan(&senderID); err != nil {
			return nil, err
		}
		senderIDs = append(senderIDs, senderID)
	}
	return senderIDs, rows.Err()
}

func isMessageUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		return false
	}
	return pgErr.Code == "23505" && pgErr.ConstraintName == "messages_sender_device_unique_client_msg"
}
