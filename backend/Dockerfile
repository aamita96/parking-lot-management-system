# One image per service (NFR-17).
#   docker compose picks a stage by name: `target: api` or `target: notifier`.
#   Render always builds the LAST stage, so `api` must stay last.
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

FROM base AS notifier
COPY --from=build /out/notifier /usr/local/bin/notifier
EXPOSE 8081
ENTRYPOINT ["/usr/local/bin/notifier"]

# The final image has both programs. It starts the API; a Render Background Worker can run the
# email sender from the same image by setting its Docker command to /usr/local/bin/notifier.
FROM base AS api
COPY --from=build /out/api /usr/local/bin/api
COPY --from=build /out/notifier /usr/local/bin/notifier
EXPOSE 8080
CMD ["/usr/local/bin/api"]
