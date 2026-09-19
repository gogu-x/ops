# Build context must be the gotree parent directory (see compose.yaml).
FROM golang:1.26-alpine AS builder
WORKDIR /src
COPY tree/ ./tree/
COPY ops/server/ ./ops/server/
WORKDIR /src/ops/server
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/ops .

FROM alpine:3.22
RUN apk add --no-cache ca-certificates tzdata \
    && addgroup -S ops \
    && adduser -S -G ops ops
COPY --from=builder /out/ops /usr/local/bin/ops
USER ops
EXPOSE 8090
ENTRYPOINT ["/usr/local/bin/ops"]
