.PHONY: up down build test test-integration seed run-all loadgen lint

up:            ## full stack: stores + all services, seeded
	docker compose up --build

down:
	docker compose down -v

deps:          ## just the backing stores, for native dev
	docker compose up -d postgres redis jaeger

seed:
	go run ./cmd/seed

build:
	go build ./...

test:
	go test ./...

test-integration:
	go test -tags integration ./...

loadgen:
	go run ./tools/loadgen -target http://localhost:8080 -rps 50 -duration 30s

lint:
	go vet ./...
