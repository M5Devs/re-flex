package main

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/coder/websocket"
	"github.com/google/uuid"

	"reflex-server/internal/models"
)

type AuthState struct {
	Token    string
	Username string
}

func main() {
	reader := bufio.NewReader(os.Stdin)

	fmt.Println("==================================================")
	fmt.Println("             Re:Flex Interactive CLI              ")
	fmt.Println("==================================================")

	// 1. Prompt Server URL
	fmt.Print("Enter Server URL [http://localhost:8080]: ")
	serverURL, _ := reader.ReadString('\n')
	serverURL = strings.TrimSpace(serverURL)
	if serverURL == "" {
		serverURL = "http://localhost:8080"
	}
	serverURL = strings.TrimRight(serverURL, "/")

	// 2. Auth: Register / Login
	auth := promptAuth(reader, serverURL)

	// 3. Connect to WebSocket
	wsURL := convertToWSURL(serverURL) + "/ws?token=" + auth.Token
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	conn, _, err := websocket.Dial(ctx, wsURL, nil)
	if err != nil {
		log.Fatalf("Failed to connect to WebSocket gateway at %s: %v", wsURL, err)
	}
	defer conn.Close(websocket.StatusNormalClosure, "cli closing")

	fmt.Printf("\n[System] Connected to WebSocket gateway as @%s\n", auth.Username)

	// Background WS listener
	go func() {
		for {
			_, message, err := conn.Read(ctx)
			if err != nil {
				if ctx.Err() == nil {
					fmt.Printf("\n[System] WebSocket disconnected: %v\n", err)
				}
				return
			}

			var frame models.WSFrame
			if err := json.Unmarshal(message, &frame); err != nil {
				continue
			}

			switch frame.Type {
			case "direct_message":
				fmt.Printf("\n[%s]: %v\n> ", frame.SenderUsername, frame.Payload)
			case "ack_delivered":
				fmt.Printf("\n[System]: Message %s delivered to @%s\n> ", frame.ClientMsgID, frame.RecipientUsername)
			case "ack_queued":
				fmt.Printf("\n[System]: Recipient @%s offline. Message %s queued.\n> ", frame.RecipientUsername, frame.ClientMsgID)
			case "error":
				fmt.Printf("\n[System Error]: %v\n> ", frame.Payload)
			}
		}
	}()

	// 4. Prompt target username to chat with
	fmt.Print("\nEnter target username to chat with: ")
	targetUsername, _ := reader.ReadString('\n')
	targetUsername = strings.TrimSpace(targetUsername)

	fmt.Println("--------------------------------------------------")
	fmt.Printf("Chat session started with @%s.\n", targetUsername)
	fmt.Println("Type message and press Enter. Commands: /target <username>, /quit")
	fmt.Println("--------------------------------------------------")

	for {
		fmt.Print("> ")
		line, err := reader.ReadString('\n')
		if err != nil {
			break
		}
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		if line == "/quit" || line == "/exit" {
			fmt.Println("[System] Exiting CLI.")
			break
		}

		if strings.HasPrefix(line, "/target ") {
			newTarget := strings.TrimSpace(strings.TrimPrefix(line, "/target "))
			if newTarget != "" {
				targetUsername = newTarget
				fmt.Printf("[System] Now chatting with @%s\n", targetUsername)
			}
			continue
		}

		clientMsgID := uuid.New().String()
		dmFrame := models.WSFrame{
			Type:              "direct_message",
			RecipientUsername: targetUsername,
			ClientMsgID:       clientMsgID,
			Payload:           line,
		}

		data, err := json.Marshal(dmFrame)
		if err != nil {
			fmt.Printf("[System Error] Failed to encode message: %v\n", err)
			continue
		}

		writeCtx, writeCancel := context.WithTimeout(ctx, 5*time.Second)
		err = conn.Write(writeCtx, websocket.MessageText, data)
		writeCancel()
		if err != nil {
			fmt.Printf("[System Error] Failed to send message: %v\n", err)
		}
	}
}

