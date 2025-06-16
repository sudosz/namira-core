FROM golang:1.24-alpine AS builder
RUN apk add --no-cache git openssh-client
WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

# Build the app from the cmd/rayping package
RUN go build -o rayping ./cmd/rayping

FROM alpine:latest
RUN apk add --no-cache git openssh-client ca-certificates
WORKDIR /app

# Create keys directory
RUN mkdir -p /app/keys

# Copy binary
COPY --from=builder /app/rayping .

# Use a non-root user for security 
RUN adduser -D -g '' raypinguser
USER raypinguser

EXPOSE 8080

ENTRYPOINT ["./rayping"]
