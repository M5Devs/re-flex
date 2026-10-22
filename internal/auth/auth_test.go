package auth

import (
	"os"
	"testing"

	"reflex-server/internal/database"
	"reflex-server/internal/models"
)

func setupTestService(t *testing.T) (*Service, func()) {
	t.Helper()
	dbPath := "test_auth.db"
	db, err := database.InitDB(dbPath)
	if err != nil {
		t.Fatalf("Failed to init db: %v", err)
	}

	tm := NewTokenManager("test-secret")
	svc := NewService(db, tm)

	cleanup := func() {
		db.Close()
		os.Remove(dbPath)
	}

	return svc, cleanup
}

func TestValidateAndCategorizeIdentifier(t *testing.T) {
	tests := []struct {
		identifier string
		wantType   models.IdentifierType
		wantErr    bool
	}{
		{"user@example.com", models.IdentifierTypeEmail, false},
		{"john.doe+test@domain.co.uk", models.IdentifierTypeEmail, false},
		{"+14155552671", models.IdentifierTypePhone, false},
		{"+447911123456", models.IdentifierTypePhone, false},
		{"invalid-identifier", "", true},
		{"1234567", "", true},
		{"@domain.com", "", true},
	}

	for _, tt := range tests {
		idType, idVal, err := ValidateAndCategorizeIdentifier(tt.identifier)
		if tt.wantErr {
			if err == nil {
				t.Errorf("ValidateAndCategorizeIdentifier(%s) expected error, got nil", tt.identifier)
			}
		} else {
			if err != nil {
				t.Errorf("ValidateAndCategorizeIdentifier(%s) unexpected error: %v", tt.identifier, err)
			}
			if idType != tt.wantType {
				t.Errorf("ValidateAndCategorizeIdentifier(%s) got type %s, want %s", tt.identifier, idType, tt.wantType)
			}
			if idVal != tt.identifier {
				t.Errorf("ValidateAndCategorizeIdentifier(%s) got val %s, want %s", tt.identifier, idVal, tt.identifier)
			}
		}
	}
}

func TestRegisterAndLoginWithEmail(t *testing.T) {
	svc, cleanup := setupTestService(t)
	defer cleanup()

	regReq := models.RegisterRequest{
		Username:          "alice",
		Identifier:        "alice@example.com",
		Password:          "secret123",
		PublicIdentityKey: "pubkey_alice_12345",
	}

	resp, err := svc.Register(regReq)
	if err != nil {
		t.Fatalf("Register with email failed: %v", err)
	}

	if resp.Token == "" {
		t.Errorf("Expected non-empty JWT token")
	}
	if resp.User.Username != "alice" {
		t.Errorf("Expected username alice, got %s", resp.User.Username)
	}
	if resp.User.IdentifierType != models.IdentifierTypeEmail {
		t.Errorf("Expected identifier_type email, got %s", resp.User.IdentifierType)
	}

	// Login with email
	loginReq := models.LoginRequest{
		Identifier: "alice@example.com",
		Password:   "secret123",
	}

	loginResp, err := svc.Login(loginReq)
	if err != nil {
		t.Fatalf("Login with email failed: %v", err)
	}

	if loginResp.Token == "" {
		t.Errorf("Expected non-empty JWT token on login")
	}
	if loginResp.User.ID != resp.User.ID {
		t.Errorf("Expected user ID %s, got %s", resp.User.ID, loginResp.User.ID)
	}
}

func TestRegisterAndLoginWithPhone(t *testing.T) {
	svc, cleanup := setupTestService(t)
	defer cleanup()

	regReq := models.RegisterRequest{
		Username:          "bob",
		Identifier:        "+14155552671",
		Password:          "password456",
		PublicIdentityKey: "pubkey_bob_67890",
	}

	resp, err := svc.Register(regReq)
	if err != nil {
		t.Fatalf("Register with phone failed: %v", err)
	}

	if resp.User.IdentifierType != models.IdentifierTypePhone {
		t.Errorf("Expected identifier_type phone, got %s", resp.User.IdentifierType)
	}

	// Login with phone
	loginReq := models.LoginRequest{
		Identifier: "+14155552671",
		Password:   "password456",
	}

	loginResp, err := svc.Login(loginReq)
	if err != nil {
		t.Fatalf("Login with phone failed: %v", err)
	}

	if loginResp.User.ID != resp.User.ID {
		t.Errorf("Expected user ID %s, got %s", resp.User.ID, loginResp.User.ID)
	}
}

func TestRegisterDuplicateUser(t *testing.T) {
	svc, cleanup := setupTestService(t)
	defer cleanup()

	regReq := models.RegisterRequest{
		Username:          "carol",
		Identifier:        "carol@example.com",
		Password:          "password123",
		PublicIdentityKey: "pubkey_carol",
	}

	_, err := svc.Register(regReq)
	if err != nil {
		t.Fatalf("First registration failed: %v", err)
	}

	// Duplicate email registration
	_, err = svc.Register(models.RegisterRequest{
		Username:          "carol2",
		Identifier:        "carol@example.com",
		Password:          "password123",
		PublicIdentityKey: "pubkey_carol2",
	})
	if err == nil {
		t.Errorf("Expected error registering duplicate email, got nil")
	}

	// Duplicate username registration
	_, err = svc.Register(models.RegisterRequest{
		Username:          "carol",
		Identifier:        "carol_diff@example.com",
		Password:          "password123",
		PublicIdentityKey: "pubkey_carol3",
	})
	if err == nil {
		t.Errorf("Expected error registering duplicate username, got nil")
	}
}

func TestGetPublicIdentityKey(t *testing.T) {
	svc, cleanup := setupTestService(t)
	defer cleanup()

	regReq := models.RegisterRequest{
		Username:          "dave",
		Identifier:        "dave@example.com",
		Password:          "password123",
		PublicIdentityKey: "pubkey_dave_ed25519",
	}

	_, err := svc.Register(regReq)
	if err != nil {
		t.Fatalf("Registration failed: %v", err)
	}

	keyResp, err := svc.GetPublicIdentityKey("dave")
	if err != nil {
		t.Fatalf("GetPublicIdentityKey failed: %v", err)
	}

	if keyResp.PublicIdentityKey != "pubkey_dave_ed25519" {
		t.Errorf("Expected public key pubkey_dave_ed25519, got %s", keyResp.PublicIdentityKey)
	}

	_, err = svc.GetPublicIdentityKey("nonexistent")
	if err == nil {
		t.Errorf("Expected error for non-existent user, got nil")
	}
}
