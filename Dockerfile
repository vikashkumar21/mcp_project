# Multi-stage Dockerfile for Remote Research MCP Server
FROM golang:1.24-alpine AS builder

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /remote_server ./cmd/remote_server

FROM alpine:latest
WORKDIR /app

# Copy compiled binary and data directories
COPY --from=builder /remote_server .
COPY papers/ ./papers/

ENV PORT=8080
EXPOSE 8080

CMD ["./remote_server"]
