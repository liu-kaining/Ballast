# syntax=docker/dockerfile:1
FROM golang:1.22-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /ballast-server ./cmd/ballast-server

FROM alpine:3.20
RUN apk add --no-cache ca-certificates
COPY --from=build /ballast-server /usr/local/bin/ballast-server
WORKDIR /usr/local/bin
EXPOSE 8080
ENTRYPOINT ["ballast-server"]
