# syntax=docker/dockerfile:1

FROM golang:1.26-alpine AS build
WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/jobtool ./cmd/jobtool

FROM alpine:3.20
RUN adduser -D -u 10001 jobtool && apk add --no-cache ca-certificates wget
WORKDIR /app
COPY --from=build /src/config ./config
COPY --from=build /out/jobtool /usr/local/bin/jobtool
USER jobtool
ENTRYPOINT ["/usr/local/bin/jobtool"]
