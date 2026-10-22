package gateway

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/google/uuid"

	"reflex-server/internal/auth"
	"reflex-server/internal/database"
	"reflex-server/internal/models"
)

func setupTestGateway(t *testing.T) (*httptest.Server, *auth.TokenManager) {
	t.Helper()
	tm := auth.NewTokenManager("test-secret")
	hub := NewHub()
	go hub.Run()

	handler := NewHandler(hub, tm, nil)
	mux := http.NewServeMux()
	mux.HandleFunc("/ws", handler.ServeWS)

	server := httptest.NewServer(mux)
	return server, tm
}

func setupTestGatewayWithDB(t *testing.T, dbPath string) (*httptest.Server, *auth.TokenManager, *sql.DB) {
	t.Helper()
	db, err := database.InitDB(dbPath)
	if err != nil {
		t.Fatalf("Failed to init DB: %v", err)
	}

	tm := auth.NewTokenManager("test-secret")
	hub := NewHub()
	go hub.Run()

	handler := NewHandler(hub, tm, db)
	mux := http.NewServeMux()
	mux.HandleFunc("/ws", handler.ServeWS)

	server := httptest.NewServer(mux)
	return server, tm, db
}

func TestWebSocketAuth(t *testing.T) {
	server, tm := setupTestGateway(t)
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/ws"

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// 1. Unauthenticated connection should fail
	_, resp, err := websocket.Dial(ctx, wsURL, nil)
	if err == nil {
		t.Fatalf("Expected connection to fail without token")
	}
	if resp != nil && resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("Expected status 401, got %d", resp.StatusCode)
	}

	// 2. Authenticated connection via query param ?token=
	token, err := tm.GenerateToken("user-1", "alice")
	if err != nil {
		t.Fatalf("Failed to generate token: %v", err)
	}

	conn, _, err := websocket.Dial(ctx, wsURL+"?token="+token, nil)
	if err != nil {
		t.Fatalf("Failed to connect with valid token query param: %v", err)
	}
	defer conn.Close(websocket.StatusNormalClosure, "")

	// 3. Authenticated connection via Authorization header
	opts := &websocket.DialOptions{
		HTTPHeader: http.Header{
			"Authorization": []string{"Bearer " + token},
		},
	}
	conn2, _, err := websocket.Dial(ctx, wsURL, opts)
	if err != nil {
		t.Fatalf("Failed to connect with valid Authorization header: %v", err)
	}
	defer conn2.Close(websocket.StatusNormalClosure, "")
}

func TestWebSocketPingPongFrame(t *testing.T) {
	server, tm := setupTestGateway(t)
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/ws"
	token, _ := tm.GenerateToken("user-1", "alice")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	conn, _, err := websocket.Dial(ctx, wsURL+"?token="+token, nil)
	if err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}
	defer conn.Close(websocket.StatusNormalClosure, "")

	// Send ping frame
	pingFrame := models.WSFrame{
		Type:    "ping",
		Payload: map[string]interface{}{"seq": float64(1)},
	}
	data, _ := json.Marshal(pingFrame)
	err = conn.Write(ctx, websocket.MessageText, data)
	if err != nil {
		t.Fatalf("Failed to write ping frame: %v", err)
	}

	// Read pong response
	_, responseBytes, err := conn.Read(ctx)
	if err != nil {
		t.Fatalf("Failed to read pong response: %v", err)
	}

	var respFrame models.WSFrame
	if err := json.Unmarshal(responseBytes, &respFrame); err != nil {
		t.Fatalf("Failed to unmarshal response: %v", err)
	}

	if respFrame.Type != "pong" {
		t.Errorf("Expected response type 'pong', got '%s'", respFrame.Type)
	}
}

