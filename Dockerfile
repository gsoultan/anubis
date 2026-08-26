# Anubis daemon. Multi-stage: a console build, a Go build with the
# toolchain, and a runtime image with nothing but the binary — no shell, no
# package manager, nothing for an attacker who reaches RCE to pivot with.
FROM oven/bun:1.4.0 AS console
WORKDIR /src/ui
COPY ui/package.json ui/bun.lock ./
RUN bun install --frozen-lockfile
COPY ui/ .
RUN bun run build

FROM golang:1.26-alpine AS build
ARG VERSION=dev
WORKDIR /src

# Dependencies first: this layer only changes when go.mod/go.sum do.
COPY go.mod go.sum ./
COPY pkg/anubis/go.mod pkg/anubis/
RUN go mod download

COPY . .
# The real console replaces the committed placeholder before the binary
# embeds ui/dist — the image always carries the console.
COPY --from=console /src/ui/dist ui/dist
# CGO off so the binary is static and the runtime image can be scratch-like.
# Migrations are embedded (migrations/embed.go), so the image carries its own
# schema and `anubisd migrate` needs no files mounted.
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath \
        -ldflags="-s -w -X main.version=${VERSION}" \
        -o /out/anubisd ./cmd/anubisd

FROM gcr.io/distroless/static-debian12:nonroot
WORKDIR /
COPY --from=build /out/anubisd /anubisd

# 7448 is the API. The debug listener (pprof/expvar) binds loopback only and
# is never published.
EXPOSE 7448
USER nonroot:nonroot

# No shell in the image, so the health check belongs to the orchestrator:
#   readinessProbe: GET /readyz   (fails while the snapshot is stale)
#   livenessProbe:  GET /healthz  (process alive)
ENTRYPOINT ["/anubisd"]
CMD ["serve"]
