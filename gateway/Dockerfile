# The gateway: Caddy with its config and the homepage. Build it from the repo
# root, like the apps.

FROM caddy:2.10.2-alpine
COPY gateway/Caddyfile /etc/caddy/Caddyfile
COPY gateway/site/ /srv/
# A broken Caddyfile would take every app offline, so fail the build instead.
RUN caddy validate --config /etc/caddy/Caddyfile --adapter caddyfile
