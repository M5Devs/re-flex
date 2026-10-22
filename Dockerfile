# Build stage
FROM golang:alpine AS builder

WORKDIR /app

# Install build dependencies
RUN apk add --no-cache gcc musl-dev

# Copy module files and download dependencies
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build statically compiled binary
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-w -s" -o /app/reflex-server ./cmd/reflex-server

# Final lightweight stage
FROM alpine:3.21

RUN apk add --no-cache ca-certificates tzdata

WORKDIR /app

# Create data directory for SQLite persistent storage
RUN mkdir -p /app/data

# Copy binary from builder stage
COPY --from=builder /app/reflex-server /app/reflex-server

EXPOSE 8080

ENV PORT=8080 \
    JWT_SECRET="default-secret-key-change-in-production" \
    DB_PATH="/app/data/reflex.db"

ENTRYPOINT ["/app/reflex-server"]
