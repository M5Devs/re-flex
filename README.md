# Re:Flex Server

**Re:Flex** is a lightweight, self-hosted, privacy-first messaging server written in Go with E2EE support and WebSocket real-time gateway.

## Features
- **Dual Authentication**: Register/login with Email (RFC5322) OR Phone number (E.164 format).
- **Public Key Binding**: Stores Ed25519 or Curve25519 public keys during registration for End-to-End Encryption (E2EE).
- **Embedded SQLite**: Zero-dependency database using `modernc.org/sqlite`.
- **Real-time WebSocket Gateway**: Authenticated `/ws` endpoint supporting ping/pong heartbeat and JSON frame messaging.
- **Dockerized**: Multi-stage Dockerfile and Docker Compose with volume persistence.

---

## API Endpoints

### Auth Endpoints (`/api/v1/auth`)

#### `POST /api/v1/auth/register`
Register a new user with an email or phone number and upload their E2EE public identity key.

Request:
```json
{
  "username": "alice",
  "identifier": "alice@example.com", // or "+14155552671"
  "password": "strongpassword123",
  "public_identity_key": "MCowBQYDK2VwAyEA..."
}
```

Response (201 Created):
```json
{
  "token": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...",
  "user": {
    "id": "c620456c-0e78-43db-b864-4424ff433230",
    "username": "alice",
    "identifier_type": "email",
    "identifier_value": "alice@example.com",
    "public_identity_key": "MCowBQYDK2VwAyEA...",
    "created_at": "2025-01-01T00:00:00Z"
  }
}
```

#### `POST /api/v1/auth/login`
Authenticate with email or phone number + password.

Request:
```json
{
  "identifier": "alice@example.com",
  "password": "strongpassword123"
}
```

Response (200 OK):
```json
{
  "token": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...",
  "user": { ... }
}
```

#### `GET /api/v1/users/:username/keys`
Fetch a user's public identity key for initiating E2EE sessions.

Response (200 OK):
```json
{
  "username": "alice",
  "public_identity_key": "MCowBQYDK2VwAyEA..."
}
```

---

## WebSocket Gateway (`/ws`)

Connect with a JWT token passed via query parameter or Authorization header:
- `ws://localhost:8080/ws?token=<JWT_TOKEN>`
- Or Header: `Authorization: Bearer <JWT_TOKEN>`

### WebSocket Frame Format
```json
{
  "type": "ping" | "message" | "ack" | "pong",
  "payload": {}
}
```

---

## Running Locally

### 1. Requirements
- Go 1.22+

### 2. Run
```bash
go run ./cmd/reflex-server
```

### 3. Run Tests
```bash
go test ./...
```

---

## Running with Docker Compose

```bash
docker compose up -d
```
The server will run on port `8080`, saving SQLite data to `./data/reflex.db`.
