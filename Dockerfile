FROM --platform=linux/amd64 python:3.9-slim

WORKDIR /app

# Copy the scripts
COPY modem_reboot.py monitor_internet.py ./

# Install required Python packages
RUN pip install --no-cache-dir requests && \
    chmod +x modem_reboot.py monitor_internet.py

# Default command runs the monitor with 60 second intervals, 5 failures threshold (5 minutes)
ENTRYPOINT ["python", "monitor_internet.py"]

# Default arguments can be overridden at runtime
CMD ["--check-interval", "60", "--failure-threshold", "5", "--noverify"]