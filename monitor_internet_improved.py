#!/usr/bin/env python3
# Monitor internet connectivity and reboot modem if connection fails
# Enhanced version with improved logging capabilities
import time
import subprocess
import logging
import logging.handlers
import sys
import requests
import argparse
import json
import os
from datetime import datetime
from pathlib import Path

# Import the modem reboot functionality
from modem_reboot import SurfboardHNAP, wait_for_reboot_cycle, is_host_reachable
from network_diagnostics import NetworkDiagnostics, NetworkLayer

def setup_logging(log_level='INFO', log_file=None, log_max_size=10*1024*1024, log_backup_count=5):
    """
    Setup comprehensive logging with rotation, structured format, and multiple handlers.
    
    Args:
        log_level: Logging level (DEBUG, INFO, WARNING, ERROR, CRITICAL)
        log_file: Path to log file (if None, logs to stdout only)
        log_max_size: Maximum size of log file before rotation (bytes)
        log_backup_count: Number of backup log files to keep
    """
    # Create logger
    logger = logging.getLogger(__name__)
    logger.setLevel(getattr(logging, log_level.upper()))
    
    # Clear any existing handlers
    logger.handlers.clear()
    
    # Create formatters
    detailed_formatter = logging.Formatter(
        '%(asctime)s | %(levelname)-8s | %(name)s:%(lineno)d | %(funcName)s() | %(message)s',
        datefmt='%Y-%m-%d %H:%M:%S'
    )
    
    simple_formatter = logging.Formatter(
        '%(asctime)s - %(levelname)s - %(message)s',
        datefmt='%Y-%m-%d %H:%M:%S'
    )
    
    # Console handler (always present)
    console_handler = logging.StreamHandler(sys.stdout)
    console_handler.setLevel(logging.INFO)
    console_handler.setFormatter(simple_formatter)
    logger.addHandler(console_handler)
    
    # File handler with rotation (if log_file specified)
    if log_file:
        # Ensure log directory exists
        log_path = Path(log_file)
        log_path.parent.mkdir(parents=True, exist_ok=True)
        
        file_handler = logging.handlers.RotatingFileHandler(
            log_file,
            maxBytes=log_max_size,
            backupCount=log_backup_count,
            encoding='utf-8'
        )
        file_handler.setLevel(logging.DEBUG)
        file_handler.setFormatter(detailed_formatter)
        logger.addHandler(file_handler)
        
        # Also create a JSON log handler for structured logging
        json_log_file = str(log_path.with_suffix('.json'))
        json_handler = logging.handlers.RotatingFileHandler(
            json_log_file,
            maxBytes=log_max_size,
            backupCount=log_backup_count,
            encoding='utf-8'
        )
        json_handler.setLevel(logging.INFO)
        json_handler.setFormatter(JsonFormatter())
        logger.addHandler(json_handler)
    
    return logger

class JsonFormatter(logging.Formatter):
    """Custom JSON formatter for structured logging"""
    
    def format(self, record):
        log_entry = {
            'timestamp': datetime.fromtimestamp(record.created).isoformat(),
            'level': record.levelname,
            'logger': record.name,
            'module': record.module,
            'function': record.funcName,
            'line': record.lineno,
            'message': record.getMessage(),
            'process_id': os.getpid()
        }
        
        # Add exception info if present
        if record.exc_info:
            log_entry['exception'] = self.formatException(record.exc_info)
        
        # Add extra fields if present
        if hasattr(record, 'extra_data'):
            log_entry['extra'] = record.extra_data
            
        return json.dumps(log_entry, ensure_ascii=False)

# Initialize logger (will be reconfigured in main())
logger = logging.getLogger(__name__)

def ping(host):
    """
    Ping a host and return True if it responds, False otherwise
    """
    try:
        logger.debug(f"Starting ping test to {host}")
        start_time = time.time()
        
        # Use -c for count, -W for timeout in seconds
        result = subprocess.check_output(
            ["ping", "-c", "3", "-W", "5", host],
            stderr=subprocess.STDOUT,
            universal_newlines=True
        )
        
        duration = time.time() - start_time
        logger.debug(f"Ping to {host} successful in {duration:.2f}s", 
                    extra={'extra_data': {'host': host, 'duration': duration, 'result': 'success'}})
        return True
        
    except subprocess.CalledProcessError as e:
        duration = time.time() - start_time
        logger.debug(f"Ping to {host} failed after {duration:.2f}s: {e.output.strip()}", 
                    extra={'extra_data': {'host': host, 'duration': duration, 'result': 'failed', 'error': str(e)}})
        return False
    except Exception as e:
        logger.error(f"Unexpected error pinging {host}: {e}", 
                    extra={'extra_data': {'host': host, 'error': str(e), 'error_type': type(e).__name__}})
        return False

