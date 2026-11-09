# Commands for the whole repo. Each app has its own Makefile for dev and tests.

.PHONY: dev test

# The Caddy image the Pi runs, so development can't drift from it.
CADDY_IMAGE := $(shell awk '/^FROM/ {print $$2}' gateway/Dockerfile)

# The gateway and every app, with simulated hardware, on port 8000. Needs
# Docker running. The homepage is served from gateway/site, so a reload shows
# edits to it. Ctrl-C stops everything.
dev:
	@trap 'docker rm -f atelier-dev-gateway >/dev/null 2>&1; kill 0' INT TERM EXIT; \
	docker rm -f atelier-dev-gateway >/dev/null 2>&1; \
	(until curl -sf -o /dev/null http://localhost:8000/humidity/api/summary; do sleep 1; done; \
		echo; echo "Atelier is ready: http://localhost:8000"; \
		echo "From a phone:     http://<this-machine>.<tailnet>.ts.net:8000"; echo) & \
	docker run --rm --name atelier-dev-gateway -p 8000:80 \
		-v "$(CURDIR)/gateway/Caddyfile.dev:/etc/caddy/Caddyfile:ro" \
		-v "$(CURDIR)/gateway/site:/srv:ro" \
		"$(CADDY_IMAGE)" & \
	$(MAKE) -C apps/humidity dev VITE_ARGS="--logLevel warn" & \
	wait

# Everything that should pass before a deploy, for every app.
test:
	$(MAKE) -C apps/humidity test
	docker build -q -f backup/Dockerfile -t atelier-backup-test . >/dev/null
	docker run --rm --entrypoint sh -v "$(CURDIR)/backup/test.sh:/test.sh:ro" atelier-backup-test /test.sh
