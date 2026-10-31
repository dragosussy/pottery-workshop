# Local development and builds. The Docker image (deploy.sh) builds the same
# way inside containers.

VERSION  ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT   ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo unknown)
BUILT_AT := $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS  := -s -w -X main.version=$(VERSION) -X main.commit=$(COMMIT) -X main.builtAt=$(BUILT_AT)

# make dev DEMO_SEED=calibrated (or learning, or false)
DEMO_SEED ?= true
STUDIO_TZ ?= Europe/Bucharest

.PHONY: dev test build build-pi web clean

# The backend with the simulated sensor and pump and demo data on ./data/dev.db, and
# the Vite dev server (it proxies /api to :8080). Ctrl-C stops both.
# Delete ./data to reseed.
dev:
	@mkdir -p data
	@trap 'kill 0' INT TERM EXIT; \
	SENSOR_DRIVER=sim PUMP_DRIVER=sim DEMO_SEED=$(DEMO_SEED) STUDIO_TZ=$(STUDIO_TZ) \
	DB_PATH=./data/dev.db PHOTO_DIR=./data/photos BACKUP_DIR=./data/backups \
	go run ./cmd/humidity & \
	npm --prefix web run dev & \
	wait

test:
	gofmt -l cmd internal web/*.go | (! grep .)
	go vet ./...
	GOOS=linux GOARCH=arm64 go vet ./...
	go test ./...

web/node_modules: web/package-lock.json
	npm --prefix web ci
	@touch web/node_modules

web: web/node_modules
	npm --prefix web run build

# A binary for this machine with the web app embedded.
build: web
	go build -tags embedweb -trimpath -ldflags "$(LDFLAGS)" -o bin/humidity ./cmd/humidity

# A binary for the Raspberry Pi 5 (linux/arm64), web app embedded.
build-pi: web
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -tags embedweb -trimpath -ldflags "$(LDFLAGS)" -o bin/humidity-linux-arm64 ./cmd/humidity

clean:
	rm -rf bin