def http_check(url):
    """
    Make HTTP request to URL and return True if successful, False otherwise
    """
    try:
        logger.debug(f"Starting HTTP check to {url}")
        start_time = time.time()
        
        response = requests.get(url, timeout=10)
        duration = time.time() - start_time
        
        success = response.status_code == 200
        logger.debug(f"HTTP check to {url} {'successful' if success else 'failed'} "
                    f"(status: {response.status_code}) in {duration:.2f}s",
                    extra={'extra_data': {
                        'url': url, 
                        'status_code': response.status_code, 
                        'duration': duration,
                        'result': 'success' if success else 'failed'
                    }})
        return success
        
    except requests.RequestException as e:
        duration = time.time() - start_time
        logger.debug(f"HTTP check to {url} failed after {duration:.2f}s: {e}",
                    extra={'extra_data': {
                        'url': url, 
                        'duration': duration, 
                        'result': 'failed', 
                        'error': str(e),
                        'error_type': type(e).__name__
                    }})
        return False
    except Exception as e:
        logger.error(f"Unexpected error during HTTP check to {url}: {e}",
                    extra={'extra_data': {'url': url, 'error': str(e), 'error_type': type(e).__name__}})
        return False

def run_network_diagnostics(modem_host):
    """
    Run comprehensive TCP/IP model diagnostics before deciding to reboot
    
    Args:
        modem_host: IP address of the modem
        
    Returns:
        Tuple of (should_reboot, diagnostics_results, failure_analysis)
    """
    logger.info("Running comprehensive TCP/IP model diagnostics before reboot decision",
               extra={'extra_data': {'modem_host': modem_host}})
    
    diagnostics_start = time.time()
    
    try:
        # Initialize network diagnostics
        network_diag = NetworkDiagnostics()
        
        # Run full diagnostics
        results = network_diag.run_full_diagnostics(modem_host)
        
        # Analyze results
        failure_analysis = network_diag.get_failure_analysis()
        
        diagnostics_duration = time.time() - diagnostics_start
        
        # Count successes and failures by layer
        layer_stats = {}
        for result in results:
            layer = result.layer.value
            if layer not in layer_stats:
                layer_stats[layer] = {'passed': 0, 'failed': 0, 'total': 0}
            
            layer_stats[layer]['total'] += 1
            if result.success:
                layer_stats[layer]['passed'] += 1
            else:
                layer_stats[layer]['failed'] += 1
        
        # Determine if reboot is necessary based on diagnostic results
        should_reboot = _analyze_reboot_necessity(results, failure_analysis)
        
        logger.info(f"Network diagnostics completed in {diagnostics_duration:.2f}s",
                   extra={'extra_data': {
                       'diagnostics_duration': diagnostics_duration,
                       'total_tests': len(results),
                       'total_passed': sum(1 for r in results if r.success),
                       'total_failed': sum(1 for r in results if not r.success),
                       'layer_stats': layer_stats,
                       'should_reboot': should_reboot,
                       'failure_analysis': failure_analysis
                   }})
        
        return should_reboot, results, failure_analysis
        
    except Exception as e:
        diagnostics_duration = time.time() - diagnostics_start
        logger.error(f"Network diagnostics failed after {diagnostics_duration:.2f}s: {e}",
                    extra={'extra_data': {
                        'diagnostics_duration': diagnostics_duration,
                        'error': str(e),
                        'error_type': type(e).__name__
                    }}, exc_info=True)
        
        # If diagnostics fail, default to reboot (conservative approach)
        return True, [], {'error': str(e)}

