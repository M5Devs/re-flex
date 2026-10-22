package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"

	"reflex-server/internal/auth"
	"reflex-server/internal/database"
	"reflex-server/internal/gateway"
	"reflex-server/internal/models"
)

func setupTestServer(t *testing.T) (*httptest.Server, func()) {
	t.Helper()
	dbPath := "test_main.db"
	db, err := database.InitDB(dbPath)
	if err != nil {
		t.Fatalf("Failed to init DB: %v", err)
	}

	tm := auth.NewTokenManager("main-test-secret")
	authSvc := auth.NewService(db, tm)
	authHdr := auth.NewHandler(authSvc)

	hub := gateway.NewHub()
	go hub.Run()
	gwHdr := gateway.NewHandler(hub, tm, db)

	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/v1/auth/register", authHdr.Register)
	mux.HandleFunc("POST /api/v1/auth/login", authHdr.Login)
	mux.HandleFunc("GET /api/v1/users/{username}/keys", authHdr.GetUserKeys)
	mux.HandleFunc("/ws", gwHdr.ServeWS)

	server := httptest.NewServer(mux)

	cleanup := func() {
		server.Close()
		db.Close()
		os.Remove(dbPath)
	}

	return server, cleanup
}

func TestE2EFlow(t *testing.T) {
	server, cleanup := setupTestServer(t)
	defer cleanup()

	client := server.Client()

	// 1. Register user with email
	regBodyEmail, _ := json.Marshal(models.RegisterRequest{
		Username:          "user_email",
		Identifier:        "user@reflex.dev",
		Password:          "password123",
		PublicIdentityKey: "ed25519_key_email",
	})
	resp, err := client.Post(server.URL+"/api/v1/auth/register", "application/json", bytes.NewBuffer(regBodyEmail))
	if err != nil {
		t.Fatalf("Failed to send register request: %v", err)
	}
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("Expected status 201 Created, got %d", resp.StatusCode)
	}

	var authRespEmail models.AuthResponse
	json.NewDecoder(resp.Body).Decode(&authRespEmail)
	resp.Body.Close()

	if authRespEmail.Token == "" {
		t.Fatalf("Expected non-empty JWT token")
	}

	// 2. Register user with phone
	regBodyPhone, _ := json.Marshal(models.RegisterRequest{
		Username:          "user_phone",
		Identifier:        "+15551234567",
		Password:          "password123",
		PublicIdentityKey: "ed25519_key_phone",
	})
	resp, err = client.Post(server.URL+"/api/v1/auth/register", "application/json", bytes.NewBuffer(regBodyPhone))
	if err != nil {
		t.Fatalf("Failed to send register phone request: %v", err)
	}
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("Expected status 201 Created, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	// 3. Login user with phone
	loginBody, _ := json.Marshal(models.LoginRequest{
		Identifier: "+15551234567",
		Password:   "password123",
	})
	resp, err = client.Post(server.URL+"/api/v1/auth/login", "application/json", bytes.NewBuffer(loginBody))
	if err != nil {
		t.Fatalf("Failed to send login request: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Expected status 200 OK on login, got %d", resp.StatusCode)
	}

	var loginAuthResp models.AuthResponse
	json.NewDecoder(resp.Body).Decode(&loginAuthResp)
	resp.Body.Close()

	if loginAuthResp.Token == "" {
		t.Fatalf("Expected token on login")
	}

	// 4. Fetch public key of registered user
	resp, err = client.Get(server.URL + "/api/v1/users/user_email/keys")
	if err != nil {
		t.Fatalf("Failed to fetch public key: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Expected status 200 OK for user keys, got %d", resp.StatusCode)
	}

	var keyResp models.UserKeysResponse
	json.NewDecoder(resp.Body).Decode(&keyResp)
	resp.Body.Close()

	if keyResp.PublicIdentityKey != "ed25519_key_email" {
		t.Errorf("Expected public key 'ed25519_key_email', got '%s'", keyResp.PublicIdentityKey)
	}

	// 5. Connect to WebSocket using JWT from registration
	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/ws?token=" + authRespEmail.Token
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	wsConn, _, err := websocket.Dial(ctx, wsURL, nil)
	if err != nil {
		t.Fatalf("Failed to connect to WebSocket: %v", err)
	}
	defer wsConn.Close(websocket.StatusNormalClosure, "")

	// Send ping
	pingData, _ := json.Marshal(models.WSFrame{Type: "ping"})
	if err := wsConn.Write(ctx, websocket.MessageText, pingData); err != nil {
		t.Fatalf("Failed to send ping: %v", err)
	}

	_, msg, err := wsConn.Read(ctx)
	if err != nil {
		t.Fatalf("Failed to read pong: %v", err)
	}

	var pongFrame models.WSFrame
	json.Unmarshal(msg, &pongFrame)
	if pongFrame.Type != "pong" {
		t.Errorf("Expected pong frame, got %s", pongFrame.Type)
	}
}
