# Internet Monitoring and Modem Reboot Container

This Docker container monitors your internet connection and automatically reboots your Motorola/Arris modem if the connection is down for a specified period.

## How it Works

1. The container checks internet connectivity every minute by:
   - Pinging reliable hosts (1.1.1.1, 8.8.8.8, 9.9.9.9)
   - Making HTTP requests to reliable websites (Google, Cloudflare, Amazon)

2. If all connectivity checks fail for 5 consecutive checks (5 minutes total), it:
   - Logs into your modem using the HNAP API
   - Sends a reboot command
   - Waits for the modem to complete the reboot cycle
   - Waits an additional recovery period (10 minutes by default) before resuming monitoring

## Setup on NAS

### Transferring Files

1. Transfer these files to your NAS:
   - modem_reboot.py
   - monitor_internet.py
   - Dockerfile.monitor.amd64 (for x86 systems like your NAS)
   - docker-compose.monitor.yml

2. SSH into your NAS: 
   ```
   ssh perezjoseph@192.168.88.22
   ```

3. Navigate to the directory containing the transferred files:
   ```
   cd /path/to/files
   ```

### Building and Running

1. Build the container on your NAS:
   ```
   docker build -t modem-monitor:amd64 -f Dockerfile.monitor.amd64 .
   ```

2. Run the container with docker-compose:
   ```
   docker-compose -f docker-compose.monitor.yml up -d
   ```

3. Verify it's running:
   ```
   docker ps
   docker logs -f internet-monitor
   ```

## Configuration Options

You can adjust these parameters in docker-compose.monitor.yml:

- `--host`: Modem IP address (default: 192.168.100.1)
- `--username`: Admin username (default: admin)
- `--password`: Admin password
- `--check-interval`: Seconds between checks (default: 60)
- `--failure-threshold`: Number of failed checks needed to trigger reboot (default: 5)
- `--recovery-wait`: Seconds to wait after reboot before resuming monitoring (default: 600)
- `--noverify`: Disable SSL certificate verification

## Setting up Auto-start

The container is configured with `restart: always` so it will automatically restart if it crashes or if your NAS reboots.

## Viewing Logs

To see the monitoring logs:
```
docker logs -f internet-monitor
```