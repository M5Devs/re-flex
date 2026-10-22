package gateway

import (
	"database/sql"
	"net/http"
	"strings"

	"github.com/coder/websocket"
	"github.com/google/uuid"

	"reflex-server/internal/auth"
)

type Handler struct {
	hub          *Hub
	tokenManager *auth.TokenManager
	db           *sql.DB
}

func NewHandler(hub *Hub, tokenManager *auth.TokenManager, db *sql.DB) *Handler {
	return &Handler{
		hub:          hub,
		tokenManager: tokenManager,
		db:           db,
	}
}

func (h *Handler) ServeWS(w http.ResponseWriter, r *http.Request) {
	tokenStr := extractToken(r)
	if tokenStr == "" {
		http.Error(w, "Unauthorized: missing token", http.StatusUnauthorized)
		return
	}

	claims, err := h.tokenManager.VerifyToken(tokenStr)
	if err != nil {
		http.Error(w, "Unauthorized: invalid token", http.StatusUnauthorized)
		return
	}

	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		InsecureSkipVerify: true, // Allow connections in dev/self-hosted setups
	})
	if err != nil {
		return
	}

	clientID := uuid.New().String()
	client := NewClient(clientID, claims.UserID, claims.Username, h.hub, conn, h.db)
	h.hub.Register(client)

	ctx := r.Context()
	go client.WritePump(ctx)
	go client.flushOfflineMessages(ctx)
	client.ReadPump(ctx)
}

func extractToken(r *http.Request) string {
	// 1. Try query param ?token=
	token := r.URL.Query().Get("token")
	if token != "" {
		return token
	}

	// 2. Try Authorization header (Bearer <token> or <token>)
	authHeader := r.Header.Get("Authorization")
	if authHeader != "" {
		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) == 2 && strings.EqualFold(parts[0], "Bearer") {
			return parts[1]
		}
		return authHeader
	}

	// 3. Try Sec-WebSocket-Protocol header if passed as protocol
	wsProtocol := r.Header.Get("Sec-WebSocket-Protocol")
	if wsProtocol != "" {
		protocols := strings.Split(wsProtocol, ",")
		if len(protocols) > 0 {
			return strings.TrimSpace(protocols[0])
		}
	}

	return ""
}
