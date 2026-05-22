# Stage 1: Build
FROM golang:1.22-alpine AS builder

# Install build dependencies
RUN apk add --no-cache make curl git

WORKDIR /app

# Copy go mod and sum files
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy the rest of the source code
COPY . .

# Build the application using the Makefile
# The Makefile handles downloading Tailwind CLI, compiling CSS, downloading the Roboto font, and building the Go binary.
# We set CGO_ENABLED=0 to ensure a completely static binary.
RUN CGO_ENABLED=0 make build

# Stage 2: Runtime
FROM alpine:latest

# Install CA certificates for HTTPS requests (e.g., RSS fetching)
RUN apk --no-cache add ca-certificates tzdata

WORKDIR /app

# Copy the pre-built binary from the builder stage
COPY --from=builder /app/rss-print .

# Set default port and database path
ENV PORT=8080
ENV DB_PATH=/app/data/rss-print.db

# Create directory for the database volume
RUN mkdir -p /app/data

EXPOSE 8080

ENTRYPOINT ["/app/rss-print"]
