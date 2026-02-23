BINARY   := crystaldolphin
MODULE   := github.com/crystaldolphin/crystaldolphin
BUILD_FLAGS := -ldflags="-s -w"
BUILD_DIR := build

.PHONY: all build run dev clean bridge bridge-dev docker docker-up docker-down tidy

# Default: build everything
all: build bridge

## Go binary
build:
	mkdir -p $(BUILD_DIR)
	CGO_ENABLED=0 go build $(BUILD_FLAGS) -o $(BUILD_DIR)/$(BINARY) ./main.go

run: build
	./$(BUILD_DIR)/$(BINARY)

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
	rm -rf $(BUILD_DIR)
	rm -rf bridge/dist
