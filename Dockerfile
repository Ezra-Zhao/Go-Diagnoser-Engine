# syntax=docker/dockerfile:1

# ---- build ----
FROM golang:1.24-alpine AS build
WORKDIR /src
COPY go.mod ./
# no external deps, but keep the layer for future ones
RUN go mod download 2>/dev/null || true
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /bin/server ./cmd/server

# ---- run ----
FROM alpine:3.21
RUN adduser -D -H appuser
USER appuser
COPY --from=build /bin/server /bin/server
EXPOSE 8080
ENV PORT=8080
ENTRYPOINT ["/bin/server"]
