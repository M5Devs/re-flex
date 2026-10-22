package gateway

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/coder/websocket"
	"github.com/google/uuid"

	"reflex-server/internal/database"
	"reflex-server/internal/models"
)

const (
	writeWait  = 10 * time.Second
	pongWait   = 60 * time.Second
	pingPeriod = (pongWait * 9) / 10
)

type Client struct {
	ID       string
	UserID   string
	Username string
	hub      *Hub
	conn     *websocket.Conn
	send     chan []byte
	db       *sql.DB
}

func NewClient(id, userID, username string, hub *Hub, conn *websocket.Conn, db *sql.DB) *Client {
	return &Client{
		ID:       id,
		UserID:   userID,
		Username: username,
		hub:      hub,
		conn:     conn,
		send:     make(chan []byte, 256),
		db:       db,
	}
}

func (c *Client) ReadPump(ctx context.Context) {
	defer func() {
		c.hub.Unregister(c)
		c.conn.Close(websocket.StatusNormalClosure, "connection closed")
	}()

	c.conn.SetReadLimit(32768)

	for {
		_, message, err := c.conn.Read(ctx)
		if err != nil {
			break
		}

		var frame models.WSFrame
		if err := json.Unmarshal(message, &frame); err != nil {
			log.Printf("Error decoding WS message: %v", err)
			continue
		}

		c.handleFrame(ctx, frame)
	}
}

func (c *Client) WritePump(ctx context.Context) {
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		c.conn.Close(websocket.StatusNormalClosure, "connection closed")
	}()

	for {
		select {
		case message, ok := <-c.send:
			if !ok {
				c.conn.Close(websocket.StatusNormalClosure, "")
				return
			}

			writeCtx, cancel := context.WithTimeout(ctx, writeWait)
			err := c.conn.Write(writeCtx, websocket.MessageText, message)
			cancel()
			if err != nil {
				return
			}

		case <-ticker.C:
			// Send WS control ping or json ping
			writeCtx, cancel := context.WithTimeout(ctx, writeWait)
			err := c.conn.Ping(writeCtx)
			cancel()
			if err != nil {
				return
			}
		}
	}
}

func (c *Client) flushOfflineMessages(ctx context.Context) {
	if c.db == nil || c.UserID == "" {
		return
	}

	messages, err := database.GetPendingOfflineMessages(c.db, c.UserID)
	if err != nil {
		log.Printf("Error fetching pending offline messages for user %s: %v", c.UserID, err)
		return
	}

	for _, msg := range messages {
		frame := models.WSFrame{
			Type:           "direct_message",
			SenderUsername: msg.SenderUsername,
			ClientMsgID:    msg.ClientMsgID,
			Payload:        msg.Payload,
		}

		bytes, err := json.Marshal(frame)
		if err != nil {
			continue
		}

		select {
		case c.send <- bytes:
			_ = database.MarkOfflineMessageDelivered(c.db, msg.ID)
		case <-ctx.Done():
			return
		}
	}
}

func (c *Client) handleFrame(ctx context.Context, frame models.WSFrame) {
	switch frame.Type {
	case "ping":
		response := models.WSFrame{
			Type:    "pong",
			Payload: frame.Payload,
		}
		bytes, err := json.Marshal(response)
		if err == nil {
			select {
			case c.send <- bytes:
			default:
			}
		}

	case "message":
		ack := models.WSFrame{
			Type: "ack",
			Payload: map[string]interface{}{
				"status": "received",
			},
		}
		bytes, err := json.Marshal(ack)
		if err == nil {
			select {
			case c.send <- bytes:
			default:
			}
		}

	case "direct_message":
		c.handleDirectMessage(ctx, frame)

	case "pong":
		// Heartbeat pong received

	default:
		log.Printf("Unknown frame type received: %s", frame.Type)
	}
}

func (c *Client) handleDirectMessage(ctx context.Context, frame models.WSFrame) {
	recipientUsername := frame.RecipientUsername
	clientMsgID := frame.ClientMsgID
	payloadStr := ""

	if str, ok := frame.Payload.(string); ok {
		payloadStr = str
	} else if m, ok := frame.Payload.(map[string]interface{}); ok {
		if recipientUsername == "" {
			if r, ok := m["recipient_username"].(string); ok {
				recipientUsername = r
			}
		}
		if clientMsgID == "" {
			if id, ok := m["client_msg_id"].(string); ok {
				clientMsgID = id
			}
		}
		if p, ok := m["payload"].(string); ok {
			payloadStr = p
		} else if t, ok := m["text"].(string); ok {
			payloadStr = t
		}
	} else if frame.Payload != nil {
		payloadStr = fmt.Sprintf("%v", frame.Payload)
	}

	if recipientUsername == "" {
		log.Printf("Direct message missing recipient_username")
		return
	}

	targetClient, online := c.hub.GetClientByUsername(recipientUsername)
	if online {
		// Deliver immediately to recipient
		dmFrame := models.WSFrame{
			Type:              "direct_message",
			SenderUsername:    c.Username,
			RecipientUsername: recipientUsername,
			ClientMsgID:       clientMsgID,
			Payload:           payloadStr,
		}
		dmBytes, err := json.Marshal(dmFrame)
		if err == nil {
			select {
			case targetClient.send <- dmBytes:
			default:
			}
		}

		// Send ack_delivered to sender
		ackFrame := models.WSFrame{
			Type:              "ack_delivered",
			RecipientUsername: recipientUsername,
			ClientMsgID:       clientMsgID,
			Payload: map[string]interface{}{
				"client_msg_id":      clientMsgID,
				"recipient_username": recipientUsername,
				"status":             "delivered",
			},
		}
		ackBytes, err := json.Marshal(ackFrame)
		if err == nil {
			select {
			case c.send <- ackBytes:
			default:
			}
		}
	} else {
		// Target user offline -> store message in DB
		if c.db != nil {
			recipientUser, err := database.GetUserByUsername(c.db, recipientUsername)
			if err != nil {
				errFrame := models.WSFrame{
					Type:    "error",
					Payload: fmt.Sprintf("recipient user '%s' not found", recipientUsername),
				}
				errBytes, _ := json.Marshal(errFrame)
				select {
				case c.send <- errBytes:
				default:
				}
				return
			}

			offMsg := &models.OfflineMessage{
				ID:          uuid.New().String(),
				SenderID:    c.UserID,
				RecipientID: recipientUser.ID,
				ClientMsgID: clientMsgID,
				Payload:     payloadStr,
				Status:      "pending",
				CreatedAt:   time.Now().UTC(),
			}

			if err := database.SaveOfflineMessage(c.db, offMsg); err != nil {
				log.Printf("Error saving offline message: %v", err)
			}
		}

		// Send ack_queued to sender
		ackFrame := models.WSFrame{
			Type:              "ack_queued",
			RecipientUsername: recipientUsername,
			ClientMsgID:       clientMsgID,
			Payload: map[string]interface{}{
				"client_msg_id":      clientMsgID,
				"recipient_username": recipientUsername,
				"status":             "queued",
			},
		}
		ackBytes, err := json.Marshal(ackFrame)
		if err == nil {
			select {
			case c.send <- ackBytes:
			default:
			}
		}
	}
}
