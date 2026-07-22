# Leave Management — common tasks. Run `make` (or `make help`) to list targets.

# Load a local .env (KEY=value, unquoted) if present and export its vars to
# recipes, so `make migrate-up` picks up DATABASE_URL just like the app does.
# No hardcoded fallback — an unset DATABASE_URL makes the migrate/DB targets fail.
-include .env
export

MIGRATIONS := sql/migrations
BIN        := bin/leave-management

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
	go run . serve

.PHONY: build
build: generate assets ## Regenerate code, build assets, compile the binary to bin/
	go build -o $(BIN) .

.PHONY: web-dev
web-dev: ## Vite dev server with HMR (pair with `make serve-dev` in another terminal)
	cd web && npm run dev

.PHONY: serve-dev
serve-dev: ## Run Go in dev mode (assets served from the Vite dev server)
	VITE_DEV=true go run . serve

# ---- Code generation ----

.PHONY: generate
generate: sqlc templ wire ## Regenerate sqlc + templ + wire code

.PHONY: sqlc
sqlc: ## Regenerate the typed DB layer from sql/
	sqlc generate

.PHONY: templ
templ: ## Regenerate templ components
	templ generate

.PHONY: wire
wire: ## Regenerate the Wire dependency-injection code (internal/app)
	go tool wire gen ./internal/app

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
	go run . seed

# ---- Quality ----

.PHONY: check
check: generate vet test ## Regenerate, vet, and test — the pre-commit sweep

.PHONY: test
test: ## Run all tests, incl. DB integration/e2e via testcontainers (needs Docker)
	go test -p 1 ./...

.PHONY: test-short
test-short: ## Run only the fast unit tests (no Docker / no DB)
	go test -short ./...

.PHONY: cover
cover: ## Full coverage run -> total + coverage.out + coverage.html (needs Docker)
	go test -p 1 -count=1 -coverpkg=./... -covermode=atomic -coverprofile=coverage.out ./...
	@go tool cover -func=coverage.out | tail -1
	@go tool cover -html=coverage.out -o coverage.html
	@echo "✔ wrote coverage.out + coverage.html (open coverage.html for the line-by-line view)"

.PHONY: vet
vet: ## Run go vet
	go vet ./...

.PHONY: fmt
fmt: ## Format Go code
	go fmt ./...

.PHONY: tidy
tidy: ## Tidy go.mod / go.sum
	go mod tidy

# ---- Docker (production-like) ----

.PHONY: docker-up
docker-up: ## Build + start the full stack (db, migrate, app) in the background
	docker compose up -d --build

.PHONY: docker-seed
docker-seed: ## Seed the Dockerized database (one-off, opt-in profile)
	docker compose --profile seed run --rm seed

.PHONY: docker-logs
docker-logs: ## Tail the app container logs
	docker compose logs -f app

.PHONY: docker-down
docker-down: ## Stop the stack (keeps the db volume)
	docker compose down

.PHONY: docker-clean
docker-clean: ## Stop the stack and delete the db volume
	docker compose down -v

# ---- Observability (Prometheus, Grafana, Loki, Tempo) ----

.PHONY: observability-up
observability-up: ## Start ONLY the monitoring backends (pair with a locally-run `go run . serve`)
	docker compose --profile observability up -d prometheus grafana loki tempo
	@echo "✔ Grafana  → http://localhost:$${GRAFANA_PORT:-3000} (admin/admin)"
	@echo "  Prometheus → http://localhost:$${PROMETHEUS_PORT:-9090}"
	@echo "  Now run the app pointed at them:"
	@echo "    OTEL_EXPORTER_OTLP_ENDPOINT=http://localhost:4318 LOKI_URL=http://localhost:3100 make run"

.PHONY: observability-down
observability-down: ## Stop the monitoring stack (keeps its volumes)
	docker compose --profile observability down

.PHONY: observability-logs
observability-logs: ## Tail the monitoring services' logs
	docker compose logs -f prometheus grafana loki tempo postgres_exporter

.PHONY: stack-up
stack-up: ## Build + start EVERYTHING (db, migrate, app, and the monitoring stack)
	docker compose --profile observability up -d --build
	@echo "✔ App      → http://localhost:$${APP_PORT:-8080}"
	@echo "  Grafana  → http://localhost:$${GRAFANA_PORT:-3000} (admin/admin)"

# ---- Cleanup ----

.PHONY: clean
clean: ## Remove build artifacts (bin/, public/build/)
	rm -rf bin public/build
