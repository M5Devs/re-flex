package database

import (
	"os"
	"testing"
	"time"

	"github.com/google/uuid"

	"reflex-server/internal/models"
)

func TestInitDB(t *testing.T) {
	tempDB := "test_reflex.db"
	defer os.Remove(tempDB)

	db, err := InitDB(tempDB)
	if err != nil {
		t.Fatalf("InitDB failed: %v", err)
	}
	defer db.Close()

	var tableName string
	err = db.QueryRow("SELECT name FROM sqlite_master WHERE type='table' AND name='users';").Scan(&tableName)
	if err != nil {
		t.Fatalf("Failed to query users table: %v", err)
	}

	if tableName != "users" {
		t.Fatalf("Expected table name 'users', got '%s'", tableName)
	}

	var offlineTableName string
	err = db.QueryRow("SELECT name FROM sqlite_master WHERE type='table' AND name='offline_messages';").Scan(&offlineTableName)
	if err != nil {
		t.Fatalf("Failed to query offline_messages table: %v", err)
	}

	if offlineTableName != "offline_messages" {
		t.Fatalf("Expected table name 'offline_messages', got '%s'", offlineTableName)
	}
}

func TestOfflineMessagesOperations(t *testing.T) {
	tempDB := "test_offline_msgs.db"
	defer os.Remove(tempDB)

	db, err := InitDB(tempDB)
	if err != nil {
		t.Fatalf("InitDB failed: %v", err)
	}
	defer db.Close()

	// 1. Insert test users: Alice (sender) and Bob (recipient)
	aliceID := uuid.New().String()
	bobID := uuid.New().String()

	_, err = db.Exec(`
		INSERT INTO users (id, username, identifier_type, identifier_value, password_hash, public_identity_key, created_at)
		VALUES
		(?, 'alice', 'email', 'alice@example.com', 'hash1', 'pubkey1', ?),
		(?, 'bob', 'email', 'bob@example.com', 'hash2', 'pubkey2', ?)
	`, aliceID, time.Now().UTC(), bobID, time.Now().UTC())
	if err != nil {
		t.Fatalf("Failed to insert test users: %v", err)
	}

	// 2. Test GetUserByUsername
	uAlice, err := GetUserByUsername(db, "alice")
	if err != nil {
		t.Fatalf("GetUserByUsername failed for alice: %v", err)
	}
	if uAlice.ID != aliceID {
		t.Errorf("Expected Alice ID %s, got %s", aliceID, uAlice.ID)
	}

	// 3. Save offline message from Alice to Bob
	msgID := uuid.New().String()
	clientMsgID := "nonce-12345"
	offlineMsg := &models.OfflineMessage{
		ID:          msgID,
		SenderID:    aliceID,
		RecipientID: bobID,
		ClientMsgID: clientMsgID,
		Payload:     "EncryptedCiphertextPayload",
		Status:      "pending",
		CreatedAt:   time.Now().UTC(),
	}

	if err := SaveOfflineMessage(db, offlineMsg); err != nil {
		t.Fatalf("SaveOfflineMessage failed: %v", err)
	}

	// 4. Retrieve pending offline messages for Bob
	pendingMsgs, err := GetPendingOfflineMessages(db, bobID)
	if err != nil {
		t.Fatalf("GetPendingOfflineMessages failed: %v", err)
	}
	if len(pendingMsgs) != 1 {
		t.Fatalf("Expected 1 pending message, got %d", len(pendingMsgs))
	}

	msg := pendingMsgs[0]
	if msg.ID != msgID {
		t.Errorf("Expected msg ID %s, got %s", msgID, msg.ID)
	}
	if msg.SenderUsername != "alice" {
		t.Errorf("Expected SenderUsername 'alice', got '%s'", msg.SenderUsername)
	}
	if msg.ClientMsgID != clientMsgID {
		t.Errorf("Expected ClientMsgID '%s', got '%s'", clientMsgID, msg.ClientMsgID)
	}
	if msg.Payload != "EncryptedCiphertextPayload" {
		t.Errorf("Expected Payload 'EncryptedCiphertextPayload', got '%s'", msg.Payload)
	}

	// 5. Mark message as delivered
	if err := MarkOfflineMessageDelivered(db, msgID); err != nil {
		t.Fatalf("MarkOfflineMessageDelivered failed: %v", err)
	}

	// 6. Verify Bob has no more pending messages
	pendingAfter, err := GetPendingOfflineMessages(db, bobID)
	if err != nil {
		t.Fatalf("GetPendingOfflineMessages failed after update: %v", err)
	}
	if len(pendingAfter) != 0 {
		t.Errorf("Expected 0 pending messages after delivery, got %d", len(pendingAfter))
	}
}
