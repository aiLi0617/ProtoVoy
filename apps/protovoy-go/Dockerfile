FROM golang:1.26-bookworm AS build
ARG PROTOVOY_VERSION=0.1.0-dev
WORKDIR /src
ENV CGO_ENABLED=0 GOFLAGS=-mod=readonly

COPY go.mod go.sum ./
RUN go mod download
COPY cmd ./cmd
COPY internal ./internal
COPY viva.toml ./
RUN go tool viva build --target web -o /out/web \
    && go build -trimpath -ldflags "-X protovoy.dev/protovoy/internal/buildinfo.Version=$PROTOVOY_VERSION" -o /out/protovoy-web ./cmd/server

FROM debian:bookworm-slim
WORKDIR /app
COPY --from=build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY --from=build /out/protovoy-web /app/protovoy-web
COPY --from=build /out/web /app/web
RUN install -d -m 0700 -o 65532 -g 65532 /var/lib/protovoy
USER 65532:65532
ENTRYPOINT ["/app/protovoy-web", "-web", "/app/web"]
