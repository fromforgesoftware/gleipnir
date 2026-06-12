# Gleipnir builds two binaries (server + migrator) into one distroless image.
# Standalone module: build context is this repo root. GOWORK=off so the module
# resolves go-kit from its published version (github.com/fromforgesoftware/go-kit).
ARG GO_VERSION=1.25
FROM --platform=$BUILDPLATFORM golang:${GO_VERSION}-alpine AS builder
ARG TARGETOS TARGETARCH
WORKDIR /src
ENV GOWORK=off

# Resolve dependencies first for a cached layer.
COPY go.mod go.sum ./
RUN go mod download

# Module source (cmd, internal, pkg, api).
COPY . .

RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -trimpath -o /out/server   ./cmd/server
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -trimpath -o /out/migrator ./cmd/migrator

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=builder /out/server   /app/server
COPY --from=builder /out/migrator /app/migrator
# 8080 = REST/OpenAPI, 9090 = gRPC
EXPOSE 8080 9090
USER nonroot:nonroot
ENTRYPOINT ["/app/server"]
