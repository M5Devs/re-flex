package main

import (
	"log"
	"net/http"

	"reflex-server/internal/auth"
	"reflex-server/internal/config"
	"reflex-server/internal/database"
	"reflex-server/internal/gateway"
)

func main() {
	cfg := config.Load()

	db, err := database.InitDB(cfg.DBPath)
	if err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
	}
	defer db.Close()

	tokenManager := auth.NewTokenManager(cfg.JWTSecret)
	authService := auth.NewService(db, tokenManager)
	authHandler := auth.NewHandler(authService)

	hub := gateway.NewHub()
	go hub.Run()
	gatewayHandler := gateway.NewHandler(hub, tokenManager, db)

	mux := http.NewServeMux()

	// REST API v1 routes
	mux.HandleFunc("POST /api/v1/auth/register", authHandler.Register)
	mux.HandleFunc("POST /api/v1/auth/login", authHandler.Login)
	mux.HandleFunc("GET /api/v1/users/{username}/keys", authHandler.GetUserKeys)

	// WebSocket Gateway route
	mux.HandleFunc("/ws", gatewayHandler.ServeWS)

	serverAddr := ":" + cfg.Port
	log.Printf("Re:Flex server starting on %s (DB: %s)...", serverAddr, cfg.DBPath)
	if err := http.ListenAndServe(serverAddr, mux); err != nil {
		log.Fatalf("Server stopped with error: %v", err)
	}
}