def _analyze_reboot_necessity(results, failure_analysis):
    """
    Analyze diagnostic results to determine if modem reboot is necessary
    
    Args:
        results: List of DiagnosticResult objects
        failure_analysis: Dictionary with failure analysis
        
    Returns:
        Boolean indicating if reboot is recommended
    """
    # Count failures by layer
    layer_failures = {}
    for result in results:
        if not result.success:
            layer = result.layer.value
            layer_failures[layer] = layer_failures.get(layer, 0) + 1
    
    # Decision logic based on TCP/IP layer failures
    
    # If physical or data link layer issues, reboot likely won't help
    if (NetworkLayer.PHYSICAL.value in layer_failures or 
        NetworkLayer.DATA_LINK.value in layer_failures):
        
        # Check if it's just interface statistics (not critical)
        physical_failures = [r for r in results 
                           if r.layer == NetworkLayer.PHYSICAL and not r.success]
        critical_physical_failures = [f for f in physical_failures 
                                    if 'Interface Statistics' not in f.test_name]
        
        if critical_physical_failures:
            logger.warning("Physical/Data Link layer issues detected - reboot may not resolve",
                          extra={'extra_data': {
                              'layer_failures': layer_failures,
                              'recommendation': 'check_cables_and_hardware'
                          }})
            return False  # Don't reboot for hardware issues
    
    # If network layer has issues but transport/application work, might be routing
    network_failures = layer_failures.get(NetworkLayer.NETWORK.value, 0)
    transport_failures = layer_failures.get(NetworkLayer.TRANSPORT.value, 0)
    app_failures = layer_failures.get(NetworkLayer.APPLICATION.value, 0)
    
    # If only application layer fails, might be DNS or specific service issues
    if app_failures > 0 and transport_failures == 0 and network_failures == 0:
        logger.info("Only application layer issues detected - checking if DNS related",
                   extra={'extra_data': {
                       'layer_failures': layer_failures,
                       'recommendation': 'check_dns_configuration'
                   }})
        
        # Check if DNS resolution is the main issue
        dns_failures = [r for r in results 
                       if not r.success and 'DNS Resolution' in r.test_name]
        http_failures = [r for r in results 
                        if not r.success and 'HTTP Request' in r.test_name]
        
        if len(dns_failures) > 0 and len(http_failures) == 0:
            # DNS issues only - might be resolved by reboot
            return True
    
    # If transport layer has issues, likely connectivity problems
    if transport_failures > 0:
        logger.info("Transport layer issues detected - likely connectivity problems",
                   extra={'extra_data': {
                       'layer_failures': layer_failures,
                       'recommendation': 'reboot_recommended'
                   }})
        return True
    
    # If network layer has routing issues, reboot might help
    if network_failures > 0:
        # Check if it's routing or ICMP issues
        icmp_failures = [r for r in results 
                        if not r.success and 'ICMP Ping' in r.test_name]
        routing_failures = [r for r in results 
                           if not r.success and ('Routing' in r.test_name or 'IP Configuration' in r.test_name)]
        
        if len(icmp_failures) > 2 or len(routing_failures) > 0:
            logger.info("Network layer routing/connectivity issues detected",
                       extra={'extra_data': {
                           'layer_failures': layer_failures,
                           'icmp_failures': len(icmp_failures),
                           'routing_failures': len(routing_failures),
                           'recommendation': 'reboot_recommended'
                       }})
            return True
    
    # If most tests pass, might not need reboot
    total_tests = len(results)
    failed_tests = sum(1 for r in results if not r.success)
    success_rate = (total_tests - failed_tests) / total_tests if total_tests > 0 else 0
    
    if success_rate > 0.7:  # If more than 70% of tests pass
        logger.info(f"High success rate ({success_rate:.1%}) in diagnostics - reboot may not be necessary",
                   extra={'extra_data': {
                       'success_rate': success_rate,
                       'total_tests': total_tests,
                       'failed_tests': failed_tests,
                       'recommendation': 'monitor_further'
                   }})
        return False
    
    # Default to reboot if significant failures
    logger.info("Significant network issues detected - recommending reboot",
               extra={'extra_data': {
                   'layer_failures': layer_failures,
                   'success_rate': success_rate,
                   'recommendation': 'reboot_recommended'
               }})
    return True
    """
    Check internet connectivity by pinging hosts and HTTP requests
    Returns True if at least one check succeeds, False otherwise
    """
    logger.debug("Starting internet connectivity check")
    check_results = {'ping': {}, 'http': {}}
    
    # First try ping checks
    for host in ping_hosts:
        logger.info(f"Pinging {host}...")
        result = ping(host)
        check_results['ping'][host] = result
        if result:
            logger.info(f"Successfully pinged {host}")
            logger.debug("Internet connectivity confirmed via ping", 
                        extra={'extra_data': {'method': 'ping', 'host': host, 'all_results': check_results}})
            return True

    # Then try HTTP checks
    for url in http_hosts:
        logger.info(f"HTTP check to {url}...")
        result = http_check(url)
        check_results['http'][url] = result
        if result:
            logger.info(f"Successfully connected to {url}")
            logger.debug("Internet connectivity confirmed via HTTP", 
                        extra={'extra_data': {'method': 'http', 'url': url, 'all_results': check_results}})
            return True
    
    logger.warning("All internet connectivity checks failed", 
                  extra={'extra_data': {'all_results': check_results}})
    return False

