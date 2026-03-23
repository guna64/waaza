# syntax=docker/dockerfile:1
FROM golang:1.25-alpine AS builder
WORKDIR /app
RUN apk add --no-cache git ca-certificates
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /waaza ./cmd/server

FROM alpine:3.20
RUN addgroup -S waaza && adduser -S waaza -G waaza \
    && apk add --no-cache ca-certificates tzdata
WORKDIR /app
COPY --from=builder /waaza /usr/local/bin/waaza
COPY openapi ./openapi
COPY web ./web
USER waaza
EXPOSE 8090
ENTRYPOINT ["/usr/local/bin/waaza"]
