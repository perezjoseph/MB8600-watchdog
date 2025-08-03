# Internet Outage Duration Tracking

## Overview

The enhanced MB8600-watchdog now includes comprehensive internet outage duration tracking that measures and logs how long the internet was down. This provides valuable metrics for network reliability analysis, SLA monitoring, and troubleshooting.

## Features

### Real-Time Outage Tracking
- **Outage Start Detection**: Automatically detects when internet connectivity is lost
- **Duration Measurement**: Continuously tracks how long the outage lasts
- **Resolution Logging**: Records when connectivity is restored with total outage duration

### Comprehensive Logging
- **Outage Start**: Logs when an outage begins with timestamp
- **Ongoing Updates**: Debug logs showing current outage duration
- **Resolution Warnings**: Warning-level logs when outages are resolved
- **Reboot Correlation**: Tracks outage duration when reboots are triggered
- **Periodic Reports**: Hourly availability and downtime summaries

### Availability Metrics
- **Total Outage Time**: Cumulative downtime during monitoring session
- **Availability Percentage**: Uptime percentage calculation
- **Outage Statistics**: Detailed metrics in structured JSON logs

## Log Examples

### Outage Start
```
2025-08-03 18:00:00 - WARNING - Internet outage started at 2025-08-03 18:00:00
```

### Outage Resolution
```
2025-08-03 18:05:30 - WARNING - Internet outage resolved after 330.5 seconds (5.5 minutes)
```

### Reboot-Triggered Outage
```
2025-08-03 18:04:00 - WARNING - Reboot initiated after 240.0 seconds (4.0 minutes) of internet outage
```

### Periodic Availability Report
```
2025-08-03 19:00:00 - INFO - Outage Report - Total downtime: 8.5 minutes (0.14 hours), Availability: 99.76%
```

### Final Summary
```
2025-08-03 20:00:00 - WARNING - Total internet outage time during monitoring: 510.0 seconds (8.5 minutes, 0.14 hours) - 0.24% of uptime
```

## JSON Log Structure

### Outage Start Event
```json
{
  "timestamp": "2025-08-03T18:00:00.123456",
  "level": "WARNING",
  "message": "Internet outage started at 2025-08-03 18:00:00",
  "extra": {
    "outage_started": true,
    "outage_start_time": "2025-08-03T18:00:00.123456",
    "failure_count": 1
  }
}
```

### Outage Resolution Event
```json
{
  "timestamp": "2025-08-03T18:05:30.123456",
  "level": "WARNING",
  "message": "Internet outage resolved after 330.5 seconds (5.5 minutes)",
  "extra": {
    "outage_duration_seconds": 330.5,
    "outage_duration_minutes": 5.5,
    "outage_duration_hours": 0.092,
    "outage_start_time": "2025-08-03T18:00:00.123456",
    "outage_end_time": "2025-08-03T18:05:30.123456",
    "failure_count_during_outage": 5,
    "total_outage_duration_today": 330.5,
    "outage_resolved": true
  }
}
```

### Periodic Availability Report
```json
{
  "timestamp": "2025-08-03T19:00:00.123456",
  "level": "INFO",
  "message": "Outage Report - Total downtime: 8.5 minutes (0.14 hours), Availability: 99.76%",
  "extra": {
    "outage_report": true,
    "total_outage_duration_seconds": 510.0,
    "total_outage_duration_minutes": 8.5,
    "total_outage_duration_hours": 0.14,
    "uptime_seconds": 3600,
    "uptime_hours": 1.0,
    "outage_percentage": 0.24,
    "availability_percentage": 99.76,
    "current_outage_duration_seconds": 0,
    "current_outage_ongoing": false
  }
}
```

## Configuration Options

### Environment Variables
```bash
# Outage reporting interval (seconds)
OUTAGE_REPORT_INTERVAL=3600      # Default: 1 hour (3600 seconds)
```

### Command Line Arguments
```bash
# Set outage report interval
--outage-report-interval 3600    # Report every hour
--outage-report-interval 1800    # Report every 30 minutes
--outage-report-interval 86400   # Report daily
```

## Usage Examples

### Basic Usage
```bash
# Run with default hourly outage reports
python3 monitor_internet_improved.py

# Generate outage reports every 30 minutes
python3 monitor_internet_improved.py --outage-report-interval 1800

# Generate outage reports every 6 hours
python3 monitor_internet_improved.py --outage-report-interval 21600
```

### Docker Configuration
```yaml
environment:
  # Generate outage reports every hour
  - OUTAGE_REPORT_INTERVAL=3600
  
  # Or every 30 minutes for more frequent reporting
  - OUTAGE_REPORT_INTERVAL=1800
```

### Test Outage Tracking
```bash
# Run comprehensive outage tracking test
python3 test_outage_tracking.py

# Test just the metrics calculation
python3 test_outage_tracking.py metrics
```

## Log Analysis Queries

### Find All Outage Events
```bash
# Find outage start events
cat logs/watchdog.json | jq 'select(.extra.outage_started == true)'

# Find outage resolution events
cat logs/watchdog.json | jq 'select(.extra.outage_resolved == true)'

# Find reboot-triggered outages
cat logs/watchdog.json | jq 'select(.extra.reboot_trigger_outage_duration_seconds)'
```

