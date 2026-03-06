# Signal Session Bootstrap Plan

## Goal
Allow both users to start encrypted conversation immediately after relationship invite acceptance.

## Trigger
- Recipient accepts invite.
- Backend marks relationship membership active and ensures conversation exists.

## Realtime Notification
- If inviter app is active: send WebSocket event (e.g. `relationship_invite_accepted`).
- If inviter app is inactive: send push notification.
- Recipient app should also receive local success state and proceed with key prefetch.

## Post-Accept Client Flow (Both Sides)
1. Fetch counterpart prekey bundle:
   - `GET /v1/users/{target_user_id}/prekeys`
2. Build per-device Signal sessions (X3DH) using:
   - identity key
   - signed prekey
   - one-time prekey (if available)
3. Cache established sessions locally.
4. Either user can now send first encrypted message.

## First Message Behavior
- Sender encrypts per recipient device (one envelope per device).
- First outbound message to each device is typically a prekey message.
- Recipient establishes/updates session upon receive.

## Ongoing Messaging
- Use Double Ratchet state for each device-to-device session.
- Advance message keys per message.
- Handle out-of-order messages via skipped-key handling.

## Backend Responsibilities
- Store and serve public prekey bundles only.
- Consume one-time prekeys safely when served.
- Never process plaintext content (ciphertext only).
- Enforce auth + relationship membership checks for message routes.

## API/Realtime Checklist
- [ ] Ensure invite accept endpoint returns `relationship_space_id` and `conversation_id`.
- [ ] Emit `relationship_invite_accepted` WS event to both users.
- [ ] Send push fallback when user is offline.
- [ ] Keep `/v1/users/{user_id}/prekeys` auth-protected and audited.
- [ ] Add client retry/backoff for prekey fetch failures.

## Notes
- Session setup is device-to-device, not user-to-user.
- Both users should prefetch each other's prekeys right after acceptance for faster first message UX.
