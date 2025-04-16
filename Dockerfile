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
RUN apk add --no-cache ca-certificates tzdata bash shadow su-exec

# Set working directory
WORKDIR /app

# Create app directories
RUN mkdir -p /app && \
    mkdir -p /etc/weblogproxy && \
    mkdir -p /var/log/weblogproxy && \
    mkdir -p /var/lib/weblogproxy

# Copy our binaries
COPY --from=builder /app/bin/* /usr/local/bin/

# Copy entrypoint script
COPY entrypoint.sh /app/entrypoint.sh
RUN chmod +x /app/entrypoint.sh

# Create the weblogproxy user/group
RUN addgroup -g 1000 -S weblogproxy && \
    adduser -u 1000 -S weblogproxy -G weblogproxy && \
    chown -R weblogproxy:weblogproxy /app /etc/weblogproxy /var/log/weblogproxy /var/lib/weblogproxy

# Create necessary directories for log and set proper permissions
RUN mkdir -p /app/log /app/config && \
    chown -R weblogproxy:weblogproxy /app

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

# Use the entrypoint script to handle UID/GID mapping
ENTRYPOINT ["/app/entrypoint.sh"]

# Default command
CMD ["/usr/local/bin/weblogproxy", "-config", "/app/config/config.yaml"]