func promptAuth(reader *bufio.Reader, serverURL string) AuthState {
	for {
		fmt.Println("\nSelect Authentication Option:")
		fmt.Println("1. Register new user")
		fmt.Println("2. Login existing user")
		fmt.Print("Option (1 or 2): ")

		choice, _ := reader.ReadString('\n')
		choice = strings.TrimSpace(choice)

		if choice == "1" {
			auth, err := doRegister(reader, serverURL)
			if err != nil {
				fmt.Printf("[Auth Error] Registration failed: %v\n", err)
				continue
			}
			return auth
		} else if choice == "2" {
			auth, err := doLogin(reader, serverURL)
			if err != nil {
				fmt.Printf("[Auth Error] Login failed: %v\n", err)
				continue
			}
			return auth
		} else {
			fmt.Println("Invalid choice. Please enter 1 or 2.")
		}
	}
}

func doRegister(reader *bufio.Reader, serverURL string) (AuthState, error) {
	fmt.Print("Username: ")
	username, _ := reader.ReadString('\n')
	username = strings.TrimSpace(username)

	fmt.Print("Identifier (email or phone, e.g. alice@example.com or +14155552671): ")
	identifier, _ := reader.ReadString('\n')
	identifier = strings.TrimSpace(identifier)

	fmt.Print("Password: ")
	password, _ := reader.ReadString('\n')
	password = strings.TrimSpace(password)

	keyBytes := make([]byte, 32)
	_, _ = rand.Read(keyBytes)
	pubKey := base64.StdEncoding.EncodeToString(keyBytes)

	reqBody := models.RegisterRequest{
		Username:          username,
		Identifier:        identifier,
		Password:          password,
		PublicIdentityKey: pubKey,
	}

	bodyBytes, _ := json.Marshal(reqBody)
	resp, err := http.Post(serverURL+"/api/v1/auth/register", "application/json", bytes.NewBuffer(bodyBytes))
	if err != nil {
		return AuthState{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		respData, _ := io.ReadAll(resp.Body)
		return AuthState{}, fmt.Errorf("server status %d: %s", resp.StatusCode, string(respData))
	}

	var authResp models.AuthResponse
	if err := json.NewDecoder(resp.Body).Decode(&authResp); err != nil {
		return AuthState{}, err
	}

	fmt.Printf("[Auth] Registration successful! Welcome @%s\n", authResp.User.Username)
	return AuthState{
		Token:    authResp.Token,
		Username: authResp.User.Username,
	}, nil
}

func doLogin(reader *bufio.Reader, serverURL string) (AuthState, error) {
	fmt.Print("Identifier (email or phone): ")
	identifier, _ := reader.ReadString('\n')
	identifier = strings.TrimSpace(identifier)

	fmt.Print("Password: ")
	password, _ := reader.ReadString('\n')
	password = strings.TrimSpace(password)

	reqBody := models.LoginRequest{
		Identifier: identifier,
		Password:   password,
	}

	bodyBytes, _ := json.Marshal(reqBody)
	resp, err := http.Post(serverURL+"/api/v1/auth/login", "application/json", bytes.NewBuffer(bodyBytes))
	if err != nil {
		return AuthState{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respData, _ := io.ReadAll(resp.Body)
		return AuthState{}, fmt.Errorf("server status %d: %s", resp.StatusCode, string(respData))
	}

	var authResp models.AuthResponse
	if err := json.NewDecoder(resp.Body).Decode(&authResp); err != nil {
		return AuthState{}, err
	}

	fmt.Printf("[Auth] Login successful! Welcome @%s\n", authResp.User.Username)
	return AuthState{
		Token:    authResp.Token,
		Username: authResp.User.Username,
	}, nil
}

func convertToWSURL(httpURL string) string {
	if strings.HasPrefix(httpURL, "https://") {
		return "wss://" + strings.TrimPrefix(httpURL, "https://")
	}
	if strings.HasPrefix(httpURL, "http://") {
		return "ws://" + strings.TrimPrefix(httpURL, "http://")
	}
	return "ws://" + httpURL
}
