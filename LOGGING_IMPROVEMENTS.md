# Logging Improvements for MB8600-Watchdog

This document outlines the comprehensive logging improvements made to the MB8600-watchdog project.

## Overview of Improvements

### 1. Enhanced Logging Infrastructure

#### Multiple Log Formats
- **Console Output**: Simple, human-readable format for real-time monitoring
- **Detailed File Logs**: Comprehensive format with function names, line numbers, and timestamps
- **JSON Structured Logs**: Machine-readable format for log analysis and monitoring tools
- **Error-Only Logs**: Separate file for errors and critical issues

#### Log Rotation
- Automatic log rotation when files reach 10MB (configurable)
- Keeps 5 backup files by default (configurable)
- Prevents disk space issues from unlimited log growth

### 2. Improved Files

#### `monitor_internet_improved.py`
Enhanced version of the main monitoring script with:

- **Structured Logging**: Each log entry includes contextual data
- **Performance Metrics**: Timing information for all operations
- **Error Context**: Detailed error information with stack traces
- **Connection Tracking**: Detailed tracking of ping and HTTP checks
- **Reboot Monitoring**: Comprehensive logging of reboot processes

#### `modem_reboot_improved.py`
Enhanced version of the reboot script with:

- **Step-by-Step Logging**: Detailed logging of each authentication step
- **Timing Analysis**: Performance metrics for all operations
- **Error Diagnostics**: Enhanced error reporting with context
- **Connection Monitoring**: Detailed tracking of network connectivity

### 3. Configuration Options

#### Environment Variables
```bash
# Logging configuration
LOG_LEVEL=INFO              # DEBUG, INFO, WARNING, ERROR, CRITICAL
LOG_FILE=/app/logs/watchdog.log  # Path to log file
LOG_MAX_SIZE=10485760       # Max log file size (10MB)
LOG_BACKUP_COUNT=5          # Number of backup files

# Existing modem settings
MODEM_HOST=192.168.100.1
MODEM_USERNAME=admin
MODEM_PASSWORD=motorola
CHECK_INTERVAL=60
FAILURE_THRESHOLD=5
RECOVERY_WAIT=600
```

#### Command Line Arguments
```bash
# Enhanced logging options
--log-level DEBUG
--log-file /path/to/logfile.log
--log-max-size 10485760
--log-backup-count 5

# All existing options still supported
--host 192.168.100.1
--username admin
--password motorola
--check-interval 60
--failure-threshold 5
--recovery-wait 600
```

### 4. Docker Integration

#### Enhanced Docker Compose (`docker-compose-improved.yml`)
- **Log Volume Mounting**: Persistent log storage
- **Log Viewer Service**: Optional Dozzle service for web-based log viewing
- **Health Checks**: Container health monitoring
- **Environment Configuration**: All settings via environment variables

#### Improved Dockerfile (`Dockerfile-improved`)
- **Log Directory Creation**: Proper log directory setup
- **Health Checks**: Built-in container health monitoring
- **Security**: Non-root user execution
- **Dependencies**: All required packages included

### 5. Log Analysis Features

#### Structured Data in Logs
Each log entry can include contextual information:

```json
{
  "timestamp": "2025-08-03T18:00:00.123456",
  "level": "INFO",
  "logger": "__main__",
  "function": "check_internet",
  "message": "Internet connection is UP",
  "extra": {
    "connection_status": "UP",
    "check_duration": 2.34,
    "failure_count": 0,
    "method": "ping",
    "host": "1.1.1.1"
  }
}
```

#### Performance Monitoring
- Connection check durations
- Login process timing
- Reboot cycle monitoring
- Network latency tracking

#### Error Tracking
- Detailed error messages with context
- Stack traces for debugging
- Error categorization and counting
- Recovery attempt logging

### 6. Usage Examples

#### Basic Usage with Enhanced Logging
```bash
# Run with INFO level logging to file
python3 monitor_internet_improved.py --log-level INFO --log-file /var/log/watchdog.log

# Run with DEBUG level for troubleshooting
python3 monitor_internet_improved.py --log-level DEBUG --log-file /tmp/debug.log

# Test reboot functionality with detailed logging
python3 modem_reboot_improved.py --dryrun --log-level DEBUG --log-file /tmp/reboot-test.log
```

#### Docker Usage
```bash
# Run with log viewer
docker-compose -f docker-compose-improved.yml --profile logs up -d

# View logs in real-time
docker-compose -f docker-compose-improved.yml logs -f mb8600-watchdog

# Access web-based log viewer
# Open http://localhost:8080 in browser
```

### 7. Log File Structure

```
logs/
├── watchdog.log          # Main application log (rotated)
├── watchdog.log.1        # Previous log file
├── watchdog.log.2        # Older log file
├── watchdog.json         # Structured JSON logs
├── watchdog.json.1       # Previous JSON log
└── errors.log            # Error-only log file
```

### 8. Monitoring and Alerting

#### Log Analysis Queries
With structured JSON logs, you can easily query for:

```bash
# Find all reboot events
jq 'select(.extra.action == "reboot_initiated")' watchdog.json

# Calculate average connection check times
jq -r 'select(.extra.check_duration) | .extra.check_duration' watchdog.json | awk '{sum+=$1; count++} END {print sum/count}'

# Find all errors in the last hour
jq 'select(.level == "ERROR" and (.timestamp | fromdateiso8601) > (now - 3600))' watchdog.json
```

#### Integration with Monitoring Systems
- **Prometheus**: Use JSON logs with log exporters
- **ELK Stack**: Direct JSON log ingestion
- **Grafana**: Visualization of metrics from logs
- **Alertmanager**: Alerts based on error patterns

### 9. Benefits

#### For Troubleshooting
- **Detailed Context**: Every operation includes timing and context
- **Error Diagnosis**: Stack traces and error categorization
- **Performance Analysis**: Identify slow operations and bottlenecks
- **Historical Data**: Rotated logs provide historical perspective

#### For Monitoring
- **Structured Data**: Easy parsing and analysis
- **Metrics Extraction**: Performance and reliability metrics
- **Alerting**: Automated alerts based on log patterns
- **Dashboards**: Visual monitoring of system health

#### For Operations
- **Reliability**: Better understanding of system behavior
- **Maintenance**: Proactive identification of issues
- **Capacity Planning**: Historical performance data
- **Compliance**: Comprehensive audit trails

### 10. Migration Guide

#### From Original to Improved Version
1. **Backup existing setup**
2. **Update Python files**: Use `*_improved.py` versions
3. **Update Docker configuration**: Use improved Docker files
4. **Configure logging**: Set environment variables or command line args
5. **Test thoroughly**: Verify logging works as expected

#### Backward Compatibility
- All original command line arguments still work
- Environment variables are backward compatible
- Docker compose can run alongside original version
- Log files are in addition to console output

This enhanced logging system provides comprehensive visibility into the MB8600-watchdog operation, making it easier to troubleshoot issues, monitor performance, and maintain reliable internet connectivity monitoring.
