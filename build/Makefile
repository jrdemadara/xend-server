SHELL := /bin/bash

ENV_FILE ?= .env
MIGRATIONS_DIR ?= migrations
GOOSE ?= goose
GOOSE_DRIVER ?= postgres
ASYNQMON_PORT ?= 8085

.PHONY: run-api run-worker run-asynqmon run-stack migrate-up migrate-down migrate-status migrate-reset migrate-create

run-api:
	@set -a; source $(ENV_FILE); set +a; \
	go run ./cmd/api

run-worker:
	@set -a; source $(ENV_FILE); set +a; \
	go run ./cmd/worker

run-asynqmon:
	@set -a; source $(ENV_FILE); set +a; \
	asynqmon --port=$(ASYNQMON_PORT) --redis-addr=$${REDIS_ADDR:-127.0.0.1:6379} --redis-password=$${REDIS_PASSWORD} --redis-db=$${REDIS_DB:-0}

run-stack:
	@set -a; source $(ENV_FILE); set +a; \
	(go run ./cmd/worker & go run ./cmd/api & wait)

migrate-up:
	@set -a; source $(ENV_FILE); set +a; \
	$(GOOSE) -dir $(MIGRATIONS_DIR) $(GOOSE_DRIVER) "$$DATABASE_URL" up

migrate-down:
	@set -a; source $(ENV_FILE); set +a; \
	$(GOOSE) -dir $(MIGRATIONS_DIR) $(GOOSE_DRIVER) "$$DATABASE_URL" down

migrate-status:
	@set -a; source $(ENV_FILE); set +a; \
	$(GOOSE) -dir $(MIGRATIONS_DIR) $(GOOSE_DRIVER) "$$DATABASE_URL" status

migrate-reset:
	@set -a; source $(ENV_FILE); set +a; \
	$(GOOSE) -dir $(MIGRATIONS_DIR) $(GOOSE_DRIVER) "$$DATABASE_URL" reset

migrate-create:
	@echo "Usage: make -f build/Makefile migrate-create NAME=create_example_table"
	@test -n "$(NAME)" || (echo "NAME is required" && exit 1)
	@$(GOOSE) -dir $(MIGRATIONS_DIR) create $(NAME) sql