def reboot_modem(host, username, password, noverify):
    """
    Reboot the modem using the modem_reboot functionality
    """
    try:
        logger.info("Attempting to reboot modem...", 
                   extra={'extra_data': {'modem_host': host, 'username': username}})
        reboot_start_time = time.time()
        
        # Initialize client
        h = SurfboardHNAP(username)
        h.host = host
        if noverify:
            h.s.verify = False
        
        # Verify modem is reachable
        logger.debug(f"Checking if modem host {host} is reachable")
        if not is_host_reachable(host, port=443):
            logger.error(f"Host {host} is unreachable", 
                        extra={'extra_data': {'modem_host': host, 'port': 443, 'reachable': False}})
            return False
        
        logger.debug(f"Modem host {host} is reachable")
        
        # Perform login
        logger.info(f"Logging in to {host} as {username}...")
        login_start_time = time.time()
        r = h.login(host, password, noverify)
        login_duration = time.time() - login_start_time
        
        logger.info(f'Login completed in {login_duration:.2f}s: {r}', 
                   extra={'extra_data': {'login_duration': login_duration, 'response': str(r)}})
        
        if r is None:
            logger.error("HNAP login failed; unable to reboot", 
                        extra={'extra_data': {'login_result': None, 'login_duration': login_duration}})
            return False

        # Send reboot command
        logger.info("Sending reboot command...")
        reboot_cmd_start = time.time()
        reboot_resp = h.reboot()
        reboot_cmd_duration = time.time() - reboot_cmd_start
        
        logger.info(f'Reboot command completed in {reboot_cmd_duration:.2f}s: {reboot_resp}',
                   extra={'extra_data': {
                       'reboot_cmd_duration': reboot_cmd_duration,
                       'status_code': reboot_resp.status_code,
                       'response': str(reboot_resp)
                   }})
        
        if reboot_resp.status_code != 200:
            logger.error(f"Reboot request failed with status {reboot_resp.status_code}",
                        extra={'extra_data': {
                            'status_code': reboot_resp.status_code,
                            'response_text': reboot_resp.text
                        }})
            return False
            
        logger.info("Reboot command sent successfully")
        
        # Wait for reboot cycle
        logger.info("Waiting for modem reboot cycle to complete...")
        cycle_start_time = time.time()
        if not wait_for_reboot_cycle(host, verify=not noverify):
            cycle_duration = time.time() - cycle_start_time
            logger.error(f"Modem did not complete reboot cycle after {cycle_duration:.2f}s",
                        extra={'extra_data': {'cycle_duration': cycle_duration, 'cycle_completed': False}})
            return False
        
        cycle_duration = time.time() - cycle_start_time
        total_reboot_duration = time.time() - reboot_start_time
        
        logger.info(f"Modem reboot completed successfully in {total_reboot_duration:.2f}s "
                   f"(cycle: {cycle_duration:.2f}s)",
                   extra={'extra_data': {
                       'total_reboot_duration': total_reboot_duration,
                       'cycle_duration': cycle_duration,
                       'reboot_successful': True
                   }})
        return True
        
    except Exception as e:
        total_duration = time.time() - reboot_start_time if 'reboot_start_time' in locals() else 0
        logger.error(f"Error during modem reboot after {total_duration:.2f}s: {e}",
                    extra={'extra_data': {
                        'error': str(e),
                        'error_type': type(e).__name__,
                        'duration': total_duration
                    }}, exc_info=True)
        return False