func TestWebSocketMessageAckFrame(t *testing.T) {
	server, tm := setupTestGateway(t)
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/ws"
	token, _ := tm.GenerateToken("user-1", "alice")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	conn, _, err := websocket.Dial(ctx, wsURL+"?token="+token, nil)
	if err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}
	defer conn.Close(websocket.StatusNormalClosure, "")

	// Send message frame
	msgFrame := models.WSFrame{
		Type:    "message",
		Payload: map[string]interface{}{"text": "hello"},
	}
	data, _ := json.Marshal(msgFrame)
	err = conn.Write(ctx, websocket.MessageText, data)
	if err != nil {
		t.Fatalf("Failed to write message frame: %v", err)
	}

	// Read ack response
	_, responseBytes, err := conn.Read(ctx)
	if err != nil {
		t.Fatalf("Failed to read ack response: %v", err)
	}

	var respFrame models.WSFrame
	if err := json.Unmarshal(responseBytes, &respFrame); err != nil {
		t.Fatalf("Failed to unmarshal response: %v", err)
	}

	if respFrame.Type != "ack" {
		t.Errorf("Expected response type 'ack', got '%s'", respFrame.Type)
	}
}

func TestDirectMessageOnlineDelivery(t *testing.T) {
	server, tm := setupTestGateway(t)
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/ws"

	tokenAlice, _ := tm.GenerateToken("user-alice", "alice")
	tokenBob, _ := tm.GenerateToken("user-bob", "bob")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Connect Alice
	connAlice, _, err := websocket.Dial(ctx, wsURL+"?token="+tokenAlice, nil)
	if err != nil {
		t.Fatalf("Failed to connect Alice: %v", err)
	}
	defer connAlice.Close(websocket.StatusNormalClosure, "")

	// Connect Bob
	connBob, _, err := websocket.Dial(ctx, wsURL+"?token="+tokenBob, nil)
	if err != nil {
		t.Fatalf("Failed to connect Bob: %v", err)
	}
	defer connBob.Close(websocket.StatusNormalClosure, "")

	// Wait briefly for WS registration
	time.Sleep(50 * time.Millisecond)

	// Alice sends direct_message to Bob
	dmFrame := models.WSFrame{
		Type:              "direct_message",
		RecipientUsername: "bob",
		ClientMsgID:       "msg-123",
		Payload:           "Hello Bob online!",
	}
	data, _ := json.Marshal(dmFrame)
	if err := connAlice.Write(ctx, websocket.MessageText, data); err != nil {
		t.Fatalf("Alice failed to write direct message: %v", err)
	}

	// Read Bob's incoming frame
	_, bobBytes, err := connBob.Read(ctx)
	if err != nil {
		t.Fatalf("Bob failed to read message: %v", err)
	}
	var bobFrame models.WSFrame
	if err := json.Unmarshal(bobBytes, &bobFrame); err != nil {
		t.Fatalf("Bob failed to unmarshal frame: %v", err)
	}

	if bobFrame.Type != "direct_message" {
		t.Errorf("Expected Bob frame type 'direct_message', got '%s'", bobFrame.Type)
	}
	if bobFrame.SenderUsername != "alice" {
		t.Errorf("Expected SenderUsername 'alice', got '%s'", bobFrame.SenderUsername)
	}
	if bobFrame.Payload != "Hello Bob online!" {
		t.Errorf("Expected Payload 'Hello Bob online!', got '%v'", bobFrame.Payload)
	}

	// Read Alice's ack_delivered response
	_, aliceBytes, err := connAlice.Read(ctx)
	if err != nil {
		t.Fatalf("Alice failed to read ack: %v", err)
	}
	var aliceAck models.WSFrame
	if err := json.Unmarshal(aliceBytes, &aliceAck); err != nil {
		t.Fatalf("Alice failed to unmarshal ack: %v", err)
	}

	if aliceAck.Type != "ack_delivered" {
		t.Errorf("Expected Alice frame type 'ack_delivered', got '%s'", aliceAck.Type)
	}
	if aliceAck.ClientMsgID != "msg-123" {
		t.Errorf("Expected ClientMsgID 'msg-123', got '%s'", aliceAck.ClientMsgID)
	}
}

