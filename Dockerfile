# enoch-edge — multi-stage Dockerfile
#
# Mirrors the operator's Dockerfile (golang:1.22-alpine -> alpine:3.20).
# Stage `go-builder` compiles, stage `edge` is the runtime image.

# ---------------------------------------------------------------------------
# Go build
# ---------------------------------------------------------------------------
FROM golang:1.22-alpine AS go-builder
RUN apk add --no-cache git
WORKDIR /src

COPY . ./

RUN --mount=type=cache,target=/root/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    go mod tidy

RUN --mount=type=cache,target=/root/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 go build -trimpath -o /out/enoch-edge ./cmd/edge

# ---------------------------------------------------------------------------
# Runtime
# ---------------------------------------------------------------------------
FROM alpine:3.20 AS edge
RUN apk add --no-cache ca-certificates curl
COPY --from=go-builder /out/enoch-edge /usr/local/bin/enoch-edge
EXPOSE 8081
ENTRYPOINT ["/usr/local/bin/enoch-edge"]
