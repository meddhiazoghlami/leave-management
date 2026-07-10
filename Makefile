# Leave Management — common tasks. Run `make` (or `make help`) to list targets.

# Overridable config (e.g. `make migrate-up DATABASE_URL=...`)
DATABASE_URL ?= postgres://postgres:postgres@localhost:5432/leave_management?sslmode=disable
MIGRATIONS   := sql/migrations
BIN          := bin/leave-management

.DEFAULT_GOAL := help

# ---- Help ----

.PHONY: help
help: ## Show available targets
	@grep -hE '^[a-zA-Z0-9_-]+:.*?## .*$$' $(MAKEFILE_LIST) \
		| sort \
		| awk 'BEGIN{FS=":.*?## "}{printf "  \033[36m%-18s\033[0m %s\n", $$1, $$2}'

# ---- Setup ----

.PHONY: setup
setup: install migrate-up seed assets ## First-time setup: deps, migrate, seed, build assets
	@echo "✔ setup complete — run 'make run', then open http://localhost:8080"

.PHONY: install
install: ## Install Go and npm dependencies
	go mod download
	cd web && npm install

.PHONY: db-docker
db-docker: ## Start a Postgres 16 container and create the database
	docker run --name leave-pg -e POSTGRES_PASSWORD=postgres -p 5432:5432 -d postgres:16
	@sleep 2
	docker exec leave-pg createdb -U postgres leave_management || true

# ---- Run / build ----

.PHONY: run
run: ## Run the server (expects assets already built)
	go run .

.PHONY: build
build: generate assets ## Regenerate code, build assets, compile the binary to bin/
	go build -o $(BIN) .

.PHONY: web-dev
web-dev: ## Vite dev server with HMR (pair with `make serve-dev` in another terminal)
	cd web && npm run dev

.PHONY: serve-dev
serve-dev: ## Run Go in dev mode (assets served from the Vite dev server)
	VITE_DEV=true go run .

# ---- Code generation ----

.PHONY: generate
generate: sqlc templ ## Regenerate sqlc + templ code

.PHONY: sqlc
sqlc: ## Regenerate the typed DB layer from sql/
	sqlc generate

.PHONY: templ
templ: ## Regenerate templ components
	templ generate

.PHONY: assets
assets: ## Build front-end assets with Vite
	cd web && npm run build

# ---- Database ----

.PHONY: migrate-up
migrate-up: ## Apply all up migrations
	migrate -path $(MIGRATIONS) -database "$(DATABASE_URL)" up

.PHONY: migrate-down
migrate-down: ## Roll back the last migration
	migrate -path $(MIGRATIONS) -database "$(DATABASE_URL)" down 1

.PHONY: migrate-version
migrate-version: ## Print the current migration version
	migrate -path $(MIGRATIONS) -database "$(DATABASE_URL)" version

.PHONY: migrate-create
migrate-create: ## Create a new migration pair (usage: make migrate-create name=add_widgets)
	migrate create -ext sql -dir $(MIGRATIONS) -seq $(name)

.PHONY: seed
seed: ## Seed demo data (employees, leave types, holidays, sample requests)
	go run ./cmd/seed

# ---- Quality ----

.PHONY: check
check: generate vet test ## Regenerate, vet, and test — the pre-commit sweep

.PHONY: test
test: ## Run tests (the store integration test is skipped without a DB)
	go test ./...

.PHONY: test-integration
test-integration: ## Run all tests incl. the DB-gated store integration test
	TEST_DATABASE_URL="$(DATABASE_URL)" go test ./...

.PHONY: vet
vet: ## Run go vet
	go vet ./...

.PHONY: fmt
fmt: ## Format Go code
	go fmt ./...

.PHONY: tidy
tidy: ## Tidy go.mod / go.sum
	go mod tidy

# ---- Cleanup ----

.PHONY: clean
clean: ## Remove build artifacts (bin/, public/build/)
	rm -rf bin public/build
