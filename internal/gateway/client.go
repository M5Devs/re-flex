package gateway

import (
	"context"
	"encoding/json"
	"log"
	"time"

	"github.com/coder/websocket"

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
}

func NewClient(id, userID, username string, hub *Hub, conn *websocket.Conn) *Client {
	return &Client{
		ID:       id,
		UserID:   userID,
		Username: username,
		hub:      hub,
		conn:     conn,
		send:     make(chan []byte, 256),
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

	case "pong":
		// Heartbeat pong received

	default:
		log.Printf("Unknown frame type received: %s", frame.Type)
	}
}
