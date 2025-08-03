# MB8600-Watchdog: Internet Monitoring and Modem Reboot Container

This Docker container monitors your internet connection and automatically reboots your Motorola/Arris modem if the connection is down for a specified period.

## 🚀 Quick Start

### Option 1: Interactive Deployment (Recommended)
```bash
git clone https://github.com/perezjoseph/MB8600-watchdog.git
cd MB8600-watchdog
./deploy.sh
```

### Option 2: Enhanced Version (Default) - Remote Image
```bash
git clone https://github.com/perezjoseph/MB8600-watchdog.git
cd MB8600-watchdog
docker compose up -d
```

### Option 3: Using Pre-built Images Directly
```bash
# Enhanced version (recommended)
docker run -d --name mb8600-watchdog --network host \
  -e MODEM_HOST=192.168.100.1 \
  -e MODEM_PASSWORD=your_password \
  -v ./logs:/app/logs \
  ghcr.io/perezjoseph/mb8600-watchdog:latest-enhanced

# Standard version  
docker run -d --name mb8600-watchdog --network host \
  -e MODEM_HOST=192.168.100.1 \
  -e MODEM_PASSWORD=your_password \
  ghcr.io/perezjoseph/mb8600-watchdog:latest
```

## 📦 Available Images

### 🔧 Standard Version
- **Image**: `ghcr.io/perezjoseph/mb8600-watchdog:latest`
- **Features**: Basic internet monitoring and modem rebooting
- **Use Case**: Simple, lightweight monitoring

### ⚡ Enhanced Version (Recommended)
- **Image**: `ghcr.io/perezjoseph/mb8600-watchdog:latest-enhanced`
- **Features**: 
  - 🧠 **TCP/IP Model Diagnostics**: Tests all network layers before rebooting
  - ⏱️ **Outage Duration Tracking**: Measures exactly how long internet was down
  - 📊 **Advanced Logging**: Structured JSON logs with rotation
  - 📈 **Availability Reports**: Periodic SLA compliance reports
  - 🎯 **Smart Reboot Decisions**: Avoids unnecessary reboots
- **Use Case**: Comprehensive network monitoring and analysis

## 🛠️ How it Works

### Standard Version
1. Checks internet connectivity at configurable intervals (default: every 60 seconds)
2. If connectivity fails for consecutive checks (default: 5), reboots the modem
3. Waits for recovery period before resuming monitoring

### Enhanced Version
1. **Connectivity Monitoring**: Same as standard version
2. **TCP/IP Diagnostics**: When failures reach threshold, runs comprehensive network tests:
   - Physical Layer: Interface status and statistics
   - Data Link Layer: ARP table and local network connectivity
   - Network Layer: IP configuration, routing, ICMP ping tests
   - Transport Layer: TCP/UDP connection tests
   - Application Layer: DNS resolution and HTTP requests
3. **Intelligent Decision**: Based on diagnostic results:
   - **Reboot**: If transport/network layer issues detected
   - **Skip Reboot**: If hardware issues or high success rate detected
   - **Monitor**: Continue monitoring with adjusted intervals
4. **Outage Tracking**: Measures and logs exact outage duration
5. **Reporting**: Generates periodic availability reports

## 📊 Monitoring Features (Enhanced Version)

### Real-Time Outage Tracking
```
Internet Down → Start Timer → Diagnostics → Decision → Resolution → Log Duration
```

### Example Log Output
```
2025-08-03 18:00:00 - WARNING - Internet outage started
2025-08-03 18:04:30 - INFO - Running TCP/IP diagnostics...
2025-08-03 18:04:35 - WARNING - Transport layer issues detected - reboot recommended
2025-08-03 18:05:30 - WARNING - Internet outage resolved after 330.5 seconds (5.5 minutes)
2025-08-03 19:00:00 - INFO - Availability Report - Total downtime: 5.5 minutes, Availability: 99.76%
```

## ⚙️ Configuration

### Environment Variables

#### Basic Settings (Both Versions)
```bash
MODEM_HOST=192.168.100.1          # Your modem's IP address
MODEM_USERNAME=admin              # Admin username (default: admin)
MODEM_PASSWORD=motorola           # Admin password (CHANGE THIS!)
MODEM_NOVERIFY=true              # Disable SSL verification
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

## 🐳 Docker Compose Profiles

### Available Profiles
- **`default`**: Enhanced version (runs when no profile specified)
- **`enhanced`**: Enhanced version with all features
- **`standard`**: Standard version (basic functionality)
- **`logs`**: Adds web-based log viewer

### Usage Examples
```bash
# Enhanced version (default)
docker-compose up -d

# Standard version only
docker-compose --profile standard up -d

# Enhanced version with log viewer
docker-compose --profile enhanced --profile logs up -d
# Access log viewer at http://localhost:8080

# Both versions (for comparison)
docker-compose --profile standard --profile enhanced up -d
```

## 📈 Log Analysis (Enhanced Version)

### JSON Log Queries
```bash
# Find all outage events
cat logs/watchdog.json | jq 'select(.extra.outage_started or .extra.outage_resolved)'

