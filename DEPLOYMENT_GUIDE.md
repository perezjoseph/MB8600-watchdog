# MB8600-Watchdog Deployment Guide

## Overview

This guide covers deploying both the standard and enhanced versions of MB8600-watchdog using Docker and Docker Compose.

## Available Versions

### Standard Version
- **Image**: `ghcr.io/perezjoseph/mb8600-watchdog:latest`
- **Features**: Basic internet monitoring and modem rebooting
- **Use Case**: Simple, lightweight monitoring

### Enhanced Version  
- **Image**: `ghcr.io/perezjoseph/mb8600-watchdog:latest-enhanced`
- **Features**: 
  - TCP/IP model diagnostics
  - Internet outage duration tracking
  - Advanced logging with rotation
  - Structured JSON logs
  - Periodic availability reports
- **Use Case**: Comprehensive network monitoring and analysis

## Quick Start

### Option 1: Enhanced Version (Recommended)
```bash
# Clone the repository
git clone https://github.com/perezjoseph/MB8600-watchdog.git
cd MB8600-watchdog

# Run enhanced version (default)
docker-compose up -d

# Or explicitly specify enhanced profile
docker-compose --profile enhanced up -d
```

### Option 2: Standard Version
```bash
# Run standard version
docker-compose --profile standard up -d
```

### Option 3: Enhanced Version with Log Viewer
```bash
# Run enhanced version with web-based log viewer
docker-compose --profile enhanced --profile logs up -d

# Access log viewer at http://localhost:8080
```

## Configuration

### Environment Variables

#### Basic Settings (Both Versions)
```bash
# Modem connection
MODEM_HOST=192.168.100.1          # Your modem's IP address
MODEM_USERNAME=admin              # Admin username
MODEM_PASSWORD=motorola           # Admin password (change this!)
MODEM_NOVERIFY=true              # Disable SSL verification

# Monitoring behavior
CHECK_INTERVAL=60                 # Seconds between checks
FAILURE_THRESHOLD=5               # Failures before action
RECOVERY_WAIT=600                # Seconds to wait after reboot
```

#### Enhanced Version Additional Settings
```bash
# Logging
LOG_LEVEL=INFO                   # DEBUG, INFO, WARNING, ERROR, CRITICAL
LOG_FILE=/app/logs/watchdog.log  # Log file path

# TCP/IP Diagnostics
ENABLE_DIAGNOSTICS=true          # Enable intelligent diagnostics
DIAGNOSTICS_TIMEOUT=120          # Diagnostics timeout in seconds

# Outage Tracking
OUTAGE_REPORT_INTERVAL=3600      # Availability report interval (seconds)
```

### Docker Compose Profiles

#### Available Profiles
- **`default`**: Enhanced version (default when no profile specified)
- **`enhanced`**: Enhanced version with all features
- **`standard`**: Standard version (basic functionality)
- **`logs`**: Adds log viewer service

#### Profile Usage Examples
```bash
# Enhanced version (default)
docker-compose up -d

# Standard version only
docker-compose --profile standard up -d

# Enhanced version with log viewer
docker-compose --profile enhanced --profile logs up -d

# All services
docker-compose --profile standard --profile enhanced --profile logs up -d
```

## Deployment Options

### 1. Local Development/Testing

```bash
# Clone and test
git clone https://github.com/perezjoseph/MB8600-watchdog.git
cd MB8600-watchdog

# Test enhanced version
docker-compose up

# Test standard version
docker-compose --profile standard up
```

### 2. Production Deployment

#### Using Pre-built Images
```yaml
version: '3.8'

services:
  internet-monitor:
    image: ghcr.io/perezjoseph/mb8600-watchdog:latest-enhanced
    container_name: mb8600-watchdog
    network_mode: "host"
    restart: unless-stopped
    environment:
      - MODEM_HOST=192.168.100.1
      - MODEM_USERNAME=admin
      - MODEM_PASSWORD=your_actual_password
      - LOG_LEVEL=INFO
      - ENABLE_DIAGNOSTICS=true
    volumes:
      - ./logs:/app/logs
```

#### Building from Source
```bash
# Build enhanced version
docker build -f Dockerfile-improved -t mb8600-watchdog:enhanced .

# Build standard version
docker build -f Dockerfile -t mb8600-watchdog:standard .
```

### 3. Docker Swarm Deployment

```yaml
version: '3.8'

services:
  internet-monitor:
    image: ghcr.io/perezjoseph/mb8600-watchdog:latest-enhanced
    networks:
      - host
    environment:
      - MODEM_HOST=192.168.100.1
      - MODEM_USERNAME=admin
      - MODEM_PASSWORD=your_password
    volumes:
      - logs:/app/logs
    deploy:
      replicas: 1
      restart_policy:
        condition: unless-stopped
      placement:
        constraints:
          - node.role == manager

volumes:
  logs:
    driver: local

networks:
  host:
    external: true
```

### 4. Kubernetes Deployment

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: mb8600-watchdog
spec:
  replicas: 1
  selector:
    matchLabels:
      app: mb8600-watchdog
  template:
    metadata:
      labels:
        app: mb8600-watchdog
    spec:
      hostNetwork: true  # Required for modem access
      containers:
      - name: watchdog
        image: ghcr.io/perezjoseph/mb8600-watchdog:latest-enhanced
        env:
        - name: MODEM_HOST
          value: "192.168.100.1"
        - name: MODEM_USERNAME
          value: "admin"
        - name: MODEM_PASSWORD
          valueFrom:
            secretKeyRef:
              name: modem-credentials
              key: password
        - name: LOG_LEVEL
          value: "INFO"
        - name: ENABLE_DIAGNOSTICS
          value: "true"
        volumeMounts:
        - name: logs
          mountPath: /app/logs
      volumes:
      - name: logs
        persistentVolumeClaim:
          claimName: watchdog-logs
