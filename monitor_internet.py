#!/usr/bin/env python3
# Monitor internet connectivity and reboot modem if connection fails
import time
import subprocess
import logging
import sys
import requests
import argparse
from datetime import datetime

# Import the modem reboot functionality
from modem_reboot import SurfboardHNAP, wait_for_reboot_cycle, is_host_reachable

# Configure logging
logging.basicConfig(
    level=logging.INFO,
    format='%(asctime)s - %(levelname)s - %(message)s',
    handlers=[
        logging.StreamHandler()
    ]
)
logger = logging.getLogger(__name__)

def ping(host):
    """
    Ping a host and return True if it responds, False otherwise
    """
    try:
        # Use -c for count, -W for timeout in seconds
        subprocess.check_output(
            ["ping", "-c", "3", "-W", "5", host],
            stderr=subprocess.STDOUT,
            universal_newlines=True
        )
        return True
    except subprocess.CalledProcessError:
        return False

def http_check(url):
    """
    Make HTTP request to URL and return True if successful, False otherwise
    """
    try:
        response = requests.get(url, timeout=10)
        return response.status_code == 200
    except requests.RequestException:
        return False

def check_internet(ping_hosts, http_hosts):
    """
    Check internet connectivity by pinging hosts and HTTP requests
    Returns True if at least one check succeeds, False otherwise
    """
    # First try ping checks
    for host in ping_hosts:
        logger.info(f"Pinging {host}...")
        if ping(host):
            logger.info(f"Successfully pinged {host}")
            return True

    # Then try HTTP checks
    for url in http_hosts:
        logger.info(f"HTTP check to {url}...")
        if http_check(url):
            logger.info(f"Successfully connected to {url}")
            return True
    
    logger.warning("All internet connectivity checks failed")
    return False

def reboot_modem(host, username, password, noverify):
    """
    Reboot the modem using the modem_reboot functionality
    """
    try:
        logger.info("Attempting to reboot modem...")
        
        # Initialize client
        h = SurfboardHNAP(username)
        h.host = host
        if noverify:
            h.s.verify = False
        
        # Verify modem is reachable
        if not is_host_reachable(host, port=443):
            logger.error(f"Host {host} is unreachable")
            return False
        
        # Perform login
        logger.info(f"Logging in to {host} as {username}...")
        r = h.login(host, password, noverify)
        logger.info(f'login (HNAP): {r}')
        
        if r is None:
            logger.error("HNAP login failed; unable to reboot")
            return False

        # Send reboot command
        logger.info("Sending reboot command...")
        reboot_resp = h.reboot()
        logger.info(f'reboot: {reboot_resp}')
        
        if reboot_resp.status_code != 200:
            logger.error(f"Reboot request failed with status {reboot_resp.status_code}")
            return False
            
        logger.info("Reboot command sent successfully")
        
        # Wait for reboot cycle
        if not wait_for_reboot_cycle(host, verify=not noverify):
            logger.error("Modem did not complete reboot cycle")
            return False
            
        logger.info("Modem reboot completed successfully")
        return True
    except Exception as e:
        logger.error(f"Error during modem reboot: {e}")
        return False

def get_arguments():
    """
    Parse command line arguments with environment variable fallbacks
    """
    import os
    
    # Environment variable defaults
    env_host = os.environ.get('MODEM_HOST', '192.168.100.1')
    env_username = os.environ.get('MODEM_USERNAME', 'admin')
    env_password = os.environ.get('MODEM_PASSWORD', 'motorola')
    env_noverify = os.environ.get('MODEM_NOVERIFY', '').lower() in ('true', '1', 'yes')
    env_check_interval = int(os.environ.get('CHECK_INTERVAL', '60'))
    env_failure_threshold = int(os.environ.get('FAILURE_THRESHOLD', '5'))
    env_recovery_wait = int(os.environ.get('RECOVERY_WAIT', '600'))
    
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
    return parser.parse_args()

def main():
    """
    Main monitoring loop
    """
    args = get_arguments()
    
    # Configuration
    CHECK_INTERVAL = args.check_interval
    FAILURE_THRESHOLD = args.failure_threshold
    RECOVERY_WAIT = args.recovery_wait
    MODEM_HOST = args.host
    MODEM_USERNAME = args.username
    MODEM_PASSWORD = args.password
    NOVERIFY = args.noverify
    
    # Hosts to check for connectivity
    PING_HOSTS = ["1.1.1.1", "8.8.8.8", "9.9.9.9"]  # Cloudflare, Google, Quad9 DNS
    HTTP_HOSTS = ["https://www.google.com", "https://www.cloudflare.com", "https://www.amazon.com"]
    
    failure_count = 0
    
    logger.info("Internet connectivity monitoring started")
    logger.info(f"Check interval: {CHECK_INTERVAL} seconds")
    logger.info(f"Failure threshold: {FAILURE_THRESHOLD} consecutive failures")
    logger.info(f"Recovery wait time: {RECOVERY_WAIT} seconds")
    
    while True:
        try:
            # Check internet connectivity
            if check_internet(PING_HOSTS, HTTP_HOSTS):
                logger.info("Internet connection is UP")
                failure_count = 0
            else:
                failure_count += 1
                logger.warning(f"Internet connection is DOWN (Failure count: {failure_count}/{FAILURE_THRESHOLD})")
                
                # If we've reached the failure threshold, reboot the modem
                if failure_count >= FAILURE_THRESHOLD:
                    logger.warning(f"Failure threshold reached ({FAILURE_THRESHOLD}). Rebooting modem...")
                    
                    if reboot_modem(MODEM_HOST, MODEM_USERNAME, MODEM_PASSWORD, NOVERIFY):
                        logger.info(f"Modem rebooted successfully. Waiting {RECOVERY_WAIT} seconds for recovery...")
                        # Reset failure count after reboot
                        failure_count = 0
                        # Wait for recovery period before checking again
                        time.sleep(RECOVERY_WAIT)
                    else:
                        logger.error("Modem reboot failed")
            
            # Wait for next check
            logger.info(f"Waiting {CHECK_INTERVAL} seconds until next check...")
            time.sleep(CHECK_INTERVAL)
            
        except Exception as e:
            logger.error(f"Error in monitoring loop: {e}")
            # Wait a bit before trying again
            time.sleep(60)

if __name__ == "__main__":
    main()