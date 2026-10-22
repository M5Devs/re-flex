package database

import (
	"os"
	"testing"
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
}
