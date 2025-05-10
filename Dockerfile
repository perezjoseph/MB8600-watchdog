# Build stage
FROM --platform=linux/amd64 python:3.9-alpine AS builder

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

# Use a non-root user for better security
RUN adduser -D appuser
USER appuser

# Default command runs the monitor with 60 second intervals, 5 failures threshold (5 minutes)
ENTRYPOINT ["python", "monitor_internet.py"]

# Default arguments can be overridden at runtime
CMD ["--check-interval", "60", "--failure-threshold", "5", "--noverify"]