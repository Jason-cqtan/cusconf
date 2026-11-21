.PHONY: web build test dev docker image run stop logs

web:
	cd web && npm ci && npm run build

build: web
	CGO_ENABLED=0 go build -trimpath -o cusconf ./cmd/cusconf

test:
	go test ./...
	cd web && npx tsc -b

# Backend on :8080, then run `npm run dev` in web/ for the UI with hot reload (proxies /api).
dev:
	CUSCONF_ADMIN_PASSWORD=admin go run ./cmd/cusconf

docker:
	docker compose build

# --- Apple container (https://github.com/apple/container) ---
# Usage: make image && CUSCONF_ADMIN_PASSWORD=change-me make run
NAME ?= cusconf
PORT ?= 8080
DATA ?= $(CURDIR)/data

image:
	container build -t cusconf:latest .

run:
	mkdir -p $(DATA)
	-container rm -f $(NAME) >/dev/null 2>&1
	container run -d --name $(NAME) -p $(PORT):8080 -v $(DATA):/data \
		-e CUSCONF_ADMIN_PASSWORD=$(CUSCONF_ADMIN_PASSWORD) \
		-e CUSCONF_BASE_URL=$(CUSCONF_BASE_URL) \
		-e TZ=Asia/Shanghai \
		cusconf:latest

stop:
	container stop $(NAME)

logs:
	container logs -f $(NAME)
