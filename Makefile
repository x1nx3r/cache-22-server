.PHONY: up down nuke build logs ps run test vet fmt tidy psql health scan db-up gen css

gen:
	templ generate

css:
	./bin/tailwindcss -i static/src/input.css -o static/app.css --minify

up:
	docker compose up -d --build

down:
	docker compose down

nuke:
	docker compose down -v

build:
	docker compose build

logs:
	docker compose logs -f

ps:
	docker compose ps

db-up:
	docker compose up -d postgres

run: gen
	go run ./cmd/server

test:
	go test ./...

vet:
	go vet ./...

fmt:
	go fmt ./...

tidy:
	go mod tidy

psql:
	docker compose exec postgres psql -U cache22 -d cache22

health:
	curl -s http://localhost:8080/v1/health; echo

scan:
	curl -s -X POST -H "Authorization: Bearer $${TOKEN:?set TOKEN to a logged-in user token}" http://localhost:8080/v1/scan; echo
