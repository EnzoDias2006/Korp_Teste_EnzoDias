# Build the billing service using a multi-stage Docker build.
# This produces a minimal, non-root production image.

# Build stage
FROM golang:1.26-alpine AS builder

WORKDIR /app

# Copy go module files
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build the application
RUN CGO_ENABLED=0 GOOS=linux go build -o /billing-service ./cmd/api

# Production stage
FROM alpine:3.20

WORKDIR /app

RUN apk add --no-cache ca-certificates

# Create a non-root user and group
RUN addgroup -S billing && adduser -S billing -G billing

# Copy the binary from the builder stage
COPY --from=builder --chown=billing:billing /billing-service /app/billing-service

# Switch to the non-root user
USER billing

# Expose the port the service runs on
EXPOSE 8082

# Health check
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
	CMD wget --no-verbose --tries=1 --spider http://localhost:8082/health/live || exit 1

# Run the service
CMD ["/app/billing-service"]
