# NOFX Makefile for testing and development

# dev-offline 送测栈 override（容器名 nofx-dev-offline / nofx-dev-offline-backend、数据卷与密钥沿用）
NOFX_DEV_OVERRIDE ?= /Users/hsikey/Trading/nofx/.nofx-local/dev-offline/compose.override.yml
COMPOSE_DEV = docker compose -f docker-compose.yml -f $(NOFX_DEV_OVERRIDE)
NOFX_FRONTEND_PORT ?= 1188

.PHONY: help test test-backend test-frontend test-coverage clean ci-local up

# Default target
help:
	@echo "NOFX Testing & Development Commands"
	@echo ""
	@echo "Testing:"
	@echo "  make test                 - Run all tests (backend + frontend)"
	@echo "  make test-backend         - Run backend tests only"
	@echo "  make test-frontend        - Run frontend tests only"
	@echo "  make test-coverage        - Generate backend coverage report"
	@echo ""
	@echo "Build:"
	@echo "  make build                - Build backend binary"
	@echo "  make build-frontend       - Build frontend"
	@echo ""
	@echo "Clean:"
	@echo "  make clean                - Clean build artifacts and test cache"

# =============================================================================
# Testing
# =============================================================================

# Run all tests
test:
	@echo "🧪 Running backend tests..."
	go test -v ./...
	@echo ""
	@echo "🧪 Running frontend tests..."
	cd web && npm run test
	@echo "✅ All tests completed"

# Backend tests only
test-backend:
	@echo "🧪 Running backend tests..."
	go test -v ./...

# Frontend tests only
test-frontend:
	@echo "🧪 Running frontend tests..."
	cd web && npm run test

# Coverage report
test-coverage:
	@echo "📊 Generating coverage..."
	go test -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html
	@echo "✅ Backend coverage: coverage.html"

# =============================================================================
# Build
# =============================================================================

# Build backend binary
build:
	@echo "🔨 Building backend..."
	go build -o nofx
	@echo "✅ Backend built: ./nofx"

# Build frontend
build-frontend:
	@echo "🔨 Building frontend..."
	cd web && npm run build
	@echo "✅ Frontend built: ./web/dist"

# =============================================================================
# Development
# =============================================================================

# Run backend in development mode
run:
	@echo "🚀 Starting backend..."
	go run main.go

# Run frontend in development mode
run-frontend:
	@echo "🚀 Starting frontend dev server..."
	cd web && npm run dev

# Format Go code
fmt:
	@echo "🎨 Formatting Go code..."
	go fmt ./...
	@echo "✅ Code formatted"

# Lint Go code (requires golangci-lint)
lint:
	@echo "🔍 Linting Go code..."
	golangci-lint run
	@echo "✅ Linting completed"

# =============================================================================
# Clean
# =============================================================================

clean:
	@echo "🧹 Cleaning..."
	rm -f nofx
	rm -f coverage.out coverage.html
	rm -rf web/dist
	go clean -testcache
	@echo "✅ Cleaned"

# =============================================================================
# Docker
# =============================================================================

# Build Docker images
docker-build:
	@echo "🐳 Building Docker images..."
	docker compose build
	@echo "✅ Docker images built"

# Run Docker containers
docker-up:
	@echo "🐳 Starting Docker containers..."
	docker compose up -d
	@echo "✅ Docker containers started"

# Stop Docker containers
docker-down:
	@echo "🐳 Stopping Docker containers..."
	docker compose down
	@echo "✅ Docker containers stopped"

# View Docker logs
docker-logs:
	docker compose logs -f

# =============================================================================
# Dependencies
# =============================================================================

# Download Go dependencies
deps:
	@echo "📦 Downloading Go dependencies..."
	go mod download
	@echo "✅ Dependencies downloaded"

# Update Go dependencies
deps-update:
	@echo "📦 Updating Go dependencies..."
	go get -u ./...
	go mod tidy
	@echo "✅ Dependencies updated"

# Install frontend dependencies
deps-frontend:
	@echo "📦 Installing frontend dependencies..."
	cd web && npm install
	@echo "✅ Frontend dependencies installed"

# =============================================================================
# Local CI & delivery（送测固件；宿主无 Go，go 门禁走 docker golang:1.25-alpine）
# =============================================================================

# 本地完整门禁：回归 + 构建 + 镜像构建（候选 push 前必须全绿）
ci-local:
	@echo "🧪 [1/5] Backend tests (docker golang:1.25-alpine)..."
	docker run --rm -v "$$PWD":/app -v nofx-gocache:/go -w /app golang:1.25-alpine sh -c "apk add --no-cache gcc musl-dev >/dev/null 2>&1 && CGO_ENABLED=1 GOPROXY=https://goproxy.cn,direct go test -count=1 ./..."
	@echo "🧪 [2/5] Frontend tests..."
	cd web && npm run test
	@echo "🔨 [3/5] Backend build..."
	docker run --rm -v "$$PWD":/app -v nofx-gocache:/go -w /app golang:1.25-alpine sh -c "apk add --no-cache gcc musl-dev >/dev/null 2>&1 && CGO_ENABLED=1 GOPROXY=https://goproxy.cn,direct go build -o nofx ."
	@echo "🔨 [4/5] Frontend build..."
	cd web && npm run build
	@echo "🐳 [5/5] Docker images build (nofx-dev-offline 送测栈)..."
	$(COMPOSE_DEV) build --build-arg GOPROXY=https://goproxy.cn,direct
	@echo "✅ ci-local passed"

# 送测启动：在固定容器 nofx-dev-offline / nofx-dev-offline-backend 上重建（数据卷沿用），
# 迁移随启动执行；后端无宿主端口，健康检查走前端 nginx 代理
up:
	$(COMPOSE_DEV) build --build-arg GOPROXY=https://goproxy.cn,direct
	$(COMPOSE_DEV) up -d
	@echo "⏳ Waiting for services (startup runs DB migrations)..."
	sleep 15
	$(COMPOSE_DEV) ps
	@echo "❤️ Backend health via frontend proxy..."
	curl -sf "http://127.0.0.1:$(NOFX_FRONTEND_PORT)/api/health" > /dev/null && echo "backend OK"
	@echo "❤️ Frontend entry (http://127.0.0.1:$(NOFX_FRONTEND_PORT))..."
	curl -sf "http://127.0.0.1:$(NOFX_FRONTEND_PORT)/" > /dev/null && echo "frontend OK"
	@echo "✅ up completed"
