FROM golang:1.26-alpine AS build
ARG VERSION="dev"

WORKDIR /build

RUN --mount=type=cache,target=/var/cache/apk \
    apk add git

COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download

COPY . .

RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 go build \
    -ldflags="-s -w -X github.com/Flgado/trino-insights-mcp/internal/timcp.Version=${VERSION}" \
    -o /bin/trino-insights-mcp ./cmd

FROM gcr.io/distroless/base-debian12

WORKDIR /server
COPY --from=build /bin/trino-insights-mcp .
ENTRYPOINT ["/server/trino-insights-mcp"]
CMD ["stdio"]
