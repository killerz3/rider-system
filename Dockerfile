# Debian supplies GCC for h3-go's vendored C; runtime uses matching glibc.
FROM golang:1.23.4-bookworm AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY *.go ./
RUN CGO_ENABLED=1 go build -trimpath -o /out/rider-system .
FROM debian:bookworm-slim
RUN useradd --system --uid 10001 app && mkdir /data && chown app /data
COPY --from=build /out/rider-system /usr/local/bin/rider-system
USER app
ENV HTTP_PORT=8080 SQLITE_PATH=/data/riders.db REDIS_ADDR=redis:6379
EXPOSE 8080
ENTRYPOINT ["rider-system"]
