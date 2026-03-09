# Build stage
FROM golang:1.22-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o toutim-server ./cmd/server

# Runtime stage
FROM alpine:3.19
RUN apk add --no-cache ca-certificates tzdata
COPY --from=builder /app/toutim-server /usr/local/bin/
RUN mkdir -p /data
EXPOSE 8080
ENV TOUTIM_DATA_DIR=/data
ENTRYPOINT ["toutim-server"]
