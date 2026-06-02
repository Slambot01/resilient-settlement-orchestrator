.PHONY: all build test run lint docker-up docker-down clean migrate-up migrate-down

# Variables
BINARY_NAME=payment-orchestrator
MAIN_PATH=./cmd/server
DB_URL=postgres://payment_user:payment_secret@localhost:5432/payment_orchestrator?sslmode=disable
MIGRATIONS_PATH=./migrations

all: lint test build

build:
	@echo "Building binary..."
	go build -o $(BINARY_NAME) $(MAIN_PATH)

test:
	@echo "Running tests..."
	go test -v -race -cover ./...

lint:
	@echo "Running linter..."
	golangci-lint run

run:
	@echo "Starting server..."
	go run $(MAIN_PATH)

docker-up:
	@echo "Starting Docker stack..."
	docker compose up -d

docker-down:
	@echo "Stopping Docker stack..."
	docker compose down

migrate-up:
	@echo "Running migrations up..."
	migrate -path $(MIGRATIONS_PATH) -database "$(DB_URL)" up

migrate-down:
	@echo "Running migrations down..."
	migrate -path $(MIGRATIONS_PATH) -database "$(DB_URL)" down 1

clean:
	@echo "Cleaning up..."
	go clean
	rm -f $(BINARY_NAME)
