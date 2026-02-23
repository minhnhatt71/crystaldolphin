BINARY   := crystaldolphin
MODULE   := github.com/crystaldolphin/crystaldolphin
BUILD_FLAGS := -ldflags="-s -w"

.PHONY: all build run dev clean bridge bridge-dev docker docker-up docker-down tidy

# Default: build everything
all: build bridge

## Go binary
build:
	CGO_ENABLED=0 go build $(BUILD_FLAGS) -o $(BINARY) ./main.go

run: build
	./$(BINARY)

dev:
	go run ./main.go

## WhatsApp bridge (Node / TypeScript)
bridge:
	cd bridge && npm install && npm run build

bridge-dev:
	cd bridge && npm run dev

## Docker
docker:
	docker compose build

docker-up:
	docker compose up -d

docker-down:
	docker compose down

## Utilities
tidy:
	go mod tidy

clean:
	rm -f $(BINARY)
	rm -rf bridge/dist
