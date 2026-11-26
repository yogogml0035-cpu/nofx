# NOFX Makefile for testing and development

# dev-offline 送测栈 override（容器名 nofx-dev-offline / nofx-dev-offline-backend、数据卷与密钥沿用）
NOFX_DEV_OVERRIDE ?= /Users/hsikey/Trading/nofx/.nofx-local/dev-offline/compose.override.yml
COMPOSE_DEV = docker compose -f docker-compose.yml -f $(NOFX_DEV_OVERRIDE)
NOFX_FRONTEND_PORT ?= 1188

# 主栈（生产候选）变量：重建在 main 仓执行；默认路径面向本机主仓，可命令行覆盖
MAIN_STACK_DIR ?= /Users/hsikey/Trading/nofx
MAIN_STACK_COMPOSE ?= $(MAIN_STACK_DIR)/.nofx-local/compose.yml
MAIN_STACK_DATA ?= $(MAIN_STACK_DIR)/.nofx-local/candidate-data/data.db
MAIN_STACK_FRONTEND_PORT ?= 8888
MAIN_STACK_SHA ?=

.PHONY: help test test-backend test-frontend test-coverage clean ci-local up main-stack infra-check

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
	@echo "Delivery gate:"
	@echo "  make ci-local             - Full local gate (tests, build, infra-check, images)"
	@echo ""
	@echo "Deploy & infra:"
	@echo "  make up                   - Rebuild dev-offline test stack (fixed containers)"
	@echo "  make main-stack           - Rebuild main stack from clean checkout (D20 guarded)"
	@echo "  make infra-check          - Validate dockerignore, compose config, healthcheck, context"
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
	@echo "🧪 [1/6] Backend tests (docker golang:1.25-alpine)..."
	docker run --rm -v "$$PWD":/app -v nofx-gocache:/go -w /app golang:1.25-alpine sh -c "apk add --no-cache gcc musl-dev >/dev/null 2>&1 && CGO_ENABLED=1 GOPROXY=https://goproxy.cn,direct go test -count=1 ./..."
	@echo "🧪 [2/6] Frontend tests..."
	cd web && npm run test
	@echo "🔨 [3/6] Backend build..."
	docker run --rm -v "$$PWD":/app -v nofx-gocache:/go -w /app golang:1.25-alpine sh -c "apk add --no-cache gcc musl-dev >/dev/null 2>&1 && CGO_ENABLED=1 GOPROXY=https://goproxy.cn,direct go build -o nofx ."
	@echo "🔨 [4/6] Frontend build (tsc + vite)..."
	cd web && npm run build
	@echo "🔎 [5/6] Infra check (dockerignore / compose / healthcheck / build context)..."
	$(MAKE) infra-check
	@echo "🐳 [6/6] Docker images build (nofx-dev-offline 送测栈)..."
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
	@echo "❤️ Waiting for container health (image HEALTHCHECK)..."
	@for i in $$(seq 1 45); do \
		B=$$($(COMPOSE_DEV) ps -q nofx | xargs docker inspect -f '{{.State.Health.Status}}' 2>/dev/null || true); \
		F=$$($(COMPOSE_DEV) ps -q nofx-frontend | xargs docker inspect -f '{{.State.Health.Status}}' 2>/dev/null || true); \
		if test "$$B" = "healthy" && test "$$F" = "healthy"; then break; fi; \
		sleep 2; \
	done; \
	B=$$($(COMPOSE_DEV) ps -q nofx | xargs docker inspect -f '{{.State.Health.Status}}' 2>/dev/null || true); \
	F=$$($(COMPOSE_DEV) ps -q nofx-frontend | xargs docker inspect -f '{{.State.Health.Status}}' 2>/dev/null || true); \
	test "$$B" = "healthy" && test "$$F" = "healthy" || { echo "❌ 容器未 healthy: nofx=$${B:-unknown} frontend=$${F:-unknown}"; $(COMPOSE_DEV) ps; exit 1; }
	@echo "✅ up completed (containers healthy)"