### Calculate Outage Statistics
```bash
# Calculate total outage time
cat logs/watchdog.json | jq -r 'select(.extra.outage_duration_seconds) | .extra.outage_duration_seconds' | awk '{sum+=$1} END {print "Total outage time:", sum, "seconds (" sum/60 " minutes)"}'

# Calculate average outage duration
cat logs/watchdog.json | jq -r 'select(.extra.outage_duration_seconds) | .extra.outage_duration_seconds' | awk '{sum+=$1; count++} END {print "Average outage:", sum/count, "seconds"}'

# Count number of outages
cat logs/watchdog.json | jq 'select(.extra.outage_resolved == true)' | wc -l

# Find longest outage
cat logs/watchdog.json | jq -r 'select(.extra.outage_duration_seconds) | .extra.outage_duration_seconds' | sort -n | tail -1
```

### Availability Analysis
```bash
# Find availability reports
cat logs/watchdog.json | jq 'select(.extra.outage_report == true)'

# Extract availability percentages
cat logs/watchdog.json | jq -r 'select(.extra.availability_percentage) | .extra.availability_percentage'

# Find periods with low availability
cat logs/watchdog.json | jq 'select(.extra.availability_percentage < 99.0)'
```

## Monitoring Integration

### Prometheus Metrics
Extract metrics from JSON logs for Prometheus:

```bash
# Outage duration metric
cat logs/watchdog.json | jq -r 'select(.extra.outage_duration_seconds) | 
  "\(.timestamp) internet_outage_duration_seconds \(.extra.outage_duration_seconds)"'

# Availability percentage metric
cat logs/watchdog.json | jq -r 'select(.extra.availability_percentage) | 
  "\(.timestamp) internet_availability_percentage \(.extra.availability_percentage)"'

# Outage count metric
cat logs/watchdog.json | jq -r 'select(.extra.outage_resolved == true) | 
  "\(.timestamp) internet_outage_count 1"'
```

### Grafana Dashboards
Create dashboards showing:
- **Outage Duration Over Time**: Track outage lengths
- **Availability Percentage**: Monitor uptime trends
- **Outage Frequency**: Count outages per day/week
- **Mean Time to Recovery**: Average outage duration

### Alerting Rules
Set up alerts for:
- **Long Outages**: Alert if outage exceeds threshold (e.g., 10 minutes)
- **Low Availability**: Alert if availability drops below SLA (e.g., 99.9%)
- **Frequent Outages**: Alert if too many outages in time period

## SLA Monitoring

### Common SLA Targets
- **99.9% (Three Nines)**: 8.77 hours downtime per year
- **99.95%**: 4.38 hours downtime per year  
- **99.99% (Four Nines)**: 52.6 minutes downtime per year

### SLA Calculation Examples
```bash
# Calculate monthly availability (30 days)
UPTIME_SECONDS=2592000  # 30 days in seconds
OUTAGE_SECONDS=3600     # 1 hour outage
AVAILABILITY=$(echo "scale=4; ($UPTIME_SECONDS - $OUTAGE_SECONDS) / $UPTIME_SECONDS * 100" | bc)
echo "Monthly availability: $AVAILABILITY%"

# Check if SLA target is met
SLA_TARGET=99.9
if (( $(echo "$AVAILABILITY >= $SLA_TARGET" | bc -l) )); then
    echo "SLA target of $SLA_TARGET% met"
else
    echo "SLA target of $SLA_TARGET% NOT met"
fi
```

## Benefits

### Network Reliability Analysis
- **Trend Analysis**: Track outage patterns over time
- **Root Cause Analysis**: Correlate outages with events
- **Performance Baseline**: Establish normal availability levels
- **Improvement Tracking**: Measure impact of network changes

### SLA Compliance
- **Automated Monitoring**: Continuous availability tracking
- **Historical Records**: Long-term availability data
- **Compliance Reporting**: Generate SLA compliance reports
- **Proactive Alerting**: Early warning of SLA violations

### Troubleshooting
- **Outage Correlation**: Link outages to specific causes
- **Recovery Time Analysis**: Measure effectiveness of fixes
- **Pattern Recognition**: Identify recurring issues
- **Impact Assessment**: Quantify business impact of outages

### Operational Insights
- **Capacity Planning**: Understand network reliability needs
- **Vendor Management**: Hold ISPs accountable with data
- **Budget Justification**: Justify network infrastructure investments
- **Performance Optimization**: Focus improvements on biggest issues

## Advanced Features

### Custom Outage Thresholds
Future enhancement to define custom outage severity levels:
- **Minor**: < 1 minute
- **Major**: 1-10 minutes  
- **Critical**: > 10 minutes

### Outage Classification
Planned enhancements for outage categorization:
- **Connectivity**: Network layer issues
- **DNS**: Application layer issues
- **Hardware**: Physical layer issues
- **Planned**: Scheduled maintenance

### Integration APIs
Future API endpoints for:
- **Real-time Status**: Current outage status
- **Historical Data**: Outage history queries
- **Metrics Export**: Prometheus/InfluxDB integration
- **Webhook Notifications**: Real-time outage alerts

This outage duration tracking system provides comprehensive visibility into internet connectivity reliability, enabling data-driven decisions about network infrastructure and helping maintain SLA compliance.
