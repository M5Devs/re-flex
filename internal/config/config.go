package config

import (
	"os"
)

type Config struct {
	Port      string
	JWTSecret string
	DBPath    string
}

func Load() *Config {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	jwtSecret := os.Getenv("JWT_SECRET")
	if jwtSecret == "" {
		jwtSecret = "default-secret-key-change-in-production"
	}

	dbPath := os.Getenv("DB_PATH")
	if dbPath == "" {
		dbPath = "./data/reflex.db"
	}

	return &Config{
		Port:      port,
		JWTSecret: jwtSecret,
		DBPath:    dbPath,
	}
}
