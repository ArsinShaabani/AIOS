# Multi-stage, multi-arch friendly build. CGO is disabled because the SQLite
# driver is pure Go, so the same Dockerfile builds for amd64, arm64 and armv7.
FROM golang:1.27-alpine AS build
WORKDIR /src
RUN apk add --no-cache git ca-certificates
COPY go.mod go.sum ./
RUN go mod download
COPY . .
ARG VERSION=1.0.0
ARG COMMIT=dev
ARG DATE=unknown
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w \
    -X github.com/ArsinShaabani/AIOS/internal/version.Version=${VERSION} \
    -X github.com/ArsinShaabani/AIOS/internal/version.Commit=${COMMIT} \
    -X github.com/ArsinShaabani/AIOS/internal/version.Date=${DATE}" \
    -o /out/aios ./cmd/aios

FROM gcr.io/distroless/static-debian12:nonroot
WORKDIR /app
COPY --from=build /out/aios /usr/local/bin/aios
COPY --from=build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY config.example.yaml /app/config.example.yaml
ENV AIOS_CONFIG=/app/config.yaml \
    AIOS_APP_DATA_DIR=/data \
    AIOS_DATABASE_DSN=/data/aios.db \
    AIOS_SERVER_ADDR=0.0.0.0:8080 \
    GOMEMLIMIT=384MiB \
    GOGC=60
VOLUME ["/data"]
EXPOSE 8080
USER nonroot:nonroot
HEALTHCHECK --interval=30s --timeout=5s --start-period=20s --retries=3 \
  CMD ["/usr/local/bin/aios", "doctor"]
ENTRYPOINT ["/usr/local/bin/aios"]
CMD ["server"]
