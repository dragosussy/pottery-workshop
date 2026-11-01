# Development and builds. deploy.sh builds the Docker image the same way.

.PHONY: dev test web build build-pi clean

# The backend with the simulated sensor and pump, and the Vite dev server
# (it proxies /api to :8080). Ctrl-C stops both.
dev:
	@trap 'kill 0' INT TERM EXIT; \
	SENSOR=sim PUMP=sim DATA_DIR=./data go run ./cmd/humidity & \
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
	go build -tags embedweb -trimpath -o bin/humidity ./cmd/humidity

# The same for the Raspberry Pi 5.
build-pi: web
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -tags embedweb -trimpath -o bin/humidity-linux-arm64 ./cmd/humidity

clean:
	rm -rf bin
