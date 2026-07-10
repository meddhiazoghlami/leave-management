# syntax=docker/dockerfile:1

# ─────────────────────────────────────────────────────────────────────────────
# Stage 1 — build the front-end assets with Vite.
# Tailwind scans views/*.templ (via @source), and vite outputs to ../public/build,
# so both web/ and views/ must be present.
# ─────────────────────────────────────────────────────────────────────────────
FROM node:22-alpine AS assets
WORKDIR /app
COPY web/package.json web/package-lock.json ./web/
RUN cd web && npm ci
COPY web/ ./web/
COPY views/ ./views/
RUN cd web && npm run build
# -> /app/public/build (hashed JS/CSS + .vite/manifest.json)

# ─────────────────────────────────────────────────────────────────────────────
# Stage 2 — compile the Go binary. One static (CGO off) binary with `serve` and
# `seed` subcommands (Cobra). Generated code (templ, sqlc) is committed, so no
# codegen is needed here.
# ─────────────────────────────────────────────────────────────────────────────
FROM golang:1.25-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/leave-management .

# ─────────────────────────────────────────────────────────────────────────────
# Stage 3 — minimal runtime. Alpine (not distroless) so the image ships a
# busybox wget for the container HEALTHCHECK. Runs as a non-root user.
# ─────────────────────────────────────────────────────────────────────────────
FROM alpine:3.20 AS runtime
RUN apk add --no-cache ca-certificates tzdata \
 && adduser -D -u 10001 appuser
WORKDIR /app
COPY --from=build   /out/leave-management  /app/leave-management
COPY --from=assets  /app/public/build      /app/public/build
USER appuser
EXPOSE 8080
ENV ADDR=":8080"
HEALTHCHECK --interval=15s --timeout=3s --start-period=10s --retries=5 \
	CMD wget -qO- http://localhost:8080/healthz >/dev/null 2>&1 || exit 1
# Default to `serve`; the seed service overrides the command with `seed`.
ENTRYPOINT ["/app/leave-management"]
CMD ["serve"]
