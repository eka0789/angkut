SHELL := /bin/bash
.DEFAULT_GOAL := help

GO          ?= go
BIN_DIR     ?= bin
API_BIN     := $(BIN_DIR)/angkut-api
WORKER_BIN  := $(BIN_DIR)/angkut-worker
MIGRATE     := $(GO) run ./api/cmd/migrate

.PHONY: help up down logs ps migrate-up migrate-down migrate-status migrate-create api worker web test lint build tidy clean

help: ## Tampilkan daftar perintah
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN{FS=":.*?## "}{printf "  \033[36m%-16s\033[0m %s\n", $$1, $$2}'

up: ## Start dependensi dev (Docker Compose)
	docker compose up -d

down: ## Stop dependensi dev
	docker compose down

logs: ## Tail log dependensi dev
	docker compose logs -f --tail=100

ps: ## Status container
	docker compose ps

migrate-up: ## Terapkan semua migrasi (runner bawaan, tanpa CLI eksternal)
	$(MIGRATE) up

migrate-down: ## Batalkan 1 migrasi terakhir
	$(MIGRATE) down 1

migrate-status: ## Tampilkan versi skema & migrasi yang belum diterapkan
	$(MIGRATE) status

migrate-create: ## Buat pasangan berkas migrasi: make migrate-create name=create_x
	@test -n "$(name)" || (echo "usage: make migrate-create name=create_x"; exit 1)
	@last=$$(ls migrations/*.up.sql 2>/dev/null | sed 's/.*\/\([0-9]\{4\}\)_.*/\1/' | sort -n | tail -1); \
	next=$$(printf '%04d' $$(( $${last:-0} + 1 ))); \
	echo "-- $$next $(name) (up)" > migrations/$${next}_$(name).up.sql; \
	echo "-- $$next $(name) (down)" > migrations/$${next}_$(name).down.sql; \
	echo "dibuat: migrations/$${next}_$(name).{up,down}.sql (isi SQL-nya sebelum commit)"

api: ## Jalankan API server
	$(GO) run ./api/cmd/api

worker: ## Jalankan background worker
	$(GO) run ./worker/cmd/worker

web: ## Jalankan frontend Nuxt
	cd web && npm run dev

test: ## Unit test Go
	$(GO) test ./... -race -count=1

lint: ## Lint Go + Web
	golangci-lint run ./...
	cd web && npm run lint --if-present

build: ## Build binary api & worker
	$(GO) build -trimpath -ldflags "-s -w" -o $(API_BIN) ./api/cmd/api
	$(GO) build -trimpath -ldflags "-s -w" -o $(WORKER_BIN) ./worker/cmd/worker

tidy: ## go mod tidy
	$(GO) mod tidy

clean: ## Hapus artefak build
	rm -rf $(BIN_DIR)