FROM golang:1.27-alpine AS build
WORKDIR /src

# go.sum pode não existir enquanto a API usa só a biblioteca padrão.
COPY go.mod go.sum* ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download

COPY . .
# healthcheck é um binário à parte porque a imagem distroless não tem shell nem curl.
RUN --mount=type=cache,target=/go/pkg/mod --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 go build -trimpath -ldflags "-s -w" -o /out/api ./cmd/api && \
    CGO_ENABLED=0 go build -trimpath -ldflags "-s -w" -o /out/healthcheck ./cmd/healthcheck

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/api /api
COPY --from=build /out/healthcheck /healthcheck
USER nonroot:nonroot
EXPOSE 8080
ENTRYPOINT ["/api"]