def log_periodic_outage_report(total_outage_duration, uptime_seconds, outage_start_time=None):
    """
    Log periodic outage statistics report
    
    Args:
        total_outage_duration: Total outage time in seconds
        uptime_seconds: Total monitoring uptime in seconds
        outage_start_time: Current outage start time if ongoing
    """
    current_outage_duration = 0
    if outage_start_time:
        current_outage_duration = time.time() - outage_start_time
    
    outage_percentage = (total_outage_duration / uptime_seconds * 100) if uptime_seconds > 0 else 0
    availability_percentage = 100 - outage_percentage
    
    logger.info(f"Outage Report - Total downtime: {total_outage_duration/60:.1f} minutes "
               f"({total_outage_duration/3600:.2f} hours), Availability: {availability_percentage:.2f}%",
               extra={'extra_data': {
                   'outage_report': True,
                   'total_outage_duration_seconds': total_outage_duration,
                   'total_outage_duration_minutes': total_outage_duration / 60,
                   'total_outage_duration_hours': total_outage_duration / 3600,
                   'uptime_seconds': uptime_seconds,
                   'uptime_hours': uptime_seconds / 3600,
                   'outage_percentage': outage_percentage,
                   'availability_percentage': availability_percentage,
                   'current_outage_duration_seconds': current_outage_duration,
                   'current_outage_ongoing': outage_start_time is not None
               }})
    
    if current_outage_duration > 0:
        logger.warning(f"Current outage ongoing for {current_outage_duration/60:.1f} minutes",
                      extra={'extra_data': {
                          'current_outage_duration_seconds': current_outage_duration,
                          'current_outage_duration_minutes': current_outage_duration / 60
                      }})
    """
    Parse command line arguments with environment variable fallbacks
    """
    # Environment variable defaults
    env_host = os.environ.get('MODEM_HOST', '192.168.100.1')
    env_username = os.environ.get('MODEM_USERNAME', 'admin')
    env_password = os.environ.get('MODEM_PASSWORD', 'motorola')
    env_noverify = os.environ.get('MODEM_NOVERIFY', '').lower() in ('true', '1', 'yes')
    env_check_interval = int(os.environ.get('CHECK_INTERVAL', '60'))
    env_failure_threshold = int(os.environ.get('FAILURE_THRESHOLD', '5'))
    env_recovery_wait = int(os.environ.get('RECOVERY_WAIT', '600'))
    env_log_level = os.environ.get('LOG_LEVEL', 'INFO')
    env_log_file = os.environ.get('LOG_FILE', None)
    env_enable_diagnostics = os.environ.get('ENABLE_DIAGNOSTICS', 'true').lower() in ('true', '1', 'yes')
    env_diagnostics_timeout = int(os.environ.get('DIAGNOSTICS_TIMEOUT', '120'))
    env_outage_report_interval = int(os.environ.get('OUTAGE_REPORT_INTERVAL', '3600'))  # 1 hour default
    
    parser = argparse.ArgumentParser(
        description="Monitor internet connectivity and reboot modem if connection fails"
    )
    parser.add_argument(
        '--host', 
        default=env_host, 
        help=f'Hostname or IP of your modem (Default: {env_host})'
    )
    parser.add_argument(
        '--username', 
        '-u', 
        default=env_username,
        help=f'Admin username (Default: {env_username})'
    )
    parser.add_argument(
        '--password', 
        default=env_password, 
        help='Admin password (Default from env var or motorola)'
    )
    parser.add_argument(
        '--noverify', 
        '-n', 
        action='store_true', 
        default=env_noverify,
        help="Disable SSL certificate verification"
    )
    parser.add_argument(
        '--check-interval', 
        type=int, 
        default=env_check_interval, 
        help=f'Seconds between connectivity checks (Default: {env_check_interval})'
    )
    parser.add_argument(
        '--failure-threshold', 
        type=int, 
        default=env_failure_threshold, 
        help=f'Number of consecutive failures before reboot (Default: {env_failure_threshold})'
    )
    parser.add_argument(
        '--recovery-wait', 
        type=int, 
        default=env_recovery_wait, 
        help=f'Seconds to wait after reboot before resuming monitoring (Default: {env_recovery_wait})'
    )
    parser.add_argument(
        '--log-level',
        default=env_log_level,
        choices=['DEBUG', 'INFO', 'WARNING', 'ERROR', 'CRITICAL'],
        help=f'Logging level (Default: {env_log_level})'
    )
    parser.add_argument(
        '--log-file',
        default=env_log_file,
        help='Path to log file (Default: stdout only)'
    )
    parser.add_argument(
        '--log-max-size',
        type=int,
        default=10*1024*1024,
        help='Maximum log file size in bytes before rotation (Default: 10MB)'
    )
    parser.add_argument(
        '--log-backup-count',
        type=int,
        default=5,
        help='Number of backup log files to keep (Default: 5)'
    )
    parser.add_argument(
        '--enable-diagnostics',
        action='store_true',
        default=env_enable_diagnostics,
        help='Enable TCP/IP model diagnostics before reboot (Default: enabled)'
    )
    parser.add_argument(
        '--disable-diagnostics',
        action='store_true',
        help='Disable TCP/IP model diagnostics (revert to simple reboot)'
    )
    parser.add_argument(
        '--diagnostics-timeout',
        type=int,
        default=env_diagnostics_timeout,
        help=f'Timeout for network diagnostics in seconds (Default: {env_diagnostics_timeout})'
    )
    parser.add_argument(
        '--outage-report-interval',
        type=int,
        default=env_outage_report_interval,
        help=f'Interval for periodic outage reports in seconds (Default: {env_outage_report_interval})'
    )
    return parser.parse_args()

