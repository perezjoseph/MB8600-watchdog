# TCP/IP Model Diagnostics for MB8600-Watchdog

## Overview

The enhanced MB8600-watchdog now includes comprehensive TCP/IP model diagnostics that run before deciding whether to reboot the modem. This intelligent system tests connectivity at all layers of the TCP/IP stack to determine if a modem reboot will actually resolve the connectivity issues.

## Why TCP/IP Diagnostics?

Traditional connectivity monitoring only tests end-to-end connectivity (ping/HTTP). This approach can lead to unnecessary modem reboots when the issue is:

- **Physical layer**: Cable problems, interface issues
- **Data link layer**: Switch problems, local network issues  
- **Network layer**: Routing configuration, IP issues
- **Transport layer**: Firewall rules, port blocking
- **Application layer**: DNS problems, service outages

By testing each layer systematically, the watchdog can make intelligent decisions about whether a modem reboot will help.

## TCP/IP Layers Tested

### Layer 1: Physical Layer
- **Network Interface Status**: Checks if network interfaces are up and configured
- **Interface Statistics**: Monitors packet counts and error rates
- **Purpose**: Detect cable disconnections, interface failures

### Layer 2: Data Link Layer  
- **ARP Table**: Verifies local network device discovery
- **Local Network Connectivity**: Tests connectivity to gateway/router
- **Purpose**: Detect switch issues, local network problems

### Layer 3: Network Layer
- **IP Configuration**: Verifies IP address assignment and routing table
- **ICMP Ping Tests**: Tests connectivity to:
  - Gateway/Router
  - Modem management interface
  - Public DNS servers (1.1.1.1, 8.8.8.8)
  - Public websites (google.com)
- **Purpose**: Detect routing issues, IP configuration problems

### Layer 4: Transport Layer
- **TCP Connection Tests**: Tests TCP connectivity to:
  - Modem HTTPS/HTTP ports (443/80)
  - DNS servers (port 53)
  - Public HTTPS/HTTP services
- **UDP Connection Tests**: Tests UDP connectivity to DNS servers
- **Purpose**: Detect firewall issues, port blocking

### Layer 5: Application Layer
- **DNS Resolution**: Tests domain name resolution for multiple domains
- **HTTP Requests**: Tests full HTTP connectivity to various websites
- **Purpose**: Detect DNS issues, application-level problems

## Intelligent Reboot Decision Logic

The system analyzes diagnostic results using the following logic:

### Don't Reboot When:
1. **Physical/Data Link Issues**: Cable problems, interface failures
   - Reboot won't fix hardware issues
   - Recommends checking cables and hardware

2. **High Success Rate**: >70% of tests pass
   - Minor issues that may resolve themselves
   - Recommends continued monitoring

3. **DNS-Only Issues**: Only DNS resolution fails, but HTTP works
   - May indicate DNS server problems
   - Reboot might help with DNS cache

### Do Reboot When:
1. **Transport Layer Issues**: TCP/UDP connectivity problems
   - Indicates modem connectivity issues
   - Reboot likely to help

2. **Network Layer Issues**: Routing or ICMP problems
   - Multiple ping failures or routing issues
   - Reboot can reset network stack

3. **Significant Failures**: <70% success rate with multiple layer issues
   - Widespread connectivity problems
   - Reboot recommended as general fix

## Configuration Options

### Environment Variables
```bash
# Enable/disable diagnostics
ENABLE_DIAGNOSTICS=true          # Enable TCP/IP diagnostics (default: true)

# Diagnostics timeout
DIAGNOSTICS_TIMEOUT=120          # Timeout in seconds (default: 120)
```

### Command Line Arguments
```bash
# Enable diagnostics (default)
--enable-diagnostics

# Disable diagnostics (revert to simple reboot)
--disable-diagnostics

# Set diagnostics timeout
--diagnostics-timeout 120
```

## Usage Examples

### Basic Usage with Diagnostics
```bash
# Run with diagnostics enabled (default)
python3 monitor_internet_improved.py

# Run with diagnostics explicitly enabled
python3 monitor_internet_improved.py --enable-diagnostics

# Run with custom diagnostics timeout
python3 monitor_internet_improved.py --diagnostics-timeout 180
```

### Disable Diagnostics
```bash
# Disable diagnostics for simple reboot behavior
python3 monitor_internet_improved.py --disable-diagnostics

# Or via environment variable
ENABLE_DIAGNOSTICS=false python3 monitor_internet_improved.py
```

### Test Diagnostics Manually
```bash
# Run comprehensive diagnostics test
python3 test_tcp_ip_diagnostics.py

# Test specific layer
python3 test_tcp_ip_diagnostics.py physical
python3 test_tcp_ip_diagnostics.py network
python3 test_tcp_ip_diagnostics.py application
```

## Docker Configuration

### Docker Compose
```yaml
environment:
  # Enable diagnostics
  - ENABLE_DIAGNOSTICS=true
  - DIAGNOSTICS_TIMEOUT=120
  
  # Other settings
  - MODEM_HOST=192.168.100.1
  - CHECK_INTERVAL=60
  - FAILURE_THRESHOLD=5
```

