-- +goose Up
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION enforce_message_sender_membership()
RETURNS TRIGGER AS $$
BEGIN
    IF NOT EXISTS (
        SELECT 1
        FROM conversations c
        JOIN relationship_space_members rsm
          ON rsm.relationship_space_id = c.relationship_space_id
        WHERE c.id = NEW.conversation_id
          AND rsm.user_id = NEW.sender_user_id
          AND rsm.membership_status = 'active'
    ) THEN
        RAISE EXCEPTION 'sender is not an active member of the relationship space for this conversation';
    END IF;

    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

CREATE TRIGGER messages_enforce_sender_membership
BEFORE INSERT ON messages
FOR EACH ROW
EXECUTE FUNCTION enforce_message_sender_membership();

-- +goose Down
DROP TRIGGER IF EXISTS messages_enforce_sender_membership ON messages;
DROP FUNCTION IF EXISTS enforce_message_sender_membership;
