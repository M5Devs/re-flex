package auth

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"reflex-server/internal/models"
)

var (
	ErrUserAlreadyExists  = errors.New("user with this username or identifier already exists")
	ErrInvalidCredentials = errors.New("invalid identifier or password")
	ErrUserNotFound       = errors.New("user not found")
)

type Service struct {
	db           *sql.DB
	tokenManager *TokenManager
}

func NewService(db *sql.DB, tokenManager *TokenManager) *Service {
	return &Service{
		db:           db,
		tokenManager: tokenManager,
	}
}

func (s *Service) Register(req models.RegisterRequest) (*models.AuthResponse, error) {
	req.Username = strings.TrimSpace(req.Username)
	req.Identifier = strings.TrimSpace(req.Identifier)
	req.PublicIdentityKey = strings.TrimSpace(req.PublicIdentityKey)

	if req.Username == "" {
		return nil, errors.New("username is required")
	}
	if req.Password == "" {
		return nil, errors.New("password is required")
	}
	if req.PublicIdentityKey == "" {
		return nil, errors.New("public_identity_key is required")
	}

	idType, idVal, err := ValidateAndCategorizeIdentifier(req.Identifier)
	if err != nil {
		return nil, err
	}

	hash, err := HashPassword(req.Password)
	if err != nil {
		return nil, fmt.Errorf("failed to hash password: %w", err)
	}

	user := &models.User{
		ID:                uuid.New().String(),
		Username:          req.Username,
		IdentifierType:    idType,
		IdentifierValue:   idVal,
		PasswordHash:      hash,
		PublicIdentityKey: req.PublicIdentityKey,
		CreatedAt:         time.Now().UTC(),
	}

	query := `
		INSERT INTO users (id, username, identifier_type, identifier_value, password_hash, public_identity_key, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`
	_, err = s.db.Exec(query, user.ID, user.Username, user.IdentifierType, user.IdentifierValue, user.PasswordHash, user.PublicIdentityKey, user.CreatedAt)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint failed") {
			return nil, ErrUserAlreadyExists
		}
		return nil, fmt.Errorf("failed to create user: %w", err)
	}

	token, err := s.tokenManager.GenerateToken(user.ID, user.Username)
	if err != nil {
		return nil, fmt.Errorf("failed to generate token: %w", err)
	}

	return &models.AuthResponse{
		Token: token,
		User:  user,
	}, nil
}

func (s *Service) Login(req models.LoginRequest) (*models.AuthResponse, error) {
	req.Identifier = strings.TrimSpace(req.Identifier)
	if req.Identifier == "" || req.Password == "" {
		return nil, ErrInvalidCredentials
	}

	_, idVal, err := ValidateAndCategorizeIdentifier(req.Identifier)
	if err != nil {
		return nil, ErrInvalidCredentials
	}

	user := &models.User{}
	query := `
		SELECT id, username, identifier_type, identifier_value, password_hash, public_identity_key, created_at
		FROM users
		WHERE identifier_value = ?
	`
	err = s.db.QueryRow(query, idVal).Scan(
		&user.ID,
		&user.Username,
		&user.IdentifierType,
		&user.IdentifierValue,
		&user.PasswordHash,
		&user.PublicIdentityKey,
		&user.CreatedAt,
	)

	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrInvalidCredentials
		}
		return nil, fmt.Errorf("failed to query user: %w", err)
	}

	if !CheckPasswordHash(req.Password, user.PasswordHash) {
		return nil, ErrInvalidCredentials
	}

	token, err := s.tokenManager.GenerateToken(user.ID, user.Username)
	if err != nil {
		return nil, fmt.Errorf("failed to generate token: %w", err)
	}

	return &models.AuthResponse{
		Token: token,
		User:  user,
	}, nil
}

func (s *Service) GetPublicIdentityKey(username string) (*models.UserKeysResponse, error) {
	username = strings.TrimSpace(username)
	if username == "" {
		return nil, errors.New("username is required")
	}

	var pubKey string
	query := `SELECT public_identity_key FROM users WHERE username = ?`
	err := s.db.QueryRow(query, username).Scan(&pubKey)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrUserNotFound
		}
		return nil, fmt.Errorf("failed to query user keys: %w", err)
	}

	return &models.UserKeysResponse{
		Username:          username,
		PublicIdentityKey: pubKey,
	}, nil
}

func (s *Service) TokenManager() *TokenManager {
	return s.tokenManager
}
