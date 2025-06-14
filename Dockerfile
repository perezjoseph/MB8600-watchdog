# Build stage
FROM --platform=linux/amd64 python:3.9-alpine AS builder

# Set labels for GitHub Container Registry
LABEL org.opencontainers.image.source=https://github.com/perezjoseph/MB8600-watchdog
LABEL org.opencontainers.image.description="Internet monitor and Motorola modem auto-reboot container"
LABEL org.opencontainers.image.licenses=MIT

WORKDIR /build

# Install build dependencies
RUN apk --no-cache add gcc musl-dev python3-dev

# Install only the specific requests package and dependencies
RUN pip install --no-cache-dir --target=/build requests

# Runtime stage
FROM --platform=linux/amd64 python:3.9-alpine

WORKDIR /app

# Install only the absolutely necessary system packages
RUN apk --no-cache add iputils-ping ca-certificates && \
    rm -rf /var/cache/apk/*

# Copy Python packages from builder
COPY --from=builder /build /usr/local/lib/python3.9/site-packages/

# Copy only the necessary scripts
COPY modem_reboot.py monitor_internet.py ./
RUN chmod +x modem_reboot.py monitor_internet.py

# Set default environment variables
ENV MODEM_HOST="192.168.100.1" \
    MODEM_USERNAME="admin" \
    MODEM_PASSWORD="motorola" \
    MODEM_NOVERIFY="true" \
    CHECK_INTERVAL="60" \
    FAILURE_THRESHOLD="5" \
    RECOVERY_WAIT="600"

# Use a non-root user for better security
RUN adduser -D appuser
USER appuser

# Use environment variables instead of command line arguments
ENTRYPOINT ["python", "monitor_internet.py"]
