FROM golang:1.24-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

# Build the app from the cmd/rayping package
RUN go build -o rayping ./cmd/rayping

FROM alpine:latest

WORKDIR /app

COPY --from=builder /app/rayping .

# Use a non-root user for security 
RUN adduser -D -g '' raypinguser
USER raypinguser

EXPOSE 8080

ENTRYPOINT ["./rayping"]
