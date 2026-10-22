package models

import (
	"time"

	"github.com/golang-jwt/jwt/v5"
)

type IdentifierType string

const (
	IdentifierTypePhone IdentifierType = "phone"
	IdentifierTypeEmail IdentifierType = "email"
)

type User struct {
	ID                 string         `json:"id"`
	Username           string         `json:"username"`
	IdentifierType     IdentifierType `json:"identifier_type"`
	IdentifierValue    string         `json:"identifier_value"`
	PasswordHash       string         `json:"-"`
	PublicIdentityKey  string         `json:"public_identity_key"`
	CreatedAt          time.Time      `json:"created_at"`
}

type RegisterRequest struct {
	Username          string `json:"username"`
	Identifier        string `json:"identifier"`
	Password          string `json:"password"`
	PublicIdentityKey string `json:"public_identity_key"`
}

type LoginRequest struct {
	Identifier string `json:"identifier"`
	Password   string `json:"password"`
}

type AuthResponse struct {
	Token string `json:"token"`
	User  *User  `json:"user"`
}

type UserKeysResponse struct {
	Username          string `json:"username"`
	PublicIdentityKey string `json:"public_identity_key"`
}

type JWTClaims struct {
	UserID   string `json:"user_id"`
	Username string `json:"username"`
	jwt.RegisteredClaims
}

type WSFrame struct {
	Type    string      `json:"type"` // "ping", "pong", "message", "ack"
	Payload interface{} `json:"payload,omitempty"`
}
