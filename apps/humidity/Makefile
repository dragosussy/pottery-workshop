# Development. The Docker image (deploy.sh) does its own build.

.PHONY: dev test

# Installs the web app's dependencies on a fresh clone, and again whenever
# the lockfile changes.
web/node_modules: web/package-lock.json
	npm --prefix web ci
	touch web/node_modules

# The backend with the simulated sensor, pump and screen (the screen is
# drawn to ./data/screen.png), and the Vite dev server (it proxies /api to
# :8080). Ctrl-C stops both.
dev: web/node_modules
	@trap 'kill 0' INT TERM EXIT; \
	SENSOR=sim PUMP=sim SCREEN=sim DATA_DIR=./data go run ./cmd/humidity & \
	npm --prefix web run dev -- $(VITE_ARGS) & \
	wait

# Everything that should pass before a deploy: Go formatting, vet (for this
# machine and the Pi) and tests, then the web app's types, lint and tests.
test: web/node_modules
	gofmt -l cmd internal web/*.go | (! grep .)
	go vet ./...
	GOOS=linux GOARCH=arm64 go vet ./...
	go test ./...
	npm --prefix web run type-check
	npm --prefix web run lint
	npm --prefix web test
