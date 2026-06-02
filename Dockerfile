# Gleipnir builds two binaries (server + migrator) into one distroless image.
# Build context is the forge repo root. GOWORK=off so the module resolves
# go/kit from its published version (not the workspace).
ARG GO_VERSION=1.25
FROM golang:${GO_VERSION}-alpine AS builder
WORKDIR /src

# Only the module surface Gleipnir needs: the service itself.
COPY services/gleipnir/ /src/services/gleipnir/

WORKDIR /src/services/gleipnir
ENV GOWORK=off
RUN CGO_ENABLED=0 go build -trimpath -o /out/server   ./cmd/server
RUN CGO_ENABLED=0 go build -trimpath -o /out/migrator ./cmd/migrator

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=builder /out/server   /app/server
COPY --from=builder /out/migrator /app/migrator
# 8080 = REST/OpenAPI, 9090 = gRPC
EXPOSE 8080 9090
USER nonroot:nonroot
ENTRYPOINT ["/app/server"]