func TestDirectMessageOfflineQueuingAndFlush(t *testing.T) {
	tempDB := "test_gateway_offline.db"
	defer os.Remove(tempDB)

	server, tm, db := setupTestGatewayWithDB(t, tempDB)
	defer server.Close()
	defer db.Close()

	// Insert Alice and Bob in DB
	aliceID := uuid.New().String()
	bobID := uuid.New().String()
	_, err := db.Exec(`
		INSERT INTO users (id, username, identifier_type, identifier_value, password_hash, public_identity_key, created_at)
		VALUES
		(?, 'alice', 'email', 'alice@test.com', 'h1', 'k1', ?),
		(?, 'bob', 'email', 'bob@test.com', 'h2', 'k2', ?)
	`, aliceID, time.Now().UTC(), bobID, time.Now().UTC())
	if err != nil {
		t.Fatalf("Failed to insert test users: %v", err)
	}

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/ws"

	tokenAlice, _ := tm.GenerateToken(aliceID, "alice")
	tokenBob, _ := tm.GenerateToken(bobID, "bob")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// 1. Alice connects (Bob is offline)
	connAlice, _, err := websocket.Dial(ctx, wsURL+"?token="+tokenAlice, nil)
	if err != nil {
		t.Fatalf("Failed to connect Alice: %v", err)
	}
	defer connAlice.Close(websocket.StatusNormalClosure, "")

	// 2. Alice sends message to offline user Bob
	dmFrame := models.WSFrame{
		Type:              "direct_message",
		RecipientUsername: "bob",
		ClientMsgID:       "msg-off-1",
		Payload:           "Hello Bob offline!",
	}
	data, _ := json.Marshal(dmFrame)
	if err := connAlice.Write(ctx, websocket.MessageText, data); err != nil {
		t.Fatalf("Alice failed to write message: %v", err)
	}

	// 3. Alice should receive ack_queued
	_, aliceBytes, err := connAlice.Read(ctx)
	if err != nil {
		t.Fatalf("Alice failed to read ack_queued: %v", err)
	}
	var aliceAck models.WSFrame
	if err := json.Unmarshal(aliceBytes, &aliceAck); err != nil {
		t.Fatalf("Alice failed to unmarshal ack_queued: %v", err)
	}

	if aliceAck.Type != "ack_queued" {
		t.Errorf("Expected frame type 'ack_queued', got '%s'", aliceAck.Type)
	}
	if aliceAck.ClientMsgID != "msg-off-1" {
		t.Errorf("Expected ClientMsgID 'msg-off-1', got '%s'", aliceAck.ClientMsgID)
	}

	// 4. Verify DB contains 1 pending offline message for Bob
	pending, err := database.GetPendingOfflineMessages(db, bobID)
	if err != nil {
		t.Fatalf("GetPendingOfflineMessages failed: %v", err)
	}
	if len(pending) != 1 {
		t.Fatalf("Expected 1 pending offline message in DB, got %d", len(pending))
	}
	if pending[0].Payload != "Hello Bob offline!" {
		t.Errorf("Expected payload 'Hello Bob offline!', got '%s'", pending[0].Payload)
	}

	// 5. Bob connects -> flush pending message
	connBob, _, err := websocket.Dial(ctx, wsURL+"?token="+tokenBob, nil)
	if err != nil {
		t.Fatalf("Failed to connect Bob: %v", err)
	}
	defer connBob.Close(websocket.StatusNormalClosure, "")

	// Bob should receive flushed message automatically
	_, bobBytes, err := connBob.Read(ctx)
	if err != nil {
		t.Fatalf("Bob failed to receive flushed message: %v", err)
	}
	var bobFrame models.WSFrame
	if err := json.Unmarshal(bobBytes, &bobFrame); err != nil {
		t.Fatalf("Bob failed to unmarshal flushed message: %v", err)
	}

	if bobFrame.Type != "direct_message" {
		t.Errorf("Expected flushed frame type 'direct_message', got '%s'", bobFrame.Type)
	}
	if bobFrame.SenderUsername != "alice" {
		t.Errorf("Expected SenderUsername 'alice', got '%s'", bobFrame.SenderUsername)
	}
	if bobFrame.Payload != "Hello Bob offline!" {
		t.Errorf("Expected payload 'Hello Bob offline!', got '%v'", bobFrame.Payload)
	}

	// 6. Verify DB has 0 pending offline messages for Bob
	time.Sleep(50 * time.Millisecond) // wait briefly for status update in DB
	pendingAfter, err := database.GetPendingOfflineMessages(db, bobID)
	if err != nil {
		t.Fatalf("GetPendingOfflineMessages after flush failed: %v", err)
	}
	if len(pendingAfter) != 0 {
		t.Errorf("Expected 0 pending messages after flush, got %d", len(pendingAfter))
	}
}
