# Makefile for RayPing

.PHONY: build up down logs clean dev prod health

# Variables
VERSION ?= $(shell git describe --tags --always --dirty)
BUILD_DATE ?= $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")
COMMIT_SHA ?= $(shell git rev-parse --short HEAD)

# Common docker-compose command
DOCKER_COMPOSE := docker-compose
DOCKER_BUILD_ARGS := VERSION=$(VERSION) BUILD_DATE=$(BUILD_DATE) COMMIT_SHA=$(COMMIT_SHA)

# Development
dev:
	@echo "Starting development environment..."
	$(DOCKER_COMPOSE) up --build

# Production
prod:
	@echo "Starting production environment..."
	$(DOCKER_BUILD_ARGS) $(DOCKER_COMPOSE) -f docker-compose.yml -f docker-compose.prod.yml up -d --build

# Build only
build:
	@echo "Building RayPing..."
	$(DOCKER_BUILD_ARGS) $(DOCKER_COMPOSE) build

# Start services
up:
	$(DOCKER_COMPOSE) up -d

# Stop services
down:
	$(DOCKER_COMPOSE) down

# View logs
logs:
	$(DOCKER_COMPOSE) logs -f rayping

# Clean up
clean:
	$(DOCKER_COMPOSE) down -v --remove-orphans
	docker system prune -f

# Health check
health:
	@echo "Checking service health..."
	@curl -f http://localhost:8080/health || echo "Service not healthy"