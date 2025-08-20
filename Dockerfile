# Build stage
FROM golang:1.21-alpine AS builder

# Install build dependencies
RUN apk add --no-cache git

# Set working directory
WORKDIR /build

# Copy go mod files
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy source code
COPY . .

# Build the application
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o relay cmd/server/main.go

# Final stage
FROM alpine:latest

# Install ca-certificates for HTTPS
RUN apk --no-cache add ca-certificates tzdata

# Create non-root user
RUN addgroup -g 1000 -S relay && \
    adduser -u 1000 -S relay -G relay

# Set working directory
WORKDIR /app

# Copy binary from builder
COPY --from=builder /build/relay .

# Copy static files
COPY --from=builder /build/static ./static

# Create directories
RUN mkdir -p /app/data/queue && \
    chown -R relay:relay /app

# Switch to non-root user
USER relay

# Expose ports
EXPOSE 2525 8080 9090

# Health check
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD wget --no-verbose --tries=1 --spider http://localhost:8080/ || exit 1

# Run the application
CMD ["./relay"]