# syntax=docker/dockerfile:1.7
FROM golang:1.27.2-bookworm AS build
WORKDIR /src
ENV CGO_ENABLED=0 GOOS=linux GOTOOLCHAIN=auto
COPY go.mod ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download
COPY cmd ./cmd
COPY internal ./internal
COPY migrations ./migrations
COPY web ./web
COPY tests ./tests
RUN mkdir -p /src/uploads /out && chown -R 65532:65532 /src/uploads
RUN --mount=type=cache,target=/go/pkg/mod --mount=type=cache,target=/root/.cache/go-build \
    go mod tidy && go test ./... && go vet ./... && \
    go build -trimpath -buildvcs=false -ldflags="-s -w" -o /out/portfolio ./cmd/server

FROM gcr.io/distroless/static-debian12:nonroot
WORKDIR /app
COPY --from=build --chown=65532:65532 /out/portfolio /app/portfolio
COPY --from=build --chown=65532:65532 /src/migrations /app/migrations
COPY --from=build --chown=65532:65532 /src/web /app/web
COPY --from=build --chown=65532:65532 /src/uploads /app/uploads
USER 65532:65532
ENV PORT=8080 TEMPLATE_DIR=/app/web/templates STATIC_DIR=/app/web/static MIGRATIONS_DIR=/app/migrations UPLOAD_DIR=/app/uploads
VOLUME ["/app/uploads"]
EXPOSE 8080
ENTRYPOINT ["/app/portfolio"]
CMD ["serve"]
