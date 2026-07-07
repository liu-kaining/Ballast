# syntax=docker/dockerfile:1
FROM golang:1.26.4-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download
COPY . .
RUN --mount=type=cache,target=/go/pkg/mod \
    CGO_ENABLED=0 go build -o /ballast-server ./cmd/ballast-server

FROM alpine:3.20
RUN apk add --no-cache ca-certificates docker-cli
COPY --from=build /ballast-server /usr/local/bin/ballast-server
WORKDIR /usr/local/bin
EXPOSE 8080
ENTRYPOINT ["ballast-server"]
