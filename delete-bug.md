# PRD: Auto-Delete Retention System — Permanent Hide + Mutual Deletion

## Summary

Fix the auto-delete retention system so that:
1. Once messages are hidden by a retention window, they stay hidden permanently — even if the user later widens the retention window.
2. When both participants in a 1:1 conversation have the same message hidden, that message is physically deleted from the database forever.
3. All internal code language changes from "delete" to "hide" to accurately reflect behavior. Only the user-facing UI keeps "Auto Delete" terminology.

## Current Behavior (Bug)

When a user changes retention from a narrow window (e.g., "1 hour") to a wider window (e.g., "24 hours"), the system rewrites `expires_at` on ALL messages. Messages that fell outside the narrow window and were hidden get a new future `expires_at` and re-appear in the UI. This violates the core requirement that expired messages must stay hidden forever.

Additionally, there is no mechanism for physical deletion when both participants in a 1:1 chat have independently hidden the same message.

## Available Retention Options

The retention picker offers these durations:
- 1 hour
- 24 hours
- 7 days
- 30 days
- 90 days
- Never (no retention)

## Requirements

### R1: Permanent Hide on Retention Change

When a user sets retention to duration `D`:

1. Query all messages in the conversation for this user
2. Split into two groups:
   - **Already expired**: `created_at < now() - D` → insert into `message_deletions` (permanently hidden from this user)
   - **Still in window**: `created_at >= now() - D` → set `expires_at = created_at + D` (cleanup goroutine handles later)
3. Never modify `expires_at` on already-expired messages
4. If user changes from "some duration" to "Never", messages already in `message_deletions` must remain hidden

### R2: Permanent Hide — 1:1 Messages

Applies R1 logic to direct messages. Query scope:
WHERE (sender_id = ? AND recipient_id = ?)
OR (sender_id = ? AND recipient_id = ?)


### R3: Permanent Hide — Group Messages

Applies R1 logic to group messages. Query scope:
WHERE group_id = ? AND sender_id = ?


### R4: Exclude Hidden Messages from All Retrieval Queries

The following queries must filter out messages in `message_deletions` for the requesting user:
- `GetDirectMessagesWithRetention`
- Group message retrieval queries
- `GetConversationMessageIDs`
- `GetUserGroupMessageIDs`

Use pattern: `LEFT JOIN message_deletions md ON m.id = md.message_id AND md.user_id = ? WHERE md.message_id IS NULL`

### R5: Mutual Permanent Deletion — 1:1 Conversations Only

When both participants in a 1:1 conversation have independently hidden the same message, physically DELETE it from the `messages` table.

**Trigger point**: Whenever a message is inserted into `message_deletions` for a user in a 1:1 conversation, check if the other participant has also hidden it.

**Check query**:
```sql
SELECT COUNT(*) FROM message_deletions 
WHERE message_id = ?

If count equals 2 (both participants have hidden it):

DELETE the row from messages

Clean up both entries in message_deletions

Group exclusion: This logic applies ONLY to 1:1 conversations (recipient_id IS NOT NULL). Group messages are never physically deleted based on two users' settings — groups have more than two participants.

R6: Cleanup Goroutine Update
Rename DeleteExpiredMessages to HideExpiredMessages. Update it to:

Move expired messages into message_deletions (per-user) instead of or in addition to hard-deleting them

After hiding, trigger the R5 mutual-deletion check for any affected 1:1 conversations


if count equals 2 (both participants have hidden it):

DELETE the row from messages

Clean up both entries in message_deletions

Group exclusion: This logic applies ONLY to 1:1 conversations (recipient_id IS NOT NULL). Group messages are never physically deleted based on two users' settings — groups have more than two participants.

R6: Cleanup Goroutine Update
Rename DeleteExpiredMessages to HideExpiredMessages. Update it to:

Move expired messages into message_deletions (per-user) instead of or in addition to hard-deleting them

After hiding, trigger the R5 mutual-deletion check for any affected 1:1 conversations

R7: Language Change — Internal Code Only
Rename all internal references from "delete" to "hide":

Current	Change To
DeleteExpiredMessages()	HideExpiredMessages()
"deleted" in log messages	"hidden" (unless referring to physical deletion)
deleted variable names	hidden
Code comments referencing deletion	Reference "hiding from view"
Do NOT change:

message_deletions table name (avoid unnecessary migration)

User-facing UI text (keep "Auto Delete" and "retention")

API response fields or endpoint paths

Do add:

Update all code comments on message_deletions table to clarify it means "hidden from user's view" — not necessarily physically deleted

Add a clearly named function like PermanentlyDeleteMutuallyHiddenMessages() for the physical deletion logic

Example Scenarios
Retention options: 1 hour, 24 hours, 7 days, 30 days, 90 days, Never

User1 Setting	User2 Setting	Messages older than User2's window	Messages between the two windows
1 hour	24 hours	Physically deleted (both hidden)	Hidden from User1, visible to User2
1 hour	7 days	Physically deleted (both hidden)	Hidden from User1, visible to User2
24 hours	24 hours	Physically deleted (both hidden)	N/A
30 days	90 days	Physically deleted (both hidden)	Hidden from User1, visible to User2
7 days	7 days	Physically deleted (both hidden)	N/A
Never	24 hours	Hidden from User2 only	N/A
1 hour	Never	Hidden from User1 only	N/A
30 days	30 days	Physically deleted (both hidden)	N/A
Key principle: The narrower retention window determines what's hidden for that user. Messages hidden by both users get physically deleted. The wider window participant can still see messages that only the narrower window participant has hidden.

Files Likely Affected
server/handlers/messages.go — updateRetention handler

server/db/queries.go — message retrieval queries, new mutual-deletion queries

server/cleanup/cleanup.go (or wherever DeleteExpiredMessages lives) — cleanup goroutine

Database schema — message_deletions table (already exists, verify structure)

Acceptance Criteria
User sets "1 hour" retention → messages older than 1 hour disappear from UI

User changes to "24 hours" → messages older than 1 hour do NOT re-appear

User changes to "Never" → previously hidden messages remain hidden

In 1:1 chat where User1 has "1 hour" and User2 has "24 hours": messages older than 24 hours are physically deleted from messages table

In 1:1 chat where User1 has "7 days" and User2 has "30 days": messages older than 30 days are physically deleted; messages between 7-30 days are hidden from User1 only

In group chat: no physical deletion ever occurs based on two users' settings

UI continues to show "Auto Delete" as the feature label

Internal function names and comments use "hide" terminology


---


