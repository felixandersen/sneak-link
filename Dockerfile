# Build stage
FROM golang:1.23-alpine AS builder

# Install build dependencies for CGO and SQLite
RUN apk add --no-cache gcc musl-dev sqlite-dev

WORKDIR /app

# Copy go mod files
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy source code
COPY . .

# Build the application with CGO enabled
ENV CGO_CFLAGS="-D_LARGEFILE64_SOURCE"
RUN CGO_ENABLED=1 GOOS=linux go build -a -tags "sqlite_omit_load_extension" -o sneak-link .

# Final stage
FROM alpine:latest

# Install ca-certificates for HTTPS requests
RUN apk --no-cache add ca-certificates

WORKDIR /root/

# Copy the binary from builder stage
COPY --from=builder /app/sneak-link .

# Expose port
EXPOSE 8080
EXPOSE 3000
EXPOSE 9090

# Run the application
CMD ["./sneak-link"]
