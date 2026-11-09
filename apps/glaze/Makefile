# Development. The Docker image (deploy.sh) does its own build.

.PHONY: dev test

# Installs the web app's dependencies on a fresh clone, and again whenever
# the lockfile changes.
web/node_modules: web/package-lock.json
	npm --prefix web ci
	touch web/node_modules

# The backend on :8081 and the Vite dev server on :5174, so this can run
# alongside the other apps. Ctrl-C stops both.
dev: web/node_modules
	@trap 'kill 0' INT TERM EXIT; \
	ADDR=:8081 DATA_DIR=./data go run ./cmd/glaze & \
	npm --prefix web run dev -- $(VITE_ARGS) & \
	wait

# Everything that should pass before a deploy.
test: web/node_modules
	gofmt -l cmd internal web/*.go | (! grep .)
	go vet ./...
	GOOS=linux GOARCH=arm64 go vet ./...
	go test ./...
	npm --prefix web run type-check
	npm --prefix web run lint
	npm --prefix web test
