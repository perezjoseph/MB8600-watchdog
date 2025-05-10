# Internet Monitoring and Modem Reboot Container

This Docker container monitors your internet connection and automatically reboots your Motorola/Arris modem if the connection is down for a specified period.

## How it Works

1. The container checks internet connectivity at configurable intervals (default: every 60 seconds) by:
   - Pinging reliable hosts (1.1.1.1, 8.8.8.8, 9.9.9.9)
   - Making HTTP requests to reliable websites (Google, Cloudflare, Amazon)

2. If all connectivity checks fail for a configurable number of consecutive checks (default: 5 checks), it:
   - Logs into your modem using the HNAP API
   - Sends a reboot command
   - Waits for the modem to complete the reboot cycle
   - Waits an additional recovery period (10 minutes by default) before resuming monitoring

## Setup

### Transferring Files

1. Transfer these files to your system:
   - modem_reboot.py
   - monitor_internet.py
   - Dockerfile
   - docker-compose.yml

2. SSH into your system: 
   ```
   ssh username@your_system_ip
   ```

3. Navigate to the directory containing the transferred files:
   ```
   cd /path/to/files
   ```

### Building and Running

1. Build the container:
   ```
   docker build -t modem-monitor .
   ```

2. Run the container with docker-compose:
   ```
   docker-compose up -d
   ```

3. Verify it's running:
   ```
   docker ps
   docker logs -f internet-monitor
   ```

## Configuration Options

You can adjust these parameters in docker-compose.yml as environment variables:

- `MODEM_HOST`: Modem IP address (default: 192.168.100.1)
- `MODEM_USERNAME`: Admin username (default: admin)
- `MODEM_PASSWORD`: Admin password (default: motorola)
- `CHECK_INTERVAL`: Seconds between checks (default: 60)
- `FAILURE_THRESHOLD`: Number of failed checks needed to trigger reboot (default: 5)
- `RECOVERY_WAIT`: Seconds to wait after reboot before resuming monitoring (default: 600)
- `MODEM_NOVERIFY`: Disable SSL certificate verification (set to "true" to enable)

Alternatively, you can override these settings using command line arguments when starting the container:

- `--host`: Modem IP address
- `--username` or `-u`: Admin username
- `--password`: Admin password
- `--check-interval`: Seconds between checks
- `--failure-threshold`: Number of failed checks needed to trigger reboot
- `--recovery-wait`: Seconds to wait after reboot
- `--noverify` or `-n`: Disable SSL certificate verification

## Setting up Auto-start

The container is configured with `restart: always` so it will automatically restart if it crashes or if your system reboots.

## Viewing Logs

To see the monitoring logs:
```
docker logs -f internet-monitor
```