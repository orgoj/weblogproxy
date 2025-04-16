# Build stage
FROM golang:1.23-alpine AS builder

# Install dependencies
RUN apk add --no-cache git

# Set working directory
WORKDIR /app

# Copy go.mod and go.sum files first to leverage Docker cache
COPY go.mod go.sum ./
RUN go mod download

# Copy the source code
COPY . .

# Build the application with version information
ARG VERSION
ARG BUILD_DATE
ARG COMMIT_HASH

RUN CGO_ENABLED=0 GOOS=linux go build \
    -ldflags "-X github.com/orgoj/weblogproxy/internal/version.Version=${VERSION} \
              -X github.com/orgoj/weblogproxy/internal/version.BuildDate=${BUILD_DATE} \
              -X github.com/orgoj/weblogproxy/internal/version.CommitHash=${COMMIT_HASH}" \
    -o bin/weblogproxy ./cmd/weblogproxy

RUN CGO_ENABLED=0 GOOS=linux go build -o bin/config-validator ./cmd/config-validator

# Final stage
FROM alpine:3.21

# Install necessary dependencies
RUN apk add --no-cache ca-certificates tzdata bash

# Set working directory
WORKDIR /app

# Create necessary directories for log
RUN mkdir -p /app/log

# Copy the binaries from the builder stage
COPY --from=builder /app/bin/weblogproxy /app/weblogproxy
COPY --from=builder /app/bin/config-validator /app/config-validator

# Copy config examples 
COPY --from=builder /app/config/example.yaml /app/config/example.yaml
COPY --from=builder /app/config/docker-config.yaml /app/config/config.yaml

# Set environment variables
ENV CONFIG_FILE=/app/config/config.yaml

# Expose the default port
EXPOSE 8080

# Standalone mode health check (doesn't use path prefix)
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
  CMD wget -qO- http://localhost:8080/health || exit 1

# Run the application with the config file
CMD ["/app/weblogproxy", "-config", "/app/config/config.yaml"]
