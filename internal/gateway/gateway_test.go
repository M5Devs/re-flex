package gateway

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"

	"reflex-server/internal/auth"
	"reflex-server/internal/models"
)

func setupTestGateway(t *testing.T) (*httptest.Server, *auth.TokenManager) {
	t.Helper()
	tm := auth.NewTokenManager("test-secret")
	hub := NewHub()
	go hub.Run()

	handler := NewHandler(hub, tm)
	mux := http.NewServeMux()
	mux.HandleFunc("/ws", handler.ServeWS)

	server := httptest.NewServer(mux)
	return server, tm
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
