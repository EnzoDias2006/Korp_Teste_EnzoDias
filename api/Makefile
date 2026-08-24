# Makefile for Korp API

.PHONY: help infra-up infra-down infra services services-up services-down run-stock run-billing fmt check-fmt test test-stock test-billing vet build build-stock build-billing docker-build docker-build-stock docker-build-billing migrate-up-stock migrate-down-stock migrate-up-billing migrate-down-billing migrate-up migrate-down

-include .env

# Go parameters
GOCMD=go
GOBUILD=$(GOCMD) build -buildvcs=false
GOTEST=$(GOCMD) test
GOVET=$(GOCMD) vet
GOFMT=gofmt

# Docker parameters
DOCKER=docker
DOCKER_COMPOSE=$(DOCKER) compose
MIGRATE_IMAGE=migrate/migrate:v4.17.0

# Service paths
STOCK_DIR=services/stock
BILLING_DIR=services/billing
STOCK_MIGRATIONS=$(STOCK_DIR)/migrations
BILLING_MIGRATIONS=$(BILLING_DIR)/migrations

# Database URLs (from environment or defaults)
STOCK_DB_URL?=$(STOCK_MIGRATION_DATABASE_URL)
BILLING_DB_URL?=$(BILLING_MIGRATION_DATABASE_URL)

help:
	@echo "Available targets:"
	@echo "  infra-up          - Start PostgreSQL infrastructure"
	@echo "  infra-down        - Stop PostgreSQL infrastructure"
	@echo "  infra             - Restart PostgreSQL infrastructure"
	@echo "  services          - Start all services"
	@echo "  services-up       - Start all services (infra + app)"
	@echo "  services-down     - Stop all services"
	@echo "  run-stock         - Run Stock locally"
	@echo "  run-billing       - Run Billing locally"
	@echo "  fmt               - Format all Go files"
	@echo "  check-fmt         - Check Go file formatting"
	@echo "  test              - Run all tests"
	@echo "  test-stock        - Run stock service tests"
	@echo "  test-billing      - Run billing service tests"
	@echo "  vet               - Run go vet on all packages"
	@echo "  build             - Build all services"
	@echo "  build-stock       - Build stock service"
	@echo "  build-billing     - Build billing service"
	@echo "  docker-build      - Build Docker images for all services"
	@echo "  migrate-up        - Run migrations up for all services"
	@echo "  migrate-down      - Run migrations down for all services"
	@echo "  migrate-up-stock  - Run stock service migrations up"
	@echo "  migrate-down-stock - Run stock service migrations down"
	@echo "  migrate-up-billing - Run billing service migrations up"
	@echo "  migrate-down-billing - Run billing service migrations down"

# Infrastructure targets
infra-up:
	$(DOCKER_COMPOSE) up -d postgres

infra-down:
	$(DOCKER_COMPOSE) down

infra: infra-down infra-up

# Service targets
services-up:
	$(DOCKER_COMPOSE) up -d

services-down:
	$(DOCKER_COMPOSE) down

services: services-down services-up

run-stock:
	@cd $(STOCK_DIR) && HTTP_ADDR="$(STOCK_HTTP_ADDR)" DATABASE_URL="$(STOCK_DATABASE_URL)" CORS_ALLOWED_ORIGINS="$(CORS_ALLOWED_ORIGINS)" $(GOCMD) run ./cmd/api

run-billing:
	@cd $(BILLING_DIR) && HTTP_ADDR="$(BILLING_HTTP_ADDR)" DATABASE_URL="$(BILLING_DATABASE_URL)" STOCK_SERVICE_URL="$(STOCK_SERVICE_URL)" CORS_ALLOWED_ORIGINS="$(CORS_ALLOWED_ORIGINS)" $(GOCMD) run ./cmd/api

# Formatting targets
fmt:
	@find $(STOCK_DIR) $(BILLING_DIR) -name '*.go' -type f -exec $(GOFMT) -w {} +

check-fmt:
	@unformatted="$$(find $(STOCK_DIR) $(BILLING_DIR) -name '*.go' -type f -exec $(GOFMT) -l {} +)"; \
	if [ -n "$$unformatted" ]; then \
		echo "$$unformatted"; \
		exit 1; \
	fi

# Testing targets
test: test-stock test-billing

test-stock:
	cd $(STOCK_DIR) && $(GOTEST) ./...

test-billing:
	cd $(BILLING_DIR) && $(GOTEST) ./...

# Vet targets
vet:
	cd $(STOCK_DIR) && $(GOVET) ./...
	cd $(BILLING_DIR) && $(GOVET) ./...

# Build targets
build: build-stock build-billing

build-stock:
	cd $(STOCK_DIR) && $(GOBUILD) -o bin/stock ./cmd/api

build-billing:
	cd $(BILLING_DIR) && $(GOBUILD) -o bin/billing ./cmd/api

# Docker build targets
docker-build: docker-build-stock docker-build-billing

docker-build-stock:
	$(DOCKER_COMPOSE) build stock

docker-build-billing:
	$(DOCKER_COMPOSE) build billing

# Migration targets (using golang-migrate Docker image)
MIGRATE_NETWORK?=korp-network
MIGRATE_CMD=$(DOCKER) run --rm --network $(MIGRATE_NETWORK) -v $(CURDIR):/workspace:ro --workdir /workspace $(MIGRATE_IMAGE)

migrate-up-stock:
	@if [ -z "$(wildcard $(STOCK_MIGRATIONS)/*.up.sql)" ]; then \
		echo "No Stock migrations registered yet."; \
	else \
		echo "Running Stock migrations up..."; \
		$(MIGRATE_CMD) -path=$(STOCK_MIGRATIONS) -database="$(STOCK_DB_URL)" up; \
	fi

migrate-down-stock:
	@if [ -z "$(wildcard $(STOCK_MIGRATIONS)/*.down.sql)" ]; then \
		echo "No Stock migrations registered yet."; \
	else \
		echo "Running Stock migrations down..."; \
		$(MIGRATE_CMD) -path=$(STOCK_MIGRATIONS) -database="$(STOCK_DB_URL)" down 1; \
	fi

migrate-up-billing:
	@if [ -z "$(wildcard $(BILLING_MIGRATIONS)/*.up.sql)" ]; then \
		echo "No Billing migrations registered yet."; \
	else \
		echo "Running Billing migrations up..."; \
		$(MIGRATE_CMD) -path=$(BILLING_MIGRATIONS) -database="$(BILLING_DB_URL)" up; \
	fi

migrate-down-billing:
	@if [ -z "$(wildcard $(BILLING_MIGRATIONS)/*.down.sql)" ]; then \
		echo "No Billing migrations registered yet."; \
	else \
		echo "Running Billing migrations down..."; \
		$(MIGRATE_CMD) -path=$(BILLING_MIGRATIONS) -database="$(BILLING_DB_URL)" down 1; \
	fi

migrate-up: migrate-up-stock migrate-up-billing

migrate-down: migrate-down-billing migrate-down-stock
