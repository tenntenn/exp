.PHONY: help install build-frontend build-backend build run clean dev

help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-20s\033[0m %s\n", $$1, $$2}'

install: ## Install dependencies
	@echo "Installing Go dependencies..."
	go mod tidy
	@echo "Installing frontend dependencies..."
	cd frontend && npm install

build-frontend: ## Build frontend
	@echo "Building frontend..."
	cd frontend && npm run build
	@echo "Copying frontend build to server..."
	cp -r frontend/dist cmd/server/frontend/

build-backend: ## Build backend
	@echo "Building backend..."
	go build -o bin/server ./cmd/server

build: build-frontend build-backend ## Build both frontend and backend

run: ## Run the production server
	@echo "Starting server on http://localhost:8080"
	./bin/server

dev-frontend: ## Run frontend dev server
	cd frontend && npm run dev

dev-backend: ## Run backend dev server
	go run ./cmd/server

clean: ## Clean build artifacts
	@echo "Cleaning build artifacts..."
	rm -rf bin/
	rm -rf frontend/dist/
	rm -rf cmd/server/frontend/dist/

.DEFAULT_GOAL := help
