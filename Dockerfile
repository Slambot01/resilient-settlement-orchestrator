# Build stage
FROM golang:1.26.3-alpine AS builder

# Set working directory
WORKDIR /app

# Install git and certificates
RUN apk add --no-cache git ca-certificates tzdata

# Copy go mod and sum files
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy source code
COPY . .

# Build the application
# CGO_ENABLED=0 creates a statically linked binary
RUN CGO_ENABLED=0 GOOS=linux go build -o /app/server ./cmd/server

# Final stage
FROM alpine:3.19

# Create non-root user for security (V-006 fix)
RUN addgroup -S app && adduser -S app -G app

WORKDIR /app

# Copy certificates and timezone data from builder
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=builder /usr/share/zoneinfo /usr/share/zoneinfo

# Copy the binary from builder
COPY --from=builder /app/server .
# Copy migrations
COPY --from=builder /app/migrations ./migrations
# Copy dashboard static files
COPY --from=builder /app/dashboard ./dashboard

# Ensure app user owns the working directory
RUN chown -R app:app /app

# Set environment variables for production (can be overridden)
ENV SERVER_ENV=production
ENV SERVER_PORT=8080

# Expose ports: 8080 (API), 9091 (internal metrics for Prometheus)
EXPOSE 8080 9091

# Run as non-root user
USER app

# Run the binary
CMD ["./server"]
