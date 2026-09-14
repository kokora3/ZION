# syntax=docker/dockerfile:1.8
ARG GO_VERSION=1.27.1
ARG GO_ALPINE_VERSION=3.23
ARG ALPINE_VERSION=3.22

FROM golang:${GO_VERSION}-alpine${GO_ALPINE_VERSION} AS builder
WORKDIR /src
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download
COPY cmd ./cmd
COPY internal ./internal
ARG VERSION=v0.1.0-alpha.1
ARG REVISION=unknown
ARG BUILD_DATE=unknown
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath \
      -ldflags="-s -w -X github.com/kokora3/zion/internal/buildinfo.Version=${VERSION} -X github.com/kokora3/zion/internal/buildinfo.Commit=${REVISION} -X github.com/kokora3/zion/internal/buildinfo.BuildDate=${BUILD_DATE}" \
      -o /out/zion-node ./cmd/zion-node && \
    CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath \
      -ldflags="-s -w -X github.com/kokora3/zion/internal/buildinfo.Version=${VERSION} -X github.com/kokora3/zion/internal/buildinfo.Commit=${REVISION} -X github.com/kokora3/zion/internal/buildinfo.BuildDate=${BUILD_DATE}" \
      -o /out/zionctl ./cmd/zionctl

FROM alpine:${ALPINE_VERSION}
ARG VERSION=v0.1.0-alpha.1
ARG REVISION=unknown
ARG BUILD_DATE=unknown
LABEL org.opencontainers.image.title="ZION Node" \
      org.opencontainers.image.description="Go-native ZION node and zionctl" \
      org.opencontainers.image.version="${VERSION}" \
      org.opencontainers.image.revision="${REVISION}" \
      org.opencontainers.image.created="${BUILD_DATE}" \
      org.opencontainers.image.source="https://github.com/kokora3/zion" \
      org.opencontainers.image.licenses="Apache-2.0"
RUN addgroup -S -g 10001 zion && \
    adduser -S -D -H -u 10001 -G zion zion && \
    mkdir -p /etc/zion /var/lib/zion /opt/zion /tmp && \
    chown -R 10001:10001 /var/lib/zion /opt/zion /tmp
COPY --from=builder /out/zion-node /usr/local/bin/zion-node
COPY --from=builder /out/zionctl /usr/local/bin/zionctl
COPY deploy/docker/zion-node-entrypoint.sh /usr/local/bin/zion-node-entrypoint
RUN chmod 0555 /usr/local/bin/zion-node /usr/local/bin/zionctl /usr/local/bin/zion-node-entrypoint
ENV ZION_API_TOKEN_FILE=/var/lib/zion/runtime/api-token
USER 10001:10001
WORKDIR /opt/zion
VOLUME ["/var/lib/zion"]
EXPOSE 42000/udp 42001/tcp 26656/tcp
STOPSIGNAL SIGTERM
HEALTHCHECK --interval=10s --timeout=5s --start-period=10s --retries=12 \
  CMD zionctl --bearer-token-file "$ZION_API_TOKEN_FILE" status || exit 1
ENTRYPOINT ["/usr/local/bin/zion-node-entrypoint"]
CMD ["zion-node", "run", "--config", "/etc/zion/zion.yaml"]
