ARG GO_VERSION=1.24.1
FROM --platform=$BUILDPLATFORM golang:${GO_VERSION}-alpine3.20 AS builder

RUN apk update && \
    apk add --no-cache git openssh-client upx

WORKDIR /build

COPY go.mod go.sum .
RUN go mod download

COPY . .

ARG VERSION="dev"
ARG BUILD_DATE
ARG COMMIT_SHA

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build \
    -trimpath \
    -ldflags="-w -s -X main.version=${VERSION} -X main.build=${BUILD_DATE} -X main.commit=${COMMIT_SHA}" \
    -tags=netgo,osusergo \
    -o /app/rayping \
    ./cmd/rayping/ \
    && upx --best --lzma /app/rayping

FROM alpine:3.20

RUN apk update && \
    apk add --no-cache git openssh-client ca-certificates tzdata && \
    adduser -D -u 1001 rayping && \
    mkdir -p /app/keys && \
    chown rayping:rayping /app/keys

RUN mkdir -p /home/rayping/.ssh && \
    ssh-keyscan -H github.com >> /home/rayping/.ssh/known_hosts && \
    chown -R rayping:rayping /home/rayping/.ssh && \
    chmod 700 /home/rayping/.ssh && \
    chmod 600 /home/rayping/.ssh/known_hosts

WORKDIR /app
COPY --from=builder --chown=rayping:rayping /app/rayping .

USER rayping
EXPOSE 8080

HEALTHCHECK --interval=30s --timeout=10s --start-period=5s --retries=3 \
    CMD wget --no-verbose --tries=1 --spider http://localhost:8080/health || exit 1

LABEL org.opencontainers.image.title="RayPing" \
      org.opencontainers.image.description="Proxy configuration checker and validator" \
      org.opencontainers.image.url="https://github.com/NaMiraNet/rayping" \
      org.opencontainers.image.vendor="NaMiraNet" \
      org.opencontainers.image.version="${VERSION}" \
      org.opencontainers.image.created="${BUILD_DATE}" \
      org.opencontainers.image.revision="${COMMIT_SHA}"

ENTRYPOINT ["./rayping"]
CMD ["api"]
