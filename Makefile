.PHONY: dev dev-ui build build-linux parity web_build certs test scrape-once deploy verify clean

# ---- config ----
BACKEND      ?= app/backend
FRONTEND     ?= dashboard/web/frontend
WEBROOT      ?= $(BACKEND)/internal/webui/webroot
ADDR         ?= :8444
DB_PATH      ?= ./.data/oilfield.db
TLS_CERT     ?= ./.data/certs/cert.pem
TLS_KEY      ?= ./.data/certs/key.pem
DEPLOY_HOST  ?= root@217.76.158.90
DEPLOY_DIR   ?= /opt/oilfield

# Load local secrets (EIA_API_KEY, etc.) if present.
-include .env
export

# ---- local dev ----
certs:
	@mkdir -p ./.data/certs
	@if [ ! -f $(TLS_CERT) ]; then \
		openssl req -x509 -newkey rsa:4096 -keyout $(TLS_KEY) -out $(TLS_CERT) -days 365 -nodes -subj "/CN=localhost"; \
		echo "Self-signed cert generated at $(TLS_CERT)"; \
	fi

# Run the full single binary locally (serves API + embedded frontend on :8444).
# Run `make web_build` first so the embedded frontend is current.
dev: certs
	cd $(BACKEND) && ADDR=$(ADDR) DB_PATH=$(abspath $(DB_PATH)) \
		TLS_CERT=$(abspath $(TLS_CERT)) TLS_KEY=$(abspath $(TLS_KEY)) \
		STATIC_DIR=$(abspath $(FRONTEND)/dist) \
		go run ./cmd/oilfield

# Vite hot-reload UI on :5173 (proxies /api to https://localhost:8444).
dev-ui:
	cd $(FRONTEND) && npm run dev

scrape-once: 
	cd $(BACKEND) && DB_PATH=$(abspath $(DB_PATH)) go run ./cmd/oilfield --scrape-once

# ---- build ----
web_build:
	cd $(FRONTEND) && npm run build
	rm -rf $(WEBROOT)/assets
	cp -r $(FRONTEND)/dist/. $(WEBROOT)/

build: web_build
	cd $(BACKEND) && CGO_ENABLED=0 go build -o ../../bin/oilfield ./cmd/oilfield
	@echo "built bin/oilfield"

# Native cross-compile (pure Go + modernc sqlite → static, byte-identical to a
# Linux build). Fast path used by `make deploy`.
build-linux: web_build
	cd $(BACKEND) && CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o ../../bin/oilfield-linux ./cmd/oilfield
	@echo "built bin/oilfield-linux (linux/amd64)"

# Colima/Docker parity build: compile + smoke-test the exact x86_64 artifact in a
# real Linux VM before shipping. Requires `colima start --arch x86_64`.
parity: web_build
	@docker info >/dev/null 2>&1 || { echo "Docker/Colima not running: 'colima start --arch x86_64 --cpu 2 --memory 4'"; exit 1; }
	docker run --rm -v "$(PWD)":/src -w /src/$(BACKEND) golang:1.26 \
		bash -lc 'CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /src/bin/oilfield-linux ./cmd/oilfield && echo PARITY_BUILD_OK'
	@echo "parity artifact at bin/oilfield-linux"

test:
	cd $(BACKEND) && go test ./...
	cd $(FRONTEND) && npm run test --silent || true

# ---- deploy to existing IONOS box (co-hosted; never touches crm) ----
deploy: build-linux
	@echo "=== stopping oilfield (ignore if first deploy) ==="
	-ssh $(DEPLOY_HOST) "systemctl stop oilfield"
	@echo "=== uploading binary → $(DEPLOY_HOST):$(DEPLOY_DIR) ==="
	ssh $(DEPLOY_HOST) "mkdir -p $(DEPLOY_DIR)/bin"
	ssh $(DEPLOY_HOST) "cat > $(DEPLOY_DIR)/bin/oilfield" < bin/oilfield-linux
	ssh $(DEPLOY_HOST) "chmod +x $(DEPLOY_DIR)/bin/oilfield && chown oilfield:oilfield $(DEPLOY_DIR)/bin/oilfield"
	@echo "=== restarting ==="
	ssh $(DEPLOY_HOST) "systemctl start oilfield && sleep 2 && systemctl is-active oilfield"
	$(MAKE) verify

verify:
	@bash deploy/scripts/verify-deploy.sh $(DEPLOY_HOST)

clean:
	rm -rf bin ./.data $(WEBROOT)/assets