# =============================================================================
# 主栈重建（合入 main 并推送后执行；D16/D20）
# - 构建上下文来自 git archive 的干净检出，不读取工作区（本地状态/密钥/数据库不进入镜像）
# - 镜像 tag 同步候选 SHA；构建前后与重建后 traders.is_running 必须为 0，查询失败即中止
# =============================================================================
main-stack:
	@set -e; set -o pipefail; \
	MAIN_DIR="$${MAIN_STACK_DIR:-$(MAIN_STACK_DIR)}"; \
	COMPOSE="$${MAIN_STACK_COMPOSE:-$(MAIN_STACK_COMPOSE)}"; \
	DB="$${MAIN_STACK_DATA:-$(MAIN_STACK_DATA)}"; \
	PORT="$${MAIN_STACK_FRONTEND_PORT:-$(MAIN_STACK_FRONTEND_PORT)}"; \
	SHA="$${MAIN_STACK_SHA:-$(MAIN_STACK_SHA)}"; \
	if test -z "$$SHA"; then SHA=$$(git -C "$$MAIN_DIR" rev-parse --short HEAD); fi; \
	test -n "$$SHA" || { echo "❌ 无法确定候选 SHA"; exit 1; }; \
	test -f "$$COMPOSE" || { echo "❌ 缺少主栈 compose: $$COMPOSE"; exit 1; }; \
	test -f "$$DB" || { echo "❌ 缺少主栈数据库: $$DB"; exit 1; }; \
	git -C "$$MAIN_DIR" cat-file -e "$$SHA^{commit}" || { echo "❌ 提交不存在: $$SHA"; exit 1; }; \
	check_running() { \
		local phase cnt names; \
		phase="$$1"; \
		cnt=$$(sqlite3 -readonly -cmd ".timeout 5000" "$$DB" "SELECT COUNT(*) FROM traders WHERE is_running=1"); \
		test "$$cnt" = "0" || { \
			names=$$(sqlite3 -readonly -cmd ".timeout 5000" "$$DB" "SELECT group_concat(id || ':' || name, ', ') FROM traders WHERE is_running=1"); \
			echo "❌ [$${phase}] 运行中的 trader 数量 $${cnt} ≠ 0（$${names}），拒绝继续（D20）"; \
			return 1; \
		}; \
	}; \
	check_running 重建前; \
	echo "🔨 从干净检出构建主栈 $$SHA ..."; \
	TMP=$$(mktemp -d); trap 'rm -rf "$$TMP"' EXIT INT TERM; \
	git -C "$$MAIN_DIR" archive "$$SHA" | tar -x -C "$$TMP"; \
	docker build -f "$$TMP/docker/Dockerfile.backend" -t "nofx-local/backend:$$SHA" --build-arg GOPROXY=https://goproxy.cn,direct "$$TMP"; \
	docker build -f "$$TMP/docker/Dockerfile.frontend" -t "nofx-local/frontend:$$SHA" "$$TMP"; \
	check_running 构建后; \
	sed -i.bak -E "s#(image: nofx-local/backend:).*#\1$$SHA#; s#(image: nofx-local/frontend:).*#\1$$SHA#" "$$COMPOSE"; \
	grep -q "image: nofx-local/backend:$$SHA" "$$COMPOSE" || { echo "❌ backend 镜像 tag 未写入 $$COMPOSE"; exit 1; }; \
	grep -q "image: nofx-local/frontend:$$SHA" "$$COMPOSE" || { echo "❌ frontend 镜像 tag 未写入 $$COMPOSE"; exit 1; }; \
	docker compose -f "$$COMPOSE" config -q; \
	rm -f "$$COMPOSE.bak"; \
	SINCE=$$(date -u +%Y-%m-%dT%H:%M:%SZ); \
	docker compose -f "$$COMPOSE" up -d; \
	echo "⏳ 等待主栈健康检查..."; \
	OK=""; \
	for i in $$(seq 1 60); do \
		if curl -sf --max-time 5 "http://127.0.0.1:$$PORT/api/health" >/dev/null && curl -sf --max-time 5 "http://127.0.0.1:$$PORT/" >/dev/null; then OK=1; break; fi; \
		sleep 2; \
	done; \
	test -n "$$OK" || { echo "❌ 主栈 HTTP 健康检查超时（$${PORT}）"; docker compose -f "$$COMPOSE" ps; exit 1; }; \
	SERVICES=$$(docker compose -f "$$COMPOSE" config --services) || { echo "❌ 无法取得主栈服务列表"; exit 1; }; \
	test -n "$$SERVICES" || { echo "❌ 主栈服务列表为空"; exit 1; }; \
	for svc in $$SERVICES; do \
		HEALTHY=""; STATUS=""; \
		for i in $$(seq 1 45); do \
			CID=$$(docker compose -f "$$COMPOSE" ps -q "$$svc"); \
			STATUS=$$(docker inspect -f '{{.State.Health.Status}}' "$$CID" 2>/dev/null || true); \
			if test "$$STATUS" = "healthy"; then HEALTHY=1; break; fi; \
			sleep 2; \
		done; \
		test -n "$$HEALTHY" || { echo "❌ 容器 $$svc 未达到 healthy（当前: $${STATUS:-unknown}）"; docker compose -f "$$COMPOSE" ps; exit 1; }; \
	done; \
	check_running 重建后; \
	LOGS=$$(docker compose -f "$$COMPOSE" logs --no-color --since "$$SINCE" nofx 2>&1) || { echo "❌ 读取主栈启动日志失败"; exit 1; }; \
	if printf '%s\n' "$$LOGS" | grep -qE "Auto-starting trader|Trader auto-started successfully|Auto-restoring trader runtime"; then \
		echo "❌ 启动日志出现 trader 自动拉起"; \
		printf '%s\n' "$$LOGS" | grep -E "Auto-starting trader|Trader auto-started successfully|Auto-restoring trader runtime"; \
		exit 1; \
	fi; \
	echo "✅ 主栈已重建到 $${SHA}（http://127.0.0.1:$${PORT}），容器 healthy，无 trader 被自动拉起"

