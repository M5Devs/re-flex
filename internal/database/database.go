package database

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"

	"reflex-server/internal/models"
)

func InitDB(dbPath string) (*sql.DB, error) {
	dir := filepath.Dir(dbPath)
	if dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return nil, fmt.Errorf("failed to create db directory: %w", err)
		}
	}

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open sqlite db: %w", err)
	}

	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("failed to ping sqlite db: %w", err)
	}

	if err := migrate(db); err != nil {
		return nil, fmt.Errorf("failed to migrate db: %w", err)
	}

	return db, nil
}

func migrate(db *sql.DB) error {
	schema := `
	CREATE TABLE IF NOT EXISTS users (
		id TEXT PRIMARY KEY,
		username TEXT NOT NULL UNIQUE,
		identifier_type TEXT NOT NULL,
		identifier_value TEXT NOT NULL UNIQUE,
		password_hash TEXT NOT NULL,
		public_identity_key TEXT NOT NULL,
		created_at DATETIME NOT NULL
	);
	CREATE INDEX IF NOT EXISTS idx_users_username ON users(username);
	CREATE INDEX IF NOT EXISTS idx_users_identifier_value ON users(identifier_value);

	CREATE TABLE IF NOT EXISTS offline_messages (
		id TEXT PRIMARY KEY,
		sender_id TEXT NOT NULL REFERENCES users(id),
		recipient_id TEXT NOT NULL REFERENCES users(id),
		client_msg_id TEXT,
		payload TEXT NOT NULL,
		status TEXT NOT NULL DEFAULT 'pending',
		created_at DATETIME NOT NULL
	);
	CREATE INDEX IF NOT EXISTS idx_offline_messages_recipient_status ON offline_messages(recipient_id, status);
	`

	_, err := db.Exec(schema)
	return err
}

func GetUserByUsername(db *sql.DB, username string) (*models.User, error) {
	user := &models.User{}
	query := `
		SELECT id, username, identifier_type, identifier_value, password_hash, public_identity_key, created_at
		FROM users
		WHERE username = ?
	`
	err := db.QueryRow(query, username).Scan(
		&user.ID,
		&user.Username,
		&user.IdentifierType,
		&user.IdentifierValue,
		&user.PasswordHash,
		&user.PublicIdentityKey,
		&user.CreatedAt,
	)
	if err != nil {
		return nil, err
	}
	return user, nil
}

func SaveOfflineMessage(db *sql.DB, msg *models.OfflineMessage) error {
	query := `
		INSERT INTO offline_messages (id, sender_id, recipient_id, client_msg_id, payload, status, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`
	_, err := db.Exec(query, msg.ID, msg.SenderID, msg.RecipientID, msg.ClientMsgID, msg.Payload, msg.Status, msg.CreatedAt)
	return err
}

func GetPendingOfflineMessages(db *sql.DB, recipientID string) ([]*models.OfflineMessage, error) {
	query := `
		SELECT om.id, om.sender_id, u.username, om.recipient_id, om.client_msg_id, om.payload, om.status, om.created_at
		FROM offline_messages om
		JOIN users u ON om.sender_id = u.id
		WHERE om.recipient_id = ? AND om.status = 'pending'
		ORDER BY om.created_at ASC
	`
	rows, err := db.Query(query, recipientID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var messages []*models.OfflineMessage
	for rows.Next() {
		msg := &models.OfflineMessage{}
		var clientMsgID sql.NullString
		if err := rows.Scan(
			&msg.ID,
			&msg.SenderID,
			&msg.SenderUsername,
			&msg.RecipientID,
			&clientMsgID,
			&msg.Payload,
			&msg.Status,
			&msg.CreatedAt,
		); err != nil {
			return nil, err
		}
		if clientMsgID.Valid {
			msg.ClientMsgID = clientMsgID.String
		}
		messages = append(messages, msg)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return messages, nil
}

func MarkOfflineMessageDelivered(db *sql.DB, msgID string) error {
	query := `UPDATE offline_messages SET status = 'delivered' WHERE id = ?`
	_, err := db.Exec(query, msgID)
	return err
}
