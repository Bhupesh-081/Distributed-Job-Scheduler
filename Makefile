.PHONY: build-all run-compose migrate

build-all:
	go build ./...

run-compose:
	docker compose up -d --build

migrate:
	go run ./cmd/migrate
