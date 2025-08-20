.PHONY: build run test clean docker-build docker-up docker-down docker-logs

# Go commands
build:
	go build -o relay cmd/server/main.go

run:
	go run cmd/server/main.go

test:
	go test ./...

clean:
	rm -f relay
	rm -rf data/

# Docker commands
docker-build:
	docker build -t relay .

docker-up:
	docker-compose up -d

docker-down:
	docker-compose down

docker-logs:
	docker-compose logs -f relay

docker-reset:
	docker-compose down -v
	rm -rf data/ credentials/token.json

# Development
dev:
	docker-compose -f docker-compose.yml -f docker-compose.dev.yml up

# Database
db-init:
	mysql -u root -p < schema.sql

# Dependencies
deps:
	go mod download
	go mod tidy

# Linting
lint:
	golangci-lint run

# Format code
fmt:
	go fmt ./...

# All-in-one commands
setup: deps db-init
	cp .env.example .env
	mkdir -p credentials data/queue

docker-setup: docker-build
	cp .env.example .env
	mkdir -p credentials