# TailChat app image: Svelte PWA + Go server in one static, non-root container.

FROM node:24-alpine AS web
WORKDIR /web
COPY web/package*.json ./
RUN npm install --no-audit --no-fund
COPY web/ ./
RUN npm run build

FROM golang:1.23-alpine AS server
WORKDIR /src
COPY server/go.mod server/go.sum ./
RUN go mod download
COPY server/ ./
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /tailchat . \
 && mkdir -p /out/data/blobs

FROM gcr.io/distroless/static-debian12:nonroot
WORKDIR /app
COPY --from=server /tailchat /app/tailchat
COPY server/db/schema.sql /app/db/schema.sql
COPY --from=web /web/dist /app/web/dist
# Pre-create the data dir so a fresh named volume is owned by nonroot.
COPY --from=server --chown=65532:65532 /out/data /data
ENV DB_PATH=/data/tailchat.db \
    BLOB_DIR=/data/blobs
USER nonroot:nonroot
HEALTHCHECK --interval=30s --timeout=5s --retries=3 CMD ["/app/tailchat", "--healthcheck"]
ENTRYPOINT ["/app/tailchat"]