# Calculate total outage time
cat logs/watchdog.json | jq -r 'select(.extra.outage_duration_seconds) | .extra.outage_duration_seconds' | awk '{sum+=$1} END {print "Total:", sum/60, "minutes"}'

# Find diagnostic results
cat logs/watchdog.json | jq 'select(.extra.diagnostics_duration)'

# Get availability reports
cat logs/watchdog.json | jq 'select(.extra.outage_report == true)'
```

### Real-Time Monitoring
```bash
# View live logs
docker-compose logs -f internet-monitor-enhanced

# View structured logs
tail -f logs/watchdog.json | jq .

# Monitor outages only
tail -f logs/watchdog.json | jq 'select(.level == "WARNING" and (.extra.outage_started or .extra.outage_resolved))'
```

## 🧪 Testing

### Test Enhanced Features
```bash
# Test TCP/IP diagnostics
docker exec mb8600-watchdog-enhanced python3 test_tcp_ip_diagnostics.py

# Test outage tracking
docker exec mb8600-watchdog-enhanced python3 test_outage_tracking.py

# Test logging system
docker exec mb8600-watchdog-enhanced python3 test_logging.py
```

### Manual Testing
```bash
# Test modem reboot (dry run)
docker exec mb8600-watchdog-enhanced python3 modem_reboot.py --dryrun

# Test specific diagnostic layer
docker exec mb8600-watchdog-enhanced python3 test_tcp_ip_diagnostics.py network
```

## 🔧 Management Commands

### Deployment Management
```bash
# Deploy with interactive setup (uses remote images)
./deploy.sh

# Check status
./deploy.sh status

# Update to latest images
./deploy.sh update

# Stop services
./deploy.sh stop

# View help
./deploy.sh help
```

### Docker Compose Commands
```bash
# Start services (pulls remote images automatically)
docker compose up -d

# View logs
docker compose logs -f

# Stop services
docker compose down

# Update images and restart
docker compose pull && docker compose up -d

# Restart specific service
docker compose restart internet-monitor-enhanced
```

## 🔍 Troubleshooting

### Common Issues

#### Container Won't Start
```bash
# Check logs
docker-compose logs internet-monitor-enhanced

# Verify configuration
docker-compose config

# Check if modem is reachable
docker exec mb8600-watchdog-enhanced ping -c 3 192.168.100.1
```

#### Diagnostics Timing Out
```bash
# Increase timeout or disable diagnostics
docker-compose down
# Edit docker-compose.yml: DIAGNOSTICS_TIMEOUT=180 or ENABLE_DIAGNOSTICS=false
docker-compose up -d
```

#### Logs Not Persisting (Enhanced Version)
```bash
# Check permissions
ls -la logs/

# Fix permissions if needed
sudo chown -R 1000:1000 logs/
```

### Debug Mode
```bash
# Enable debug logging
docker-compose down
# Edit docker-compose.yml: LOG_LEVEL=DEBUG
docker-compose up -d

# View debug logs
docker-compose logs -f internet-monitor-enhanced
```

## 📚 Documentation

- **[Deployment Guide](DEPLOYMENT_GUIDE.md)**: Comprehensive deployment instructions
- **[TCP/IP Diagnostics](TCP_IP_DIAGNOSTICS.md)**: Detailed diagnostics documentation
- **[Outage Tracking](OUTAGE_TRACKING.md)**: Outage measurement and analysis
- **[Logging Improvements](LOGGING_IMPROVEMENTS.md)**: Advanced logging features
- **[Improvements Summary](IMPROVEMENTS_SUMMARY.md)**: Complete feature overview

## 🏗️ Architecture

### Standard Version
```
Internet Check → Failure Count → Threshold Reached → Reboot Modem → Wait → Resume
```

### Enhanced Version
```
Internet Check → Failure Count → Threshold Reached → TCP/IP Diagnostics → Smart Decision
                                                                              ↓
                                                    Reboot ← Yes ← Analysis → No → Monitor
                                                      ↓                              ↓
                                                   Wait & Track              Adjust & Continue
                                                      ↓
                                                   Resume
```

## 🤝 Contributing

1. Fork the repository
2. Create a feature branch
3. Make your changes
4. Test with both versions
5. Submit a pull request

### Development Setup
```bash
git clone https://github.com/perezjoseph/MB8600-watchdog.git
cd MB8600-watchdog

# Test standard version
docker-compose --profile standard up

# Test enhanced version
docker-compose --profile enhanced up

# Run tests
python3 test_tcp_ip_diagnostics.py
python3 test_outage_tracking.py
```

## 📄 License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.

## 🙏 Acknowledgments

- Original concept for Motorola/Arris modem HNAP API integration
- Community contributions for enhanced features
- Docker and containerization best practices

## 📞 Support

- **Issues**: [GitHub Issues](https://github.com/perezjoseph/MB8600-watchdog/issues)
- **Discussions**: [GitHub Discussions](https://github.com/perezjoseph/MB8600-watchdog/discussions)
- **Documentation**: See docs folder for detailed guides

---

**⚠️ Security Note**: Always change the default modem password from `motorola` to something secure!