### Docker Run
```bash
docker run -d \
  -e ENABLE_DIAGNOSTICS=true \
  -e DIAGNOSTICS_TIMEOUT=120 \
  -e MODEM_HOST=192.168.100.1 \
  mb8600-watchdog
```

## Log Output Examples

### Diagnostics Initiated
```
2025-08-03 18:00:00 - WARNING - Failure threshold reached (5). Running TCP/IP diagnostics...
2025-08-03 18:00:00 - INFO - Starting comprehensive TCP/IP model diagnostics
```

### Layer Test Results
```
2025-08-03 18:00:01 - DEBUG - Testing Physical Layer
2025-08-03 18:00:01 - DEBUG - Network interfaces found: 2 active, 3 total
2025-08-03 18:00:02 - DEBUG - Testing Network Layer  
2025-08-03 18:00:02 - INFO - Successfully pinged gateway (192.168.1.1) in 0.05s
2025-08-03 18:00:03 - WARNING - Failed to ping 8.8.8.8: Network unreachable
```

### Reboot Decision
```
2025-08-03 18:00:05 - WARNING - Network diagnostics recommend modem reboot
2025-08-03 18:00:05 - INFO - Diagnostic results: 15 tests, 8 passed, 7 failed
2025-08-03 18:00:05 - INFO - Failure analysis: Transport layer issues detected
```

### Reboot Skipped
```
2025-08-03 18:00:05 - INFO - Network diagnostics suggest reboot may not resolve issues
2025-08-03 18:00:05 - INFO - Recommended actions: ['check_cables_and_hardware']
2025-08-03 18:00:05 - INFO - Waiting 300 seconds before next check (diagnostics mode)
```

## JSON Log Structure

Diagnostic results are logged in structured JSON format:

```json
{
  "timestamp": "2025-08-03T18:00:00.123456",
  "level": "INFO",
  "message": "Network diagnostics completed in 5.23s",
  "extra": {
    "diagnostics_duration": 5.23,
    "total_tests": 15,
    "total_passed": 8,
    "total_failed": 7,
    "layer_stats": {
      "Physical": {"passed": 2, "failed": 0, "total": 2},
      "Network": {"passed": 2, "failed": 3, "total": 5},
      "Transport": {"passed": 1, "failed": 3, "total": 4},
      "Application": {"passed": 3, "failed": 1, "total": 4}
    },
    "should_reboot": true,
    "failure_analysis": {
      "failures_by_layer": {
        "Network": ["ICMP Ping (8.8.8.8)", "ICMP Ping (google.com)"],
        "Transport": ["TCP Connection (HTTPS)", "UDP Connection (DNS)"]
      },
      "likely_causes": ["Transport layer issues", "Network connectivity problems"],
      "recommended_actions": ["reboot_recommended"]
    }
  }
}
```

## Performance Impact

- **Typical Duration**: 5-15 seconds for full diagnostics
- **Timeout Protection**: Configurable timeout prevents hanging
- **Resource Usage**: Minimal CPU/memory impact
- **Network Impact**: Light network testing, similar to normal connectivity checks

## Benefits

### Reduced Unnecessary Reboots
- Avoids reboots when hardware issues are detected
- Prevents reboots for temporary DNS/service outages
- Reduces wear on modem hardware

### Better Problem Diagnosis
- Identifies root cause of connectivity issues
- Provides actionable recommendations
- Helps with network troubleshooting

### Improved Reliability
- More intelligent decision making
- Comprehensive connectivity testing
- Better logging and monitoring

### Operational Insights
- Historical data on network layer performance
- Trends in connectivity issues
- Better understanding of network health

## Troubleshooting

### Diagnostics Taking Too Long
```bash
# Reduce timeout
--diagnostics-timeout 60

# Or disable diagnostics temporarily
--disable-diagnostics
```

### False Positives/Negatives
```bash
# Test diagnostics manually
python3 test_tcp_ip_diagnostics.py

# Check logs for detailed results
tail -f logs/watchdog.json | jq 'select(.extra.diagnostics_duration)'
```

### Permission Issues
Some diagnostic tests require network access. Ensure the container/process has appropriate network permissions.

## Migration from Simple Reboot

The diagnostics feature is enabled by default but maintains backward compatibility:

1. **Existing behavior**: Use `--disable-diagnostics` to revert to simple reboot
2. **Gradual adoption**: Start with diagnostics enabled and monitor results
3. **Configuration**: Adjust timeout and settings based on your network

## Future Enhancements

Planned improvements include:
- **Custom test configuration**: Define specific tests for your network
- **Machine learning**: Learn from historical patterns
- **Integration**: Export metrics to monitoring systems
- **Advanced analysis**: More sophisticated failure pattern recognition

This TCP/IP diagnostics system transforms the MB8600-watchdog from a simple reboot tool into an intelligent network monitoring and troubleshooting system.
