package db

import (
	"database/sql"
	"time"
)

// Queries wraps prepared SQL statements.
type Queries struct {
	db *sql.DB
}

// NewQueries creates a Queries instance.
func NewQueries(db *sql.DB) *Queries {
	return &Queries{db: db}
}

// ─── Users ───────────────────────────────────────────────────────────────────

func (q *Queries) CreateUser(id, handle string, pubKeyEd25519, pubKeyX25519, derivedPubKeyEd25519 []byte) error {
	_, err := q.db.Exec(
		`INSERT INTO users (id, handle, public_key_ed25519, public_key_x25519, derived_public_key_ed25519) VALUES (?, ?, ?, ?, ?)`,
		id, handle, pubKeyEd25519, pubKeyX25519, derivedPubKeyEd25519,
	)
	return err
}

func (q *Queries) GetUserByHandle(handle string) (*UserRow, error) {
	row := q.db.QueryRow(`SELECT id, handle, public_key_ed25519, public_key_x25519, derived_public_key_ed25519, created_at FROM users WHERE handle = ?`, handle)
	return scanUser(row)
}

func (q *Queries) GetUserByID(id string) (*UserRow, error) {
	row := q.db.QueryRow(`SELECT id, handle, public_key_ed25519, public_key_x25519, derived_public_key_ed25519, created_at FROM users WHERE id = ?`, id)
	return scanUser(row)
}

// GetMe returns user + recovery codes remaining.
func (q *Queries) GetMe(id string) (*UserRow, int, error) {
	u, err := q.GetUserByID(id)
	if err != nil {
		return nil, 0, err
	}
	var remaining int
	err = q.db.QueryRow(`SELECT COUNT(*) FROM encrypted_key_backups WHERE user_id = ? AND used = 0`, id).Scan(&remaining)
	if err != nil {
		return nil, 0, err
	}
	return u, remaining, nil
}

