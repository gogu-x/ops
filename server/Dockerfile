# Build context is this server/ directory. The tree module is pulled from the
# Go module proxy (github.com/gogu-x/tree), so no local copy is needed.
FROM golang:1.26-alpine AS builder
WORKDIR /src
# Download dependencies first so this layer is cached until go.mod/go.sum change.
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/ops .

FROM alpine:3.22
RUN apk add --no-cache ca-certificates tzdata \
    && addgroup -S ops \
    && adduser -S -G ops ops
COPY --from=builder /out/ops /usr/local/bin/ops
USER ops
EXPOSE 8090
ENTRYPOINT ["/usr/local/bin/ops"]