---
apiVersion: v1
kind: Secret
metadata:
  name: modem-credentials
type: Opaque
stringData:
  password: "your_actual_password"
---
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: watchdog-logs
spec:
  accessModes:
    - ReadWriteOnce
  resources:
    requests:
      storage: 1Gi
```

## Monitoring and Maintenance

### Log Management

#### Enhanced Version Logs
```bash
# View real-time logs
docker-compose logs -f internet-monitor-enhanced

# View structured JSON logs
tail -f logs/watchdog.json | jq .

# Find outage events
cat logs/watchdog.json | jq 'select(.extra.outage_started or .extra.outage_resolved)'

# Calculate availability
cat logs/watchdog.json | jq 'select(.extra.availability_percentage)' | tail -1
```

#### Standard Version Logs
```bash
# View real-time logs
docker-compose logs -f internet-monitor

# Basic log viewing
docker-compose logs --tail=100 internet-monitor
```

### Health Checks

#### Enhanced Version
```bash
# Check container health
docker inspect mb8600-watchdog-enhanced --format='{{.State.Health.Status}}'

# Manual health check
docker exec mb8600-watchdog-enhanced python3 -c "import os; print('Healthy' if os.path.exists('/app/logs/watchdog.log') else 'Unhealthy')"
```

#### Standard Version
```bash
# Check if container is running
docker ps | grep mb8600-watchdog-standard

# Check logs for errors
docker-compose logs internet-monitor | grep -i error
```

### Performance Monitoring

#### Resource Usage
```bash
# Monitor resource usage
docker stats mb8600-watchdog-enhanced

# Check disk usage (enhanced version)
du -sh logs/
```

#### Network Diagnostics (Enhanced Version Only)
```bash
# Test diagnostics manually
docker exec mb8600-watchdog-enhanced python3 test_tcp_ip_diagnostics.py

# Test outage tracking
docker exec mb8600-watchdog-enhanced python3 test_outage_tracking.py
```

## Troubleshooting

### Common Issues

#### 1. Container Won't Start
```bash
# Check logs
docker-compose logs internet-monitor-enhanced

# Check configuration
docker-compose config

# Verify image
docker images | grep mb8600-watchdog
```

#### 2. Can't Access Modem
```bash
# Test network connectivity
docker exec mb8600-watchdog-enhanced ping -c 3 192.168.100.1

# Check if using host networking
docker inspect mb8600-watchdog-enhanced | grep NetworkMode
```

#### 3. Logs Not Persisting (Enhanced Version)
```bash
# Check volume mount
docker inspect mb8600-watchdog-enhanced | grep -A 10 Mounts

# Verify permissions
ls -la logs/

# Fix permissions if needed
sudo chown -R 1000:1000 logs/
```

#### 4. Diagnostics Timing Out
```bash
# Increase timeout
docker-compose down
# Edit docker-compose.yml: DIAGNOSTICS_TIMEOUT=180
docker-compose up -d

# Or disable diagnostics temporarily
# Edit docker-compose.yml: ENABLE_DIAGNOSTICS=false
```

### Debug Mode

#### Enhanced Version
```bash
# Run with debug logging
docker-compose down
# Edit docker-compose.yml: LOG_LEVEL=DEBUG
docker-compose up -d

# View debug logs
docker-compose logs -f internet-monitor-enhanced
```

#### Standard Version
```bash
# Run interactively for debugging
docker run -it --rm --network host \
  -e MODEM_HOST=192.168.100.1 \
  -e MODEM_USERNAME=admin \
  -e MODEM_PASSWORD=motorola \
  ghcr.io/perezjoseph/mb8600-watchdog:latest \
  python3 monitor_internet.py
```

## Security Considerations

### 1. Change Default Password
```yaml
environment:
  - MODEM_PASSWORD=your_secure_password  # Never use default!
```

### 2. Use Docker Secrets (Swarm)
```yaml
secrets:
  modem_password:
    external: true

services:
  internet-monitor:
    secrets:
      - modem_password
    environment:
      - MODEM_PASSWORD_FILE=/run/secrets/modem_password
```

### 3. Network Security
```yaml
# Use custom network instead of host mode if possible
networks:
  modem_network:
    driver: bridge
    ipam:
      config:
        - subnet: 192.168.100.0/24
```

### 4. File Permissions
```bash
# Secure log directory
chmod 750 logs/
chown 1000:1000 logs/

# Secure config files
chmod 600 docker-compose.yml
```

## Backup and Recovery

### Configuration Backup
```bash
# Backup configuration
tar -czf mb8600-backup-$(date +%Y%m%d).tar.gz \
  docker-compose.yml \
  logs/ \
  config/

# Restore configuration
tar -xzf mb8600-backup-20250803.tar.gz
```

### Log Backup (Enhanced Version)
```bash
# Rotate and backup logs
docker exec mb8600-watchdog-enhanced \
  python3 -c "import logging.handlers; h=logging.handlers.RotatingFileHandler('/app/logs/watchdog.log', maxBytes=1, backupCount=10); h.doRollover()"

# Archive old logs
tar -czf logs-archive-$(date +%Y%m%d).tar.gz logs/*.log.*
```

## Upgrading

### From Standard to Enhanced
```bash
# Stop standard version
docker-compose --profile standard down

# Start enhanced version
docker-compose --profile enhanced up -d

# Verify upgrade
docker-compose logs -f internet-monitor-enhanced
```

### Updating Images
```bash
# Pull latest images
docker-compose pull

# Restart with new images
docker-compose down
docker-compose up -d

# Clean up old images
docker image prune
```

This deployment guide provides comprehensive instructions for running MB8600-watchdog in various environments with both standard and enhanced configurations.
