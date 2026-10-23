# Caddy with the Cloudflare DNS plugin, so it can get a Let's Encrypt cert via
# DNS-01 for a hostname that only resolves to a private WireGuard IP.
FROM caddy:2-builder AS build
RUN xcaddy build --with github.com/caddy-dns/cloudflare

FROM caddy:2-alpine
COPY --from=build /usr/bin/caddy /usr/bin/caddy