# =============================================================================
# 基础设施卫生检查（不启动/停止容器、不写业务数据）
# =============================================================================
infra-check:
	@set -e; set -o pipefail; \
	echo "🔎 [1/5] .dockerignore 必需条目..."; \
	for p in '.nofx-local/' 'scratch/' '.env' 'data/' 'web/node_modules/'; do \
		grep -qx "$$p" .dockerignore || { echo "❌ .dockerignore 缺少 $$p"; exit 1; }; \
	done; \
	echo "🔎 [2/5] compose 配置解析..."; \
	docker compose -f docker-compose.yml config --no-env-resolution -q; \
	if test -f .nofx-local/compose.yml; then docker compose -f .nofx-local/compose.yml config --no-env-resolution -q; fi; \
	echo "🔎 [3/5] 前端健康检查单份且探测 127.0.0.1..."; \
	if sed -n '/^  nofx-frontend:/,/^[a-zA-Z]/p' docker-compose.yml | grep -q 'healthcheck:'; then \
		echo "❌ docker-compose.yml 仍重复声明前端健康检查"; exit 1; \
	fi; \
	awk '/^HEALTHCHECK/{f=1} f&&/CMD/{print; exit}' docker/Dockerfile.frontend | grep -q 'http://127.0.0.1/health' || { echo "❌ Dockerfile.frontend 的 HEALTHCHECK 必须存在并探测 127.0.0.1（musl 将 localhost 解析为 ::1）"; exit 1; }; \
	if test -f .nofx-local/compose.yml; then \
		for svc in nofx frontend; do \
			docker compose -f .nofx-local/compose.yml config --no-env-resolution --services | grep -qx "$$svc" || { echo "❌ 主栈 compose 应包含服务 $${svc}"; exit 1; }; \
		done; \
	fi; \
	echo "🔎 [4/5] 构建上下文不含本地文件（标记目录实测）..."; \
	TMP=$$(mktemp -d); trap 'rm -rf "$$TMP"' EXIT INT TERM; \
	cp .dockerignore "$$TMP/.dockerignore"; \
	mkdir -p "$$TMP/.nofx-local" "$$TMP/scratch" "$$TMP/data" "$$TMP/.trellis"; \
	touch "$$TMP/.env" "$$TMP/.nofx-local/marker" "$$TMP/scratch/marker" "$$TMP/data/marker" "$$TMP/.trellis/marker" "$$TMP/keep.txt"; \
	printf 'FROM alpine:latest\nCOPY . /ctx\nRUN test -e /ctx/keep.txt && test ! -e /ctx/.nofx-local && test ! -e /ctx/scratch && test ! -e /ctx/.env && test ! -e /ctx/data && test ! -e /ctx/.trellis\n' > "$$TMP/Dockerfile"; \
	docker build -q -f "$$TMP/Dockerfile" -t nofx-context-check:tmp "$$TMP" >/dev/null; \
	docker rmi -f nofx-context-check:tmp >/dev/null; \
	echo "🔎 [5/5] Makefile 目标可解析..."; \
	make -n main-stack >/dev/null; \
	echo "✅ infra-check passed"
