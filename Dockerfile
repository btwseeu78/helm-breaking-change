# Multi-stage build for smaller final image

# Build stage
FROM golang:1.25.1 AS builder

WORKDIR /app

# Copy Go modules and download dependencies
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build the application
ARG TARGETARCH
RUN CGO_ENABLED=0 GOOS=linux GOARCH=${TARGETARCH:-amd64} go build -o check-breaking-change .

# Final stage with Google Cloud CLI
FROM gcr.io/google.com/cloudsdktool/google-cloud-cli:latest

# Install required dependencies
RUN apt-get update && apt-get install -y \
    curl \
    git \
    ca-certificates \
    && rm -rf /var/lib/apt/lists/*

# Install Helm
ARG TARGETARCH
RUN ARCH=${TARGETARCH:-amd64} && \
    curl https://get.helm.sh/helm-v3.20.1-linux-${ARCH}.tar.gz | tar -xz && \
    mv linux-${ARCH}/helm /usr/local/bin/helm && \
    rm -rf linux-${ARCH}

# Copy the built binary from builder stage
COPY --from=builder /app/check-breaking-change /usr/local/bin/check-breaking-change

# Make binary executable
RUN chmod +x /usr/local/bin/check-breaking-change

# Set working directory for usage
WORKDIR /workspace

# Default command
CMD ["check-breaking-change", "--help"]
