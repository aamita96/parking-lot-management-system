# One image per service (NFR-17): `--target api` or `--target notifier`.
FROM golang:1.27-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/api ./cmd/api \
 && CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/notifier ./cmd/notifier

FROM alpine:3.22 AS base
RUN apk add --no-cache ca-certificates tzdata && adduser -D -u 10001 plms
USER plms

FROM base AS api
COPY --from=build /out/api /usr/local/bin/api
EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/api"]

FROM base AS notifier
COPY --from=build /out/notifier /usr/local/bin/notifier
EXPOSE 8081
ENTRYPOINT ["/usr/local/bin/notifier"]