def main():
    """
    Main monitoring loop
    """
    args = get_arguments()
    
    # Setup logging with improved configuration
    global logger
    logger = setup_logging(
        log_level=args.log_level,
        log_file=args.log_file,
        log_max_size=args.log_max_size,
        log_backup_count=args.log_backup_count
    )
    
    # Configuration
    CHECK_INTERVAL = args.check_interval
    FAILURE_THRESHOLD = args.failure_threshold
    RECOVERY_WAIT = args.recovery_wait
    MODEM_HOST = args.host
    MODEM_USERNAME = args.username
    MODEM_PASSWORD = args.password
    NOVERIFY = args.noverify
    ENABLE_DIAGNOSTICS = args.enable_diagnostics and not args.disable_diagnostics
    DIAGNOSTICS_TIMEOUT = args.diagnostics_timeout
    OUTAGE_REPORT_INTERVAL = args.outage_report_interval
    
    # Hosts to check for connectivity
    PING_HOSTS = ["1.1.1.1", "8.8.8.8", "9.9.9.9"]  # Cloudflare, Google, Quad9 DNS
    HTTP_HOSTS = ["https://www.google.com", "https://www.cloudflare.com", "https://www.amazon.com"]
    
    failure_count = 0
    start_time = datetime.now()
    outage_start_time = None  # Track when internet went down
    total_outage_duration = 0  # Track cumulative outage time
    last_report_time = time.time()  # Track when we last generated a report
    report_interval = OUTAGE_REPORT_INTERVAL  # Use configured report interval
    
    logger.info("Internet connectivity monitoring started", 
               extra={'extra_data': {
                   'start_time': start_time.isoformat(),
                   'check_interval': CHECK_INTERVAL,
                   'failure_threshold': FAILURE_THRESHOLD,
                   'recovery_wait': RECOVERY_WAIT,
                   'modem_host': MODEM_HOST,
                   'modem_username': MODEM_USERNAME,
                   'ssl_verify': not NOVERIFY,
                   'ping_hosts': PING_HOSTS,
                   'http_hosts': HTTP_HOSTS,
                   'log_level': args.log_level,
                   'log_file': args.log_file,
                   'enable_diagnostics': ENABLE_DIAGNOSTICS,
                   'diagnostics_timeout': DIAGNOSTICS_TIMEOUT,
                   'outage_report_interval': OUTAGE_REPORT_INTERVAL
               }})
    
    logger.info(f"Check interval: {CHECK_INTERVAL} seconds")
    logger.info(f"Failure threshold: {FAILURE_THRESHOLD} consecutive failures")
    logger.info(f"Recovery wait time: {RECOVERY_WAIT} seconds")
    logger.info(f"Log level: {args.log_level}")
    if args.log_file:
        logger.info(f"Logging to file: {args.log_file}")
    logger.info(f"TCP/IP diagnostics: {'enabled' if ENABLE_DIAGNOSTICS else 'disabled'}")
    if ENABLE_DIAGNOSTICS:
        logger.info(f"Diagnostics timeout: {DIAGNOSTICS_TIMEOUT} seconds")
    logger.info(f"Outage report interval: {OUTAGE_REPORT_INTERVAL} seconds ({OUTAGE_REPORT_INTERVAL/3600:.1f} hours)")
    
    while True:
        try:
            check_start_time = time.time()
            
            # Check internet connectivity
            if check_internet(PING_HOSTS, HTTP_HOSTS):
                check_duration = time.time() - check_start_time
                
                # Internet is back up - calculate outage duration if there was one
                if outage_start_time is not None:
                    outage_duration = time.time() - outage_start_time
                    total_outage_duration += outage_duration
                    
                    # Log the outage duration as a warning
                    logger.warning(f"Internet outage resolved after {outage_duration:.1f} seconds "
                                  f"({outage_duration/60:.1f} minutes)",
                                  extra={'extra_data': {
                                      'outage_duration_seconds': outage_duration,
                                      'outage_duration_minutes': outage_duration / 60,
                                      'outage_duration_hours': outage_duration / 3600,
                                      'outage_start_time': datetime.fromtimestamp(outage_start_time).isoformat(),
                                      'outage_end_time': datetime.now().isoformat(),
                                      'failure_count_during_outage': failure_count,
                                      'total_outage_duration_today': total_outage_duration,
                                      'outage_resolved': True
                                  }})
                    
                    # Reset outage tracking
                    outage_start_time = None
                
                logger.info(f"Internet connection is UP (checked in {check_duration:.2f}s)",
                           extra={'extra_data': {
                               'connection_status': 'UP',
                               'check_duration': check_duration,
                               'failure_count': failure_count,
                               'was_in_outage': outage_start_time is not None
                           }})
                failure_count = 0
            else:
                check_duration = time.time() - check_start_time
                
                # Internet is down - start tracking outage if this is the first failure
                if outage_start_time is None:
                    outage_start_time = time.time()
                    logger.warning(f"Internet outage started at {datetime.now().strftime('%Y-%m-%d %H:%M:%S')}",
                                  extra={'extra_data': {
                                      'outage_started': True,
                                      'outage_start_time': datetime.now().isoformat(),
                                      'failure_count': 1
                                  }})
                else:
                    # Calculate current outage duration
                    current_outage_duration = time.time() - outage_start_time
                    logger.debug(f"Internet outage continues - duration: {current_outage_duration:.1f}s "
                                f"({current_outage_duration/60:.1f} minutes)",
                                extra={'extra_data': {
                                    'outage_ongoing': True,
                                    'current_outage_duration_seconds': current_outage_duration,
                                    'current_outage_duration_minutes': current_outage_duration / 60,
                                    'failure_count': failure_count + 1
                                }})
                
                failure_count += 1
                logger.warning(f"Internet connection is DOWN (Failure count: {failure_count}/{FAILURE_THRESHOLD}) "
                              f"(checked in {check_duration:.2f}s)",
                              extra={'extra_data': {
                                  'connection_status': 'DOWN',
                                  'check_duration': check_duration,
                                  'failure_count': failure_count,
                                  'failure_threshold': FAILURE_THRESHOLD,
                                  'outage_start_time': datetime.fromtimestamp(outage_start_time).isoformat() if outage_start_time else None,
                                  'current_outage_duration_seconds': time.time() - outage_start_time if outage_start_time else 0
                              }})
                
                # If we've reached the failure threshold, decide on action based on diagnostics setting
                if failure_count >= FAILURE_THRESHOLD:
                    if ENABLE_DIAGNOSTICS:
                        logger.warning(f"Failure threshold reached ({FAILURE_THRESHOLD}). Running TCP/IP diagnostics...",
                                      extra={'extra_data': {
                                          'action': 'diagnostics_initiated',
                                          'failure_count': failure_count,
                                          'failure_threshold': FAILURE_THRESHOLD
                                      }})
                        
                        # Run comprehensive network diagnostics with timeout
                        try:
                            import signal
                            
                            def timeout_handler(signum, frame):
                                raise TimeoutError("Network diagnostics timed out")
                            
                            signal.signal(signal.SIGALRM, timeout_handler)
                            signal.alarm(DIAGNOSTICS_TIMEOUT)
                            
                            should_reboot, diag_results, failure_analysis = run_network_diagnostics(MODEM_HOST)
                            
                            signal.alarm(0)  # Cancel the alarm
                            
                        except TimeoutError:
                            logger.warning(f"Network diagnostics timed out after {DIAGNOSTICS_TIMEOUT}s - proceeding with reboot",
                                          extra={'extra_data': {
                                              'action': 'diagnostics_timeout',
                                              'timeout': DIAGNOSTICS_TIMEOUT
                                          }})
                            should_reboot = True
                            diag_results = []
                            failure_analysis = {'error': 'diagnostics_timeout'}
                        except Exception as e:
                            logger.error(f"Network diagnostics failed: {e} - proceeding with reboot",
                                        extra={'extra_data': {
                                            'action': 'diagnostics_error',
                                            'error': str(e)
                                        }})
                            should_reboot = True
                            diag_results = []
                            failure_analysis = {'error': str(e)}
                        
                        if should_reboot:
                            logger.warning("Network diagnostics recommend modem reboot",
                                          extra={'extra_data': {
                                              'action': 'reboot_recommended',
                                              'diagnostic_results_count': len(diag_results),
                                              'failure_analysis': failure_analysis
                                          }})
                            
                            if reboot_modem(MODEM_HOST, MODEM_USERNAME, MODEM_PASSWORD, NOVERIFY):
                                # Calculate outage duration at time of reboot
                                outage_duration_at_reboot = time.time() - outage_start_time if outage_start_time else 0
                                
                                logger.info(f"Modem rebooted successfully. Waiting {RECOVERY_WAIT} seconds for recovery...",
                                           extra={'extra_data': {
                                               'action': 'reboot_successful',
                                               'recovery_wait': RECOVERY_WAIT,
                                               'outage_duration_at_reboot_seconds': outage_duration_at_reboot,
                                               'outage_duration_at_reboot_minutes': outage_duration_at_reboot / 60
                                           }})
                                
                                if outage_duration_at_reboot > 0:
                                    logger.warning(f"Reboot initiated after {outage_duration_at_reboot:.1f} seconds "
                                                  f"({outage_duration_at_reboot/60:.1f} minutes) of internet outage",
                                                  extra={'extra_data': {
                                                      'reboot_trigger_outage_duration_seconds': outage_duration_at_reboot,
                                                      'reboot_trigger_outage_duration_minutes': outage_duration_at_reboot / 60,
                                                      'failure_count_at_reboot': failure_count
                                                  }})
                                
                                # Reset failure count after reboot
                                failure_count = 0
                                # Wait for recovery period before checking again
                                time.sleep(RECOVERY_WAIT)
                            else:
                                logger.error("Modem reboot failed",
                                            extra={'extra_data': {
                                                'action': 'reboot_failed',
                                                'failure_count': failure_count
                                            }})
                        else:
                            logger.info("Network diagnostics suggest reboot may not resolve issues",
                                       extra={'extra_data': {
                                           'action': 'reboot_skipped',
                                           'diagnostic_results_count': len(diag_results),
                                           'failure_analysis': failure_analysis,
                                           'recommended_actions': failure_analysis.get('recommended_actions', [])
                                       }})
                            
                            # Reset failure count to avoid continuous diagnostic runs
                            # but set it to threshold-1 to trigger diagnostics again if issues persist
                            failure_count = FAILURE_THRESHOLD - 1
                            
                            # Wait a bit longer before next check when diagnostics suggest no reboot
                            logger.info(f"Waiting {RECOVERY_WAIT // 2} seconds before next check (diagnostics mode)...")
                            time.sleep(RECOVERY_WAIT // 2)
                    else:
                        # Simple reboot without diagnostics (original behavior)
                        logger.warning(f"Failure threshold reached ({FAILURE_THRESHOLD}). Rebooting modem (diagnostics disabled)...",
                                      extra={'extra_data': {
                                          'action': 'reboot_initiated',
                                          'failure_count': failure_count,
                                          'failure_threshold': FAILURE_THRESHOLD,
                                          'diagnostics_enabled': False
                                      }})
                        
                        if reboot_modem(MODEM_HOST, MODEM_USERNAME, MODEM_PASSWORD, NOVERIFY):
                            # Calculate outage duration at time of reboot
                            outage_duration_at_reboot = time.time() - outage_start_time if outage_start_time else 0
                            
                            logger.info(f"Modem rebooted successfully. Waiting {RECOVERY_WAIT} seconds for recovery...",
                                       extra={'extra_data': {
                                           'action': 'reboot_successful',
                                           'recovery_wait': RECOVERY_WAIT,
                                           'outage_duration_at_reboot_seconds': outage_duration_at_reboot,
                                           'outage_duration_at_reboot_minutes': outage_duration_at_reboot / 60
                                       }})
                            
                            if outage_duration_at_reboot > 0:
                                logger.warning(f"Reboot initiated after {outage_duration_at_reboot:.1f} seconds "
                                              f"({outage_duration_at_reboot/60:.1f} minutes) of internet outage",
                                              extra={'extra_data': {
                                                  'reboot_trigger_outage_duration_seconds': outage_duration_at_reboot,
                                                  'reboot_trigger_outage_duration_minutes': outage_duration_at_reboot / 60,
                                                  'failure_count_at_reboot': failure_count
                                              }})
                            
                            # Reset failure count after reboot
                            failure_count = 0
                            # Wait for recovery period before checking again
                            time.sleep(RECOVERY_WAIT)
                        else:
                            logger.error("Modem reboot failed",
                                        extra={'extra_data': {
                                            'action': 'reboot_failed',
                                            'failure_count': failure_count
                                        }})
            
            # Wait for next check
            logger.debug(f"Waiting {CHECK_INTERVAL} seconds until next check...")
            time.sleep(CHECK_INTERVAL)
            
            # Generate periodic outage report
            current_time = time.time()
            if current_time - last_report_time >= report_interval:
                uptime_seconds = (datetime.now() - start_time).total_seconds()
                log_periodic_outage_report(total_outage_duration, uptime_seconds, outage_start_time)
                last_report_time = current_time
            
        except KeyboardInterrupt:
            uptime = datetime.now() - start_time
            
            # Log final outage duration if internet was down when stopped
            final_outage_duration = 0
            if outage_start_time is not None:
                final_outage_duration = time.time() - outage_start_time
                total_outage_duration += final_outage_duration
                
                logger.warning(f"Monitoring stopped during internet outage - final outage duration: "
                              f"{final_outage_duration:.1f} seconds ({final_outage_duration/60:.1f} minutes)",
                              extra={'extra_data': {
                                  'final_outage_duration_seconds': final_outage_duration,
                                  'final_outage_duration_minutes': final_outage_duration / 60,
                                  'outage_ongoing_at_shutdown': True
                              }})
            
            # Log total outage statistics
            outage_percentage = (total_outage_duration / uptime.total_seconds() * 100) if uptime.total_seconds() > 0 else 0
            
            logger.info(f"Monitoring stopped by user after {uptime}",
                       extra={'extra_data': {
                           'action': 'shutdown',
                           'uptime_seconds': uptime.total_seconds(),
                           'reason': 'keyboard_interrupt',
                           'total_outage_duration_seconds': total_outage_duration,
                           'total_outage_duration_minutes': total_outage_duration / 60,
                           'total_outage_duration_hours': total_outage_duration / 3600,
                           'outage_percentage': outage_percentage,
                           'final_outage_ongoing': outage_start_time is not None
                       }})
            
            if total_outage_duration > 0:
                logger.warning(f"Total internet outage time during monitoring: "
                              f"{total_outage_duration:.1f} seconds ({total_outage_duration/60:.1f} minutes, "
                              f"{total_outage_duration/3600:.2f} hours) - {outage_percentage:.1f}% of uptime",
                              extra={'extra_data': {
                                  'total_outage_summary': True,
                                  'total_outage_duration_seconds': total_outage_duration,
                                  'total_outage_duration_minutes': total_outage_duration / 60,
                                  'total_outage_duration_hours': total_outage_duration / 3600,
                                  'uptime_seconds': uptime.total_seconds(),
                                  'outage_percentage': outage_percentage
                              }})
            
            break
        except Exception as e:
            logger.error(f"Error in monitoring loop: {e}",
                        extra={'extra_data': {
                            'error': str(e),
                            'error_type': type(e).__name__,
                            'failure_count': failure_count
                        }}, exc_info=True)
            # Wait a bit before trying again
            logger.info("Waiting 60 seconds before retrying after error...")
            time.sleep(60)

if __name__ == "__main__":
    main()