func (q *Queries) SearchUsers(query string, limit int) ([]UserRow, error) {
	rows, err := q.db.Query(`SELECT id, handle, public_key_ed25519, public_key_x25519, derived_public_key_ed25519, created_at FROM users WHERE handle LIKE ? LIMIT ?`, "%"+query+"%", limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var users []UserRow
	for rows.Next() {
		u, err := scanUser(rows)
		if err != nil {
			return nil, err
		}
		users = append(users, *u)
	}
	return users, rows.Err()
}

// ─── Sessions ─────────────────────────────────────────────────────────────────

func (q *Queries) CreateSession(tokenHash, userID string, expiresAt time.Time) error {
	_, err := q.db.Exec(
		`INSERT INTO sessions (token_hash, user_id, expires_at) VALUES (?, ?, ?)`,
		tokenHash, userID, expiresAt.UTC().Format(time.RFC3339),
	)
	return err
}

func (q *Queries) GetSessionByToken(tokenHash string) (*SessionRow, error) {
	row := q.db.QueryRow(`SELECT token_hash, user_id, created_at, expires_at FROM sessions WHERE token_hash = ?`, tokenHash)
	s := &SessionRow{}
	var createdAt, expiresAt string
	if err := row.Scan(&s.TokenHash, &s.UserID, &createdAt, &expiresAt); err != nil {
		return nil, err
	}
	s.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
	s.ExpiresAt, _ = time.Parse(time.RFC3339, expiresAt)
	return s, nil
}

func (q *Queries) DeleteSession(tokenHash string) error {
	_, err := q.db.Exec(`DELETE FROM sessions WHERE token_hash = ?`, tokenHash)
	return err
}

// ─── Messages ─────────────────────────────────────────────────────────────────

func (q *Queries) InsertMessage(id, senderID, recipientID, groupID string, ciphertext, ephemeralPubKey, nonce []byte, expiresAt *time.Time) error {
	var exp *string
	if expiresAt != nil {
		s := expiresAt.UTC().Format(time.RFC3339)
		exp = &s
	}
	_, err := q.db.Exec(
		`INSERT INTO messages (id, sender_id, recipient_id, group_id, ciphertext, ephemeral_public_key, nonce, expires_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		id, senderID, nullStr(recipientID), nullStr(groupID), ciphertext, ephemeralPubKey, nonce, exp,
	)
	return err
}

// GetDirectMessages fetches messages in a 1:1 conversation between two users.
func (q *Queries) GetDirectMessages(userID, otherUserID string, after, before *time.Time, limit int) ([]MessageRow, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}

	query := `SELECT id, sender_id, recipient_id, group_id, ciphertext, ephemeral_public_key, nonce, created_at, expires_at, read_at FROM messages WHERE ((sender_id = ? AND recipient_id = ?) OR (sender_id = ? AND recipient_id = ?))`
	var args []interface{}
	args = append(args, userID, otherUserID, otherUserID, userID)

	if after != nil {
		query += ` AND created_at > ? `
		args = append(args, after.UTC().Format(time.RFC3339))
	}
	if before != nil {
		query += ` AND created_at < ? `
		args = append(args, before.UTC().Format(time.RFC3339))
	}

	query += ` ORDER BY created_at DESC LIMIT ?`
	args = append(args, limit)

	rows, err := q.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var msgs []MessageRow
	for rows.Next() {
		m, err := scanMessage(rows)
		if err != nil {
			return nil, err
		}
		msgs = append(msgs, *m)
	}
	return msgs, rows.Err()
}

func (q *Queries) GetMessages(recipientID, groupID string, after, before *time.Time, limit int) ([]MessageRow, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}

	query := `SELECT id, sender_id, recipient_id, group_id, ciphertext, ephemeral_public_key, nonce, created_at, expires_at, read_at FROM messages WHERE `
	var args []interface{}

	if groupID != "" {
		query += `group_id = ? `
		args = append(args, groupID)
	} else if recipientID != "" {
		query += `recipient_id = ? `
		args = append(args, recipientID)
	} else {
		return nil, nil
	}

	if after != nil {
		query += `AND created_at > ? `
		args = append(args, after.UTC().Format(time.RFC3339))
	}
	if before != nil {
		query += `AND created_at < ? `
		args = append(args, before.UTC().Format(time.RFC3339))
	}

	query += `ORDER BY created_at DESC LIMIT ?`
	args = append(args, limit)

	rows, err := q.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var msgs []MessageRow
	for rows.Next() {
		m, err := scanMessage(rows)
		if err != nil {
			return nil, err
		}
		msgs = append(msgs, *m)
	}
	return msgs, rows.Err()
}

func (q *Queries) MarkConversationRead(userID, conversationWith string, upToMsgID string) error {
	if upToMsgID == "" {
		// Mark ALL messages from this conversation partner as read
		_, err := q.db.Exec(
			`UPDATE messages SET read_at = datetime('now') WHERE ((sender_id = ? AND recipient_id = ?) OR (sender_id = ? AND recipient_id = ?)) AND read_at IS NULL`,
			conversationWith, userID, userID, conversationWith,
		)
		return err
	}
	_, err := q.db.Exec(
		`UPDATE messages SET read_at = datetime('now') WHERE ((sender_id = ? AND recipient_id = ?) OR (sender_id = ? AND recipient_id = ?)) AND id <= ? AND read_at IS NULL`,
		conversationWith, userID, userID, conversationWith, upToMsgID,
	)
	return err
}

// GetConversationMessageIDs returns message IDs sent BY the user in a 1:1 conversation.
// Used for per-user message retention — only the sender's own messages are affected.
func (q *Queries) GetConversationMessageIDs(userID, otherUserID string) ([]string, error) {
	rows, err := q.db.Query(
		`SELECT id FROM messages WHERE sender_id = ? AND recipient_id = ?`,
		userID, otherUserID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// GetGroupMessageIDs returns all message IDs in a group.
func (q *Queries) GetGroupMessageIDs(groupID string) ([]string, error) {
	rows, err := q.db.Query(`SELECT id FROM messages WHERE group_id = ?`, groupID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// GetUserGroupMessageIDs returns message IDs sent BY a specific user in a group.
// Used for per-user group retention — only the sender's own messages are affected.
func (q *Queries) GetUserGroupMessageIDs(userID, groupID string) ([]string, error) {
	rows, err := q.db.Query(
		`SELECT id FROM messages WHERE sender_id = ? AND group_id = ?`,
		userID, groupID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	ids := make([]string, 0)
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// expiresInToSQL converts a shorthand duration to a SQLite datetime modifier.
// "1h" → "+1 hours", "24h" → "+24 hours", "7d" → "+7 days", etc.
func expiresInToSQL(s string) string {
	if len(s) < 2 {
		return "+" + s
	}
	num := s[:len(s)-1]
	switch s[len(s)-1:] {
	case "h":
		return "+" + num + " hours"
	case "d":
		return "+" + num + " days"
	default:
		return "+" + s
	}
}

func (q *Queries) UpdateRetention(msgIDs []string, expiresIn string) error {
	if len(msgIDs) == 0 {
		return nil
	}
	tx, err := q.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// Clear retention
	if expiresIn == "" {
		stmt, err := tx.Prepare(`UPDATE messages SET expires_at = NULL WHERE id = ?`)
		if err != nil {
			return err
		}
		defer stmt.Close()
		for _, id := range msgIDs {
			if _, err := stmt.Exec(id); err != nil {
				return err
			}
		}
		return tx.Commit()
	}

	// Set retention relative to each message's created_at so old messages
	// are deleted retroactively (not just now + duration).
	// Uses strftime to output RFC 3339 so it matches DeleteExpiredMessages.
	mod := expiresInToSQL(expiresIn)
	stmt, err := tx.Prepare(`UPDATE messages SET expires_at = strftime('%Y-%m-%dT%H:%M:%SZ', created_at, ?) WHERE id = ?`)
	if err != nil {
		return err
	}
	defer stmt.Close()
	for _, id := range msgIDs {
		if _, err := stmt.Exec(mod, id); err != nil {
			return err
		}
	}
	return tx.Commit()
}

type ConversationRow struct {
	UserID       string  `json:"user_id"`
	Handle       string  `json:"handle"`
	LastMessage  string  `json:"last_message_at"`
	UnreadCount  int     `json:"unread_count"`
	LastActivity string  `json:"last_active"`
	ExpiresAt    *string `json:"expires_at"`
}

func (q *Queries) GetConversations(userID string) ([]ConversationRow, error) {
	query := `
		SELECT
			u.id,
			u.handle,
			COALESCE(SUBSTR(HEX(m.ciphertext), 1, 32), '') AS last_msg,
			COALESCE((SELECT COUNT(*) FROM messages WHERE recipient_id = ? AND sender_id = u.id AND read_at IS NULL), 0) AS unread,
			COALESCE(MAX(m.created_at), '') AS last_active,
			MAX(CASE WHEN m.sender_id = ? THEN m.expires_at END) AS expires_at
		FROM users u
		INNER JOIN messages m ON (m.sender_id = u.id AND m.recipient_id = ?) OR (m.sender_id = ? AND m.recipient_id = u.id)
		WHERE u.id != ?
		GROUP BY u.id
		ORDER BY last_active DESC
	`
	rows, err := q.db.Query(query, userID, userID, userID, userID, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	convs := make([]ConversationRow, 0)
	for rows.Next() {
		var c ConversationRow
		if err := rows.Scan(&c.UserID, &c.Handle, &c.LastMessage, &c.UnreadCount, &c.LastActivity, &c.ExpiresAt); err != nil {
			return nil, err
		}
		normalizeTimestamp(&c.LastActivity)
		normalizeTimestamp(c.ExpiresAt)
		convs = append(convs, c)
	}
	return convs, rows.Err()
}

// ─── Groups ───────────────────────────────────────────────────────────────────

func (q *Queries) CreateGroup(id string, encryptedName, encryptedSymmetricKey []byte) error {
	_, err := q.db.Exec(
		`INSERT INTO groups (id, encrypted_name, encrypted_symmetric_key) VALUES (?, ?, ?)`,
		id, encryptedName, encryptedSymmetricKey,
	)
	return err
}

func (q *Queries) AddMember(groupID, userID string, encryptedGroupKey, encryptedMemberMetadata []byte) error {
	_, err := q.db.Exec(
		`INSERT INTO group_members (group_id, user_id, encrypted_group_key, encrypted_member_metadata) VALUES (?, ?, ?, ?)`,
		groupID, userID, encryptedGroupKey, encryptedMemberMetadata,
	)
	return err
}

func (q *Queries) RemoveMember(groupID, userID string) error {
	_, err := q.db.Exec(`DELETE FROM group_members WHERE group_id = ? AND user_id = ?`, groupID, userID)
	return err
}

func (q *Queries) GetGroupMemberIDs(groupID string) ([]string, error) {
	rows, err := q.db.Query(`SELECT user_id FROM group_members WHERE group_id = ?`, groupID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func (q *Queries) GetGroupMessages(groupID string, after, before *time.Time, limit int) ([]MessageRow, error) {
	return q.GetMessages("", groupID, after, before, limit)
}

func (q *Queries) GetUserGroups(userID string) ([]GroupRow, error) {
	rows, err := q.db.Query(`
		SELECT g.id, g.encrypted_name, g.encrypted_symmetric_key, g.created_at
		FROM groups g
		INNER JOIN group_members gm ON gm.group_id = g.id
		WHERE gm.user_id = ?
		ORDER BY g.created_at DESC
	`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var groups []GroupRow
	for rows.Next() {
		var g GroupRow
		if err := rows.Scan(&g.ID, &g.EncryptedName, &g.EncryptedSymmetricKey, &g.CreatedAt); err != nil {
			return nil, err
		}
		groups = append(groups, g)
	}
	return groups, rows.Err()
}

// GroupWithActivity extends GroupRow with the latest message timestamp.
type GroupWithActivity struct {
	ID                    string
	EncryptedName         []byte
	EncryptedSymmetricKey []byte
	CreatedAt             string
	LastActive            string
}

// GetUserGroupsWithActivity returns groups the user is a member of,
// including the timestamp of the latest group message (or group creation time).
func (q *Queries) GetUserGroupsWithActivity(userID string) ([]GroupWithActivity, error) {
	query := `
		SELECT g.id, g.encrypted_name, g.encrypted_symmetric_key, g.created_at,
		       COALESCE(MAX(m.created_at), g.created_at) AS last_active
		FROM groups g
		INNER JOIN group_members gm ON gm.group_id = g.id
		LEFT JOIN messages m ON m.group_id = g.id
		WHERE gm.user_id = ?
		GROUP BY g.id
		ORDER BY last_active DESC
	`
	rows, err := q.db.Query(query, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var groups []GroupWithActivity
	for rows.Next() {
		var g GroupWithActivity
		if err := rows.Scan(&g.ID, &g.EncryptedName, &g.EncryptedSymmetricKey, &g.CreatedAt, &g.LastActive); err != nil {
			return nil, err
		}
		normalizeTimestamp(&g.LastActive)
		groups = append(groups, g)
	}
	return groups, rows.Err()
}

// ─── Files ────────────────────────────────────────────────────────────────────

func (q *Queries) InsertFile(id, uploaderID, encryptedBlobPath string, encryptedMetadata []byte, sizeBytes int64, expiresAt *time.Time) error {
	var exp *string
	if expiresAt != nil {
		s := expiresAt.UTC().Format(time.RFC3339)
		exp = &s
	}
	_, err := q.db.Exec(
		`INSERT INTO files (id, uploader_id, encrypted_blob_path, encrypted_metadata, size_bytes, expires_at) VALUES (?, ?, ?, ?, ?, ?)`,
		id, uploaderID, encryptedBlobPath, encryptedMetadata, sizeBytes, exp,
	)
	return err
}

func (q *Queries) GetFile(id string) (*FileRow, error) {
	row := q.db.QueryRow(`SELECT id, uploader_id, encrypted_blob_path, encrypted_metadata, size_bytes, created_at, expires_at FROM files WHERE id = ?`, id)
	f := &FileRow{}
	var createdAt string
	var expiresAt *string
	if err := row.Scan(&f.ID, &f.UploaderID, &f.EncryptedBlobPath, &f.EncryptedMetadata, &f.SizeBytes, &createdAt, &expiresAt); err != nil {
		return nil, err
	}
	f.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
	if expiresAt != nil {
		t, _ := time.Parse(time.RFC3339, *expiresAt)
		f.ExpiresAt = &t
	}
	return f, nil
}

func (q *Queries) DeleteFile(id string) error {
	_, err := q.db.Exec(`DELETE FROM files WHERE id = ?`, id)
	return err
}

// ─── Recovery ─────────────────────────────────────────────────────────────────

func (q *Queries) InsertRecoveryBackup(userID, recoveryCodeHash string, encryptedPrivateKey, salt, authSalt []byte) error {
	_, err := q.db.Exec(
		`INSERT INTO encrypted_key_backups (user_id, recovery_code_hash, encrypted_private_key, salt, auth_salt) VALUES (?, ?, ?, ?, ?)`,
		userID, recoveryCodeHash, encryptedPrivateKey, salt, authSalt,
	)
	return err
}

func (q *Queries) GetRecoveryBackup(userID, recoveryCodeHash string) ([]byte, []byte, error) {
	var encryptedKey, salt []byte
	err := q.db.QueryRow(
		`SELECT encrypted_private_key, salt FROM encrypted_key_backups WHERE user_id = ? AND recovery_code_hash = ? AND used = 0`,
		userID, recoveryCodeHash,
	).Scan(&encryptedKey, &salt)
	if err != nil {
		return nil, nil, err
	}
	return encryptedKey, salt, nil
}

func (q *Queries) MarkCodeUsed(userID, recoveryCodeHash string) error {
	_, err := q.db.Exec(`UPDATE encrypted_key_backups SET used = 1 WHERE user_id = ? AND recovery_code_hash = ?`, userID, recoveryCodeHash)
	return err
}

// GetUnusedRecoveryBackups returns all unused recovery backup entries for a user.
// Used for constant-time comparison in the recover handler.
func (q *Queries) GetUnusedRecoveryBackups(userID string) ([]RecoveryBackupRow, error) {
	rows, err := q.db.Query(
		`SELECT recovery_code_hash, encrypted_private_key, salt, auth_salt FROM encrypted_key_backups WHERE user_id = ? AND used = 0`,
		userID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var backups []RecoveryBackupRow
	for rows.Next() {
		var b RecoveryBackupRow
		if err := rows.Scan(&b.RecoveryCodeHash, &b.EncryptedPrivateKey, &b.Salt, &b.AuthSalt); err != nil {
			return nil, err
		}
		backups = append(backups, b)
	}
	return backups, rows.Err()
}

func (q *Queries) GetRecoveryCodesRemaining(userID string) (int, error) {
	var count int
	err := q.db.QueryRow(`SELECT COUNT(*) FROM encrypted_key_backups WHERE user_id = ? AND used = 0`, userID).Scan(&count)
	return count, err
}

// ─── Per-User Retention ───────────────────────────────────────────────────────

// SetUserRetention stores a user's retention preference for a conversation.
// expiresIn: "1h", "7d", etc. Empty string clears the retention.
func (q *Queries) SetUserRetention(userID, targetID, targetType, expiresIn string) error {
	_, err := q.db.Exec(`
		INSERT INTO user_retention (user_id, target_id, target_type, expires_in)
		VALUES (?, ?, ?, ?)
		ON CONFLICT(user_id, target_id, target_type)
		DO UPDATE SET expires_in = excluded.expires_in, updated_at = datetime('now')
	`, userID, targetID, targetType, expiresIn)
	return err
}

// GetUserRetention returns the user's retention setting for a conversation.
// Returns empty string if no retention is set.
func (q *Queries) GetUserRetention(userID, targetID, targetType string) (string, error) {
	var expiresIn string
	err := q.db.QueryRow(
		`SELECT expires_in FROM user_retention WHERE user_id = ? AND target_id = ? AND target_type = ?`,
		userID, targetID, targetType,
	).Scan(&expiresIn)
	if err == sql.ErrNoRows {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return expiresIn, nil
}

// GetDirectMessagesWithRetention fetches messages in a 1:1 conversation,
// filtering out messages that have expired per the user's retention setting.
// retentionMod is a SQLite modifier like "-1 hours" or "" for no filter.
func (q *Queries) GetDirectMessagesWithRetention(userID, otherUserID string, after, before *time.Time, limit int, retentionMod string) ([]MessageRow, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}

	query := `SELECT id, sender_id, recipient_id, group_id, ciphertext, ephemeral_public_key, nonce, created_at, expires_at, read_at FROM messages WHERE ((sender_id = ? AND recipient_id = ?) OR (sender_id = ? AND recipient_id = ?))`
	var args []interface{}
	args = append(args, userID, otherUserID, otherUserID, userID)

	if retentionMod != "" {
		query += ` AND created_at >= datetime('now', ?) `
		args = append(args, retentionMod)
	}
	if after != nil {
		query += ` AND created_at > ? `
		args = append(args, after.UTC().Format(time.RFC3339))
	}
	if before != nil {
		query += ` AND created_at < ? `
		args = append(args, before.UTC().Format(time.RFC3339))
	}

	query += ` ORDER BY created_at DESC LIMIT ?`
	args = append(args, limit)

	rows, err := q.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var msgs []MessageRow
	for rows.Next() {
		m, err := scanMessage(rows)
		if err != nil {
			return nil, err
		}
		msgs = append(msgs, *m)
	}
	return msgs, rows.Err()
}

// GetGroupMessagesWithRetention fetches messages in a group,
// filtering out messages that have expired per the user's retention setting.
func (q *Queries) GetGroupMessagesWithRetention(groupID string, after, before *time.Time, limit int, retentionMod string) ([]MessageRow, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}

	query := `SELECT id, sender_id, recipient_id, group_id, ciphertext, ephemeral_public_key, nonce, created_at, expires_at, read_at FROM messages WHERE group_id = ?`
	var args []interface{}
	args = append(args, groupID)

	if retentionMod != "" {
		query += ` AND created_at >= datetime('now', ?) `
		args = append(args, retentionMod)
	}
	if after != nil {
		query += ` AND created_at > ? `
		args = append(args, after.UTC().Format(time.RFC3339))
	}
	if before != nil {
		query += ` AND created_at < ? `
		args = append(args, before.UTC().Format(time.RFC3339))
	}

	query += ` ORDER BY created_at DESC LIMIT ?`
	args = append(args, limit)

	rows, err := q.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var msgs []MessageRow
	for rows.Next() {
		m, err := scanMessage(rows)
		if err != nil {
			return nil, err
		}
		msgs = append(msgs, *m)
	}
	return msgs, rows.Err()
}

// ─── Cleanup ──────────────────────────────────────────────────────────────────

// DeleteExpiredMessages removes messages past their expires_at.
// This only affects messages with an explicit per-message TTL set via expires_in.
// Per-user retention filtering is handled separately on read (see GetDirectMessagesWithRetention).
// Uses RFC3339 comparison to match the format written by UpdateRetention.
func (q *Queries) DeleteExpiredMessages() (int64, error) {
	now := time.Now().UTC().Format(time.RFC3339)
	res, err := q.db.Exec(`DELETE FROM messages WHERE expires_at IS NOT NULL AND expires_at < ?`, now)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// DeleteExpiredFiles removes file metadata for expired files.
func (q *Queries) DeleteExpiredFiles() ([]string, error) {
	now := time.Now().UTC().Format(time.RFC3339)
	rows, err := q.db.Query(`SELECT id, encrypted_blob_path FROM files WHERE expires_at IS NOT NULL AND expires_at < ?`, now)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var paths []string
	for rows.Next() {
		var id, path string
		if err := rows.Scan(&id, &path); err != nil {
			return nil, err
		}
		paths = append(paths, path)
	}
	return paths, nil
}

// RemoveFileRecord removes a file record by path after blob deletion.
func (q *Queries) RemoveFileRecord(path string) error {
	_, err := q.db.Exec(`DELETE FROM files WHERE encrypted_blob_path = ?`, path)
	return err
}

// ─── Row types ────────────────────────────────────────────────────────────────

type UserRow struct {
	ID                      string `json:"id"`
	Handle                  string `json:"handle"`
	PublicKeyEd25519        []byte `json:"public_key_ed25519"`
	PublicKeyX25519         []byte `json:"public_key_x25519"`
	DerivedPublicKeyEd25519 []byte `json:"derived_public_key_ed25519"`
	CreatedAt               string `json:"created_at"`
}

type SessionRow struct {
	TokenHash string
	UserID    string
	CreatedAt time.Time
	ExpiresAt time.Time
}

type MessageRow struct {
	ID              string  `json:"id"`
	SenderID        string  `json:"sender_id"`
	RecipientID     *string `json:"recipient_id"`
	GroupID         *string `json:"group_id"`
	Ciphertext      []byte  `json:"ciphertext"`
	EphemeralPubKey []byte  `json:"ephemeral_public_key"`
	Nonce           []byte  `json:"nonce"`
	CreatedAt       string  `json:"created_at"`
	ExpiresAt       *string `json:"expires_at"`
	ReadAt          *string `json:"read_at"`
}

type GroupRow struct {
	ID                    string
	EncryptedName         []byte
	EncryptedSymmetricKey []byte
	CreatedAt             string
}

type FileRow struct {
	ID                string
	UploaderID        string
	EncryptedBlobPath string
	EncryptedMetadata []byte
	SizeBytes         int64
	CreatedAt         time.Time
	ExpiresAt         *time.Time
}

type RecoveryBackupRow struct {
	RecoveryCodeHash    string
	EncryptedPrivateKey []byte
	Salt                []byte
	AuthSalt            []byte
}

// ─── Helpers ──────────────────────────────────────────────────────────────────

func scanUser(s interface {
	Scan(dest ...interface{}) error
}) (*UserRow, error) {
	u := &UserRow{}
	if err := s.Scan(&u.ID, &u.Handle, &u.PublicKeyEd25519, &u.PublicKeyX25519, &u.DerivedPublicKeyEd25519, &u.CreatedAt); err != nil {
		return nil, err
	}
	return u, nil
}

// normalizeTimestamp converts SQLite datetime('now') format ("2026-06-14 05:36:00")
// to RFC 3339 ("2026-06-14T05:36:00Z") so JavaScript can parse it as UTC.
func normalizeTimestamp(s *string) {
	if s == nil || *s == "" {
		return
	}
	t := *s
	if len(t) == 19 && t[10] == ' ' {
		*s = t[:10] + "T" + t[11:] + "Z"
	}
}

func scanMessage(s interface {
	Scan(dest ...interface{}) error
}) (*MessageRow, error) {
	m := &MessageRow{}
	if err := s.Scan(&m.ID, &m.SenderID, &m.RecipientID, &m.GroupID, &m.Ciphertext, &m.EphemeralPubKey, &m.Nonce, &m.CreatedAt, &m.ExpiresAt, &m.ReadAt); err != nil {
		return nil, err
	}
	normalizeTimestamp(&m.CreatedAt)
	normalizeTimestamp(m.ExpiresAt)
	normalizeTimestamp(m.ReadAt)
	return m, nil
}

func nullStr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
