FROM golang:1.24-alpine AS build

WORKDIR /src
COPY go.mod ./
COPY cmd ./cmd
COPY internal ./internal

RUN CGO_ENABLED=0 GOOS=linux go build \
    -trimpath \
    -ldflags="-s -w" \
    -o /out/cupnotifier \
    ./cmd/cupnotifier

FROM alpine:3.22

RUN addgroup -S -g 65532 cupnotifier \
    && adduser -S -D -H -u 65532 -G cupnotifier cupnotifier \
    && mkdir -p /data \
    && chown cupnotifier:cupnotifier /data

COPY --from=build /out/cupnotifier /usr/local/bin/cupnotifier

USER cupnotifier:cupnotifier
VOLUME ["/data"]
ENTRYPOINT ["/usr/local/bin/cupnotifier"]
