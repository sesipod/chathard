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
	_, err := q.db.Exec(
		`UPDATE messages SET read_at = datetime('now') WHERE ((sender_id = ? AND recipient_id = ?) OR (sender_id = ? AND recipient_id = ?)) AND id <= ? AND read_at IS NULL`,
		conversationWith, userID, userID, conversationWith, upToMsgID,
	)
	return err
}

func (q *Queries) UpdateRetention(msgIDs []string, expiresAt *time.Time) error {
	tx, err := q.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare(`UPDATE messages SET expires_at = ? WHERE id = ?`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, id := range msgIDs {
		var exp *string
		if expiresAt != nil {
			s := expiresAt.UTC().Format(time.RFC3339)
			exp = &s
		}
		if _, err := stmt.Exec(exp, id); err != nil {
			return err
		}
	}
	return tx.Commit()
}

type ConversationRow struct {
	UserID       string `json:"user_id"`
	Handle       string `json:"handle"`
	LastMessage  string `json:"last_message_at"`
	UnreadCount  int    `json:"unread_count"`
	LastActivity string `json:"last_active"`
}

func (q *Queries) GetConversations(userID string) ([]ConversationRow, error) {
	query := `
		SELECT
			u.id,
			u.handle,
			COALESCE(SUBSTR(HEX(m.ciphertext), 1, 32), '') AS last_msg,
			COALESCE((SELECT COUNT(*) FROM messages WHERE recipient_id = ? AND sender_id = u.id AND read_at IS NULL), 0) AS unread,
			COALESCE(MAX(m.created_at), '') AS last_active
		FROM users u
		INNER JOIN messages m ON (m.sender_id = u.id AND m.recipient_id = ?) OR (m.sender_id = ? AND m.recipient_id = u.id)
		WHERE u.id != ?
		GROUP BY u.id
		ORDER BY last_active DESC
	`
	rows, err := q.db.Query(query, userID, userID, userID, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	convs := make([]ConversationRow, 0)
	for rows.Next() {
		var c ConversationRow
		if err := rows.Scan(&c.UserID, &c.Handle, &c.LastMessage, &c.UnreadCount, &c.LastActivity); err != nil {
			return nil, err
		}
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

// ─── Cleanup ──────────────────────────────────────────────────────────────────

// DeleteExpiredMessages removes messages past their expires_at.
func (q *Queries) DeleteExpiredMessages() (int64, error) {
	res, err := q.db.Exec(`DELETE FROM messages WHERE expires_at IS NOT NULL AND expires_at < datetime('now')`)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// DeleteExpiredFiles removes file metadata for expired files.
func (q *Queries) DeleteExpiredFiles() ([]string, error) {
	rows, err := q.db.Query(`SELECT id, encrypted_blob_path FROM files WHERE expires_at IS NOT NULL AND expires_at < datetime('now')`)
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
	ID                     string `json:"id"`
	Handle                 string `json:"handle"`
	PublicKeyEd25519       []byte `json:"public_key_ed25519"`
	PublicKeyX25519        []byte `json:"public_key_x25519"`
	DerivedPublicKeyEd25519 []byte `json:"derived_public_key_ed25519"`
	CreatedAt              string `json:"created_at"`
}

type SessionRow struct {
	TokenHash string
	UserID    string
	CreatedAt time.Time
	ExpiresAt time.Time
}

type MessageRow struct {
	ID                string
	SenderID          string
	RecipientID       *string
	GroupID           *string
	Ciphertext        []byte
	EphemeralPubKey   []byte
	Nonce             []byte
	CreatedAt         string
	ExpiresAt         *string
	ReadAt            *string
}

type GroupRow struct {
	ID                   string
	EncryptedName        []byte
	EncryptedSymmetricKey []byte
	CreatedAt            string
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
	RecoveryCodeHash   string
	EncryptedPrivateKey []byte
	Salt               []byte
	AuthSalt           []byte
}

// ─── Helpers ──────────────────────────────────────────────────────────────────

func scanUser(s interface{ Scan(dest ...interface{}) error }) (*UserRow, error) {
	u := &UserRow{}
	if err := s.Scan(&u.ID, &u.Handle, &u.PublicKeyEd25519, &u.PublicKeyX25519, &u.DerivedPublicKeyEd25519, &u.CreatedAt); err != nil {
		return nil, err
	}
	return u, nil
}

func scanMessage(s interface{ Scan(dest ...interface{}) error }) (*MessageRow, error) {
	m := &MessageRow{}
	if err := s.Scan(&m.ID, &m.SenderID, &m.RecipientID, &m.GroupID, &m.Ciphertext, &m.EphemeralPubKey, &m.Nonce, &m.CreatedAt, &m.ExpiresAt, &m.ReadAt); err != nil {
		return nil, err
	}
	return m, nil
}

func nullStr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
