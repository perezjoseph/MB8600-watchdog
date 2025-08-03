#!/usr/bin/env python3
# Script to reboot Motorola/Arris Surfboard modems via the HNAP API
# Enhanced version with improved logging capabilities
# Supports models like MB8600, MB8611 and others with similar interfaces
import hmac
import time
import argparse
import requests
import json
import hashlib
import logging
import logging.handlers
import socket
import sys
import os
from datetime import datetime
from pathlib import Path
from requests.adapters import HTTPAdapter
from urllib3.util.retry import Retry

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

def is_host_reachable(host, port=443, timeout=3):
    """
    Check if a host is reachable by attempting a socket connection.
    
    Args:
        host: Hostname or IP address to check
        port: Port to connect to (default 443 for HTTPS)
        timeout: Connection timeout in seconds
        
    Returns:
        Boolean indicating if connection succeeded
    """
    try:
        logger.debug(f"Checking if {host}:{port} is reachable (timeout: {timeout}s)")
        start_time = time.time()
        socket.create_connection((host, port), timeout=timeout)
        duration = time.time() - start_time
        logger.debug(f"Host {host}:{port} is reachable (connected in {duration:.2f}s)",
                    extra={'extra_data': {
                        'host': host, 
                        'port': port, 
                        'duration': duration, 
                        'reachable': True
                    }})
        return True
    except OSError as e:
        duration = time.time() - start_time
        logger.debug(f"Host {host}:{port} is not reachable after {duration:.2f}s: {e}",
                    extra={'extra_data': {
                        'host': host, 
                        'port': port, 
                        'duration': duration, 
                        'reachable': False,
                        'error': str(e)
                    }})
        return False

def wait_for_reboot_cycle(host, verify=False, poll_seconds=5, max_time_seconds=480):
    """
    Waits for the modem to (a) drop off the network at least once,
    then (b) come back online.  Returns True on success, False otherwise.
    """
    logger.info(f"Waiting for reboot cycle (up to {max_time_seconds}s, polling every {poll_seconds}s)...",
               extra={'extra_data': {
                   'host': host,
                   'max_time_seconds': max_time_seconds,
                   'poll_seconds': poll_seconds,
                   'ssl_verify': verify
               }})
    
    drop_detected = False
    t_start = time.time()
    poll_count = 0
    
    while time.time() - t_start < max_time_seconds:
        poll_count += 1
        try:
            # we hit a very lightweight endpoint; timeout aggressively
            poll_start = time.time()
            response = requests.head(f"https://{host}/HNAP1/", timeout=3, verify=verify)
            poll_duration = time.time() - poll_start
            reachable = True
            logger.debug(f"Poll #{poll_count}: Modem responded (status: {response.status_code}) in {poll_duration:.2f}s",
                        extra={'extra_data': {
                            'poll_number': poll_count,
                            'poll_duration': poll_duration,
                            'status_code': response.status_code,
                            'reachable': True,
                            'drop_detected': drop_detected
                        }})
        except requests.exceptions.RequestException as e:
            poll_duration = time.time() - poll_start
            reachable = False
            logger.debug(f"Poll #{poll_count}: Modem not responding after {poll_duration:.2f}s: {e}",
                        extra={'extra_data': {
                            'poll_number': poll_count,
                            'poll_duration': poll_duration,
                            'reachable': False,
                            'drop_detected': drop_detected,
                            'error': str(e)
                        }})

        if not drop_detected:
            if not reachable:
                drop_detected = True
                elapsed = time.time() - t_start
                logger.info(f"Modem appears offline after {elapsed:.1f}s – good sign.",
                           extra={'extra_data': {
                               'drop_detected': True,
                               'drop_time': elapsed,
                               'poll_number': poll_count
                           }})
        else:
            if reachable:
                total_elapsed = time.time() - t_start
                logger.info(f"Modem is back online after {total_elapsed:.1f}s total.",
                           extra={'extra_data': {
                               'reboot_completed': True,
                               'total_duration': total_elapsed,
                               'poll_number': poll_count
                           }})
                return True
        
        time.sleep(poll_seconds)

    total_elapsed = time.time() - t_start
    logger.error(f"Modem did not complete a reboot cycle within expected time ({total_elapsed:.1f}s).",
                extra={'extra_data': {
                    'reboot_completed': False,
                    'total_duration': total_elapsed,
                    'max_time_seconds': max_time_seconds,
                    'drop_detected': drop_detected,
                    'total_polls': poll_count
                }})
    return False

class SurfboardHNAP:
    """
    Client for Surfboard/Motorola modem HNAP API interface.
    
    This class handles authentication and API calls for Motorola/Arris modems
    that implement the HNAP (Home Network Administration Protocol) API.
    """

    def __init__(self, username: str = 'admin'):
        """
        Initialize the HNAP client.
        
        Args:
            username: Admin username to use for authentication (default: admin)
        """
        logger.debug(f"Initializing SurfboardHNAP client for user: {username}")
        
        # Create session with retry capabilities
        self.s = requests.Session()
        retry_strategy = Retry(
            total=3,
            backoff_factor=1,
            status_forcelist=[429, 500, 502, 503, 504],
            allowed_methods=["HEAD", "GET", "OPTIONS", "POST"]
        )
        adapter = HTTPAdapter(max_retries=retry_strategy)
        self.s.mount("http://", adapter)
        self.s.mount("https://", adapter)
        
        # Authentication state
        self.privatekey = None
        self.cookie_id = None
        self.host = None
        self.username = username
        
        logger.debug("SurfboardHNAP client initialized successfully")

    def login_html_form(self, password: str) -> bool:
        """
        Perform HTML form login to establish session for HNAP JSON API.

        Some MB8600 firmware builds take a moment to bring the HTTPS
        management interface online after a reboot. We therefore:
        1. Retry the initial GET /Login.html for up to 90 s.
        2. Fall back to plain‑HTTP on port 80 if HTTPS is unreachable.
        3. Retry the credential POST the same way.
        """
        logger.debug("Starting HTML form login process")

        def _attempt_fetch_login_page(scheme):
            url = f'{scheme}://{self.host}/Login.html'
            try:
                logger.debug(f"Attempting to fetch login page via {scheme.upper()}: {url}")
                start_time = time.time()
                response = self.s.get(url, timeout=5)
                duration = time.time() - start_time
                logger.debug(f"{scheme.upper()} fetch successful in {duration:.2f}s (status: {response.status_code})",
                            extra={'extra_data': {
                                'scheme': scheme,
                                'url': url,
                                'duration': duration,
                                'status_code': response.status_code
                            }})
                return response
            except requests.exceptions.RequestException as e:
                duration = time.time() - start_time
                logger.debug(f"{scheme.upper()} fetch to /Login.html failed after {duration:.2f}s: {e}",
                            extra={'extra_data': {
                                'scheme': scheme,
                                'url': url,
                                'duration': duration,
                                'error': str(e)
                            }})
                return None

        # --- Step 1: fetch the login page with retries ---
        fetch_ok = None
        deadline = time.time() + 90      # 90‑second window
        attempt_count = 0
        
        logger.info("Attempting to fetch login page (up to 90s)...")
        while time.time() < deadline:
            attempt_count += 1
            logger.debug(f"Login page fetch attempt #{attempt_count}")
            
            for scheme in ("https", "http"):
                resp = _attempt_fetch_login_page(scheme)
                if resp is not None and resp.status_code == 200:
                    fetch_ok = (scheme, resp)
                    logger.info(f"Successfully fetched login page via {scheme.upper()} on attempt #{attempt_count}")
                    break
            if fetch_ok:
                break
            time.sleep(2)

        if not fetch_ok:
            elapsed = time.time() - (deadline - 90)
            logger.error(f"Unable to reach /Login.html after {elapsed:.1f}s and {attempt_count} attempts.",
                        extra={'extra_data': {
                            'elapsed_time': elapsed,
                            'attempt_count': attempt_count,
                            'success': False
                        }})
            return False

        scheme, rp = fetch_ok
        login_url = f'{scheme}://{self.host}/cgi-bin/moto/goform/MotoLogin'
        logger.debug(f"Using login URL: {login_url}")

        # --- Step 2: submit credentials (retry loop) ---
        data = {
            'loginUsername': self.username,
            'loginPassword': password
        }
        headers = {
            'Content-Type': 'application/x-www-form-urlencoded',
            'Origin': f'{scheme}://{self.host}',
            'Referer': f'{scheme}://{self.host}/Login.html'
        }

        logger.info("Submitting login credentials...")
        deadline = time.time() + 30      # 30‑second window for POST retry
        post_attempt = 0
        
        while time.time() < deadline:
            post_attempt += 1
            try:
                logger.debug(f"Credential POST attempt #{post_attempt}")
                start_time = time.time()
                r = self.s.post(login_url, data=data, headers=headers, timeout=5)
                duration = time.time() - start_time
                
                logger.info(f"HTML form login status: {r.status_code} (attempt #{post_attempt}, {duration:.2f}s)",
                           extra={'extra_data': {
                               'status_code': r.status_code,
                               'attempt': post_attempt,
                               'duration': duration,
                               'scheme': scheme
                           }})
                
                if r.status_code == 200:
                    cookies = r.cookies.get_dict()
                    logger.info(f"Session cookies after HTML login: {list(cookies.keys())}",
                               extra={'extra_data': {'cookies': cookies}})
                    return True
                    
            except requests.exceptions.RequestException as e:
                duration = time.time() - start_time
                logger.warning(f"Credential POST attempt #{post_attempt} failed after {duration:.2f}s: {e}",
                              extra={'extra_data': {
                                  'attempt': post_attempt,
                                  'duration': duration,
                                  'error': str(e)
                              }})
            time.sleep(2)

        logger.error(f"HTML form login ultimately failed after {post_attempt} attempts.",
                    extra={'extra_data': {'total_attempts': post_attempt, 'success': False}})
        return False

    def generate_keys(self, challenge: bytes, pubkey: bytes, password: bytes):
        """
        Generate authentication keys using HNAP challenge-response protocol.
        
        Args:
            challenge: Challenge string from modem
            pubkey: Public key from modem
            password: User's password
            
        Returns:
            Tuple of (private_key, password_key)
        """
        logger.debug("Generating HNAP authentication keys")
        start_time = time.time()
        
        privatekey = hmac.new(pubkey+password, challenge, hashlib.md5).hexdigest().upper()
        passkey = hmac.new(privatekey.encode(), challenge, hashlib.md5).hexdigest().upper()
        self.privatekey = privatekey
        
        duration = time.time() - start_time
        logger.debug(f"Authentication keys generated in {duration:.4f}s",
                    extra={'extra_data': {
                        'duration': duration,
                        'challenge_length': len(challenge),
                        'pubkey_length': len(pubkey)
                    }})
        
        return (privatekey, passkey)

    def generate_hnap_auth(self, operation):
        """
        Generate HNAP authentication header for API operations.
        
        Args:
            operation: The HNAP operation name
            
        Returns:
            Authentication string for HNAP_AUTH header
        """
        privkey = self.privatekey
        curtime = str(int(time.time() * 1000))
        auth_key = curtime + '"http://purenetworks.com/HNAP1/{}"'.format(operation)
        privkey = privkey.encode()
        auth = hmac.new(privkey, auth_key.encode(), hashlib.md5)
        auth_header = auth.hexdigest().upper() + ' ' + curtime
        
        logger.debug(f"Generated HNAP auth header for operation: {operation}",
                    extra={'extra_data': {
                        'operation': operation,
                        'timestamp': curtime
                    }})
        
        return auth_header
    def _login_request(self, host):
        """
        Initial login request to get challenge and public key.
        
        Args:
            host: Modem hostname or IP
            
        Returns:
            Response from HNAP login request
        """
        url = f'https://{host}/HNAP1/'
        headers = {
            'Content-Type': 'application/json; charset=UTF-8',
            'SOAPACTION': '"http://purenetworks.com/HNAP1/Login"',
            'Accept': 'application/json',
            'Origin': f'https://{host}',
            'Referer': f'https://{host}/MotoStatusSecurity.html',
            'User-Agent': 'Mozilla/5.0'
        }
        payload = {
            "Login": {
                "Action": "request",
                "Username": self.username,
                "LoginPassword": "",
                "Captcha": "",
                "PrivateLogin": "LoginPassword"
            }
        }
        
        logger.debug(f"Sending initial HNAP login request to {url}")
        start_time = time.time()
        
        try:
            r = self.s.post(url, headers=headers, json=payload)
            duration = time.time() - start_time
            
            logger.debug(f"Initial login request completed in {duration:.2f}s (status: {r.status_code})",
                        extra={'extra_data': {
                            'url': url,
                            'duration': duration,
                            'status_code': r.status_code,
                            'response_length': len(r.text)
                        }})
            return r
            
        except Exception as e:
            duration = time.time() - start_time
            logger.error(f"Initial login request failed after {duration:.2f}s: {e}",
                        extra={'extra_data': {
                            'url': url,
                            'duration': duration,
                            'error': str(e),
                            'error_type': type(e).__name__
                        }})
            raise

    def _login_real(self, host, privatekey, passkey):
        """
        Second phase of login with challenge response.
        
        Args:
            host: Modem hostname or IP
            privatekey: Generated private key
            passkey: Generated password key
            
        Returns:
            Response from HNAP login completion
        """
        url = f'https://{host}/HNAP1/'
        auth = self.generate_hnap_auth('Login')
        headers = {
            'HNAP_AUTH': auth,
            'Content-Type': 'application/json; charset=UTF-8',
            'SOAPACTION': '"http://purenetworks.com/HNAP1/Login"',
            'Accept': 'application/json'
        }
        payload = {
            "Login": {
                "Action": "login",
                "Username": self.username,
                "LoginPassword": passkey,
                "Captcha": "",
                "PrivateLogin": "LoginPassword"
            }
        }
        
        logger.debug(f"Sending HNAP login completion request to {url}")
        start_time = time.time()
        
        try:
            r = self.s.post(url, headers=headers, json=payload)
            duration = time.time() - start_time
            
            logger.debug(f"Login completion request finished in {duration:.2f}s (status: {r.status_code})",
                        extra={'extra_data': {
                            'url': url,
                            'duration': duration,
                            'status_code': r.status_code,
                            'response_length': len(r.text)
                        }})
            return r
            
        except Exception as e:
            duration = time.time() - start_time
            logger.error(f"Login completion request failed after {duration:.2f}s: {e}",
                        extra={'extra_data': {
                            'url': url,
                            'duration': duration,
                            'error': str(e),
                            'error_type': type(e).__name__
                        }})
            raise

    def login(self, host, password, noverify):
        """
        Complete login process including HTML form and HNAP JSON API.
        
        Args:
            host: Modem hostname or IP address
            password: Admin password
            noverify: If True, disable SSL verification
            
        Returns:
            Response object if login successful, None if failed
        """
        logger.info(f"Starting complete login process for {host}",
                   extra={'extra_data': {
                       'host': host,
                       'username': self.username,
                       'ssl_verify': not noverify
                   }})
        
        login_start_time = time.time()
        self.host = host
        
        if noverify:
            self.s.verify = False
            logger.debug("SSL certificate verification disabled")
            
        if not is_host_reachable(host, port=443):
            logger.error(f"Host {host} is unreachable.")
            return None
            
        # Step 1: HTML form login for session cookies
        logger.info("Step 1: Performing HTML form login...")
        html_start = time.time()
        if not self.login_html_form(password):
            html_duration = time.time() - html_start
            logger.error(f"HTML form login failed after {html_duration:.2f}s")
            return None
        html_duration = time.time() - html_start
        logger.info(f"HTML form login successful in {html_duration:.2f}s")
            
        # Step 2: First HNAP login request to get challenge
        logger.info("Step 2: Getting HNAP challenge...")
        hnap1_start = time.time()
        try:
            r = self._login_request(host)
            if r.status_code != 200:
                logger.error(f"Login failed with status code {r.status_code}")
                return None
        except Exception as e:
            logger.error(f"HNAP login request failed: {e}")
            return None
        hnap1_duration = time.time() - hnap1_start

        # Handle JSON-based HNAP login request
        try:
            data1 = r.json()
            logger.debug("Successfully parsed HNAP challenge response as JSON")
        except ValueError as e:
            logger.error("First login request returned non‑JSON (likely HTML). Snippet:")
            logger.error(r.text[:400])
            return None
            
        # Extract authentication parameters
        lr1 = data1.get("LoginResponse", {})
        cookie_id = lr1.get("Cookie")
        pubkey = lr1.get("PublicKey")
        challenge = lr1.get("Challenge")
        
        if not cookie_id or not pubkey or not challenge:
            logger.error(f"LoginResponse missing required fields: {lr1}",
                        extra={'extra_data': {
                            'has_cookie': bool(cookie_id),
                            'has_pubkey': bool(pubkey),
                            'has_challenge': bool(challenge),
                            'response': lr1
                        }})
            return None

        logger.debug("Successfully extracted authentication parameters",
                    extra={'extra_data': {
                        'cookie_id': cookie_id[:8] + "..." if cookie_id else None,
                        'pubkey_length': len(pubkey) if pubkey else 0,
                        'challenge_length': len(challenge) if challenge else 0
                    }})

        # Step 3: Perform real login with challenge response
        logger.info("Step 3: Completing HNAP authentication...")
        hnap2_start = time.time()
        
        privkey, passkey = self.generate_keys(challenge.encode(), pubkey.encode(), password.encode())
        self.s.cookies.update({'uid': cookie_id, 'PrivateKey': privkey})
        
        try:
            r2 = self._login_real(host, privkey, passkey)
            if r2.status_code != 200:
                logger.error(f"Login (real) failed with status {r2.status_code}")
                return None
        except Exception as e:
            logger.error(f"HNAP login completion failed: {e}")
            return None
            
        hnap2_duration = time.time() - hnap2_start
            
        # Verify login success
        try:
            data2 = r2.json()
            logger.debug("Successfully parsed HNAP login completion response as JSON")
        except ValueError:
            logger.error("Second login request returned non‑JSON. Snippet:")
            logger.error(r2.text[:400])
            return None
            
        login_result = data2.get("LoginResponse", {}).get("LoginResult")
        if login_result != "OK":
            logger.error(f"LoginResult not OK: {data2.get('LoginResponse')}",
                        extra={'extra_data': {'login_response': data2.get('LoginResponse')}})
            return None

        self.cookie_id = cookie_id
        total_login_duration = time.time() - login_start_time
        
        logger.info(f"Complete login process successful in {total_login_duration:.2f}s "
                   f"(HTML: {html_duration:.2f}s, HNAP1: {hnap1_duration:.2f}s, HNAP2: {hnap2_duration:.2f}s)",
                   extra={'extra_data': {
                       'total_duration': total_login_duration,
                       'html_duration': html_duration,
                       'hnap1_duration': hnap1_duration,
                       'hnap2_duration': hnap2_duration,
                       'login_successful': True
                   }})
        
        return r2

    def get_status(self):
        """
        Get modem status information.
        
        Returns:
            Response from status request
        """
        logger.debug("Requesting modem status information")
        host = self.host
        
        url = 'https://{}/HNAP1/'.format(host)
        auth = self.generate_hnap_auth('GetMultipleHNAPs')
        headers = {
            'HNAP_AUTH': auth,
            'SOAPACTION': '"http://purenetworks.com/HNAP1/GetMultipleHNAPs"',
        }

        payload = {'GetMultipleHNAPs': {
            'GetMotoStatusSecAccount': '',
            'GetMotoStatusSecXXX': ''
        }}

        start_time = time.time()
        try:
            r = self.s.post(url, headers=headers, json=payload)
            duration = time.time() - start_time
            
            logger.debug(f"Status request completed in {duration:.2f}s (status: {r.status_code})",
                        extra={'extra_data': {
                            'duration': duration,
                            'status_code': r.status_code,
                            'response_length': len(r.text)
                        }})
            return r
            
        except Exception as e:
            duration = time.time() - start_time
            logger.error(f"Status request failed after {duration:.2f}s: {e}",
                        extra={'extra_data': {
                            'duration': duration,
                            'error': str(e),
                            'error_type': type(e).__name__
                        }})
            raise

    def get_security(self):
        """
        Get modem security settings.
        
        Returns:
            Response from security settings request
        """
        logger.debug("Requesting modem security settings")
        host = self.host

        url = 'https://{}/HNAP1/'.format(host)
        auth = self.generate_hnap_auth('GetMultipleHNAPs')
        headers = {
            'HNAP_AUTH': auth,
            'SOAPACTION': '"http://purenetworks.com/HNAP1/GetMultipleHNAPs"'
        }

        payload = {'GetMultipleHNAPs': {
            'GetMotoStatusSecAccount': '',
            'GetMotoStatusSecXXX': ''
        }}

        start_time = time.time()
        try:
            r = self.s.post(url, headers=headers, json=payload)
            duration = time.time() - start_time
            
            logger.debug(f"Security request completed in {duration:.2f}s (status: {r.status_code})",
                        extra={'extra_data': {
                            'duration': duration,
                            'status_code': r.status_code,
                            'response_length': len(r.text)
                        }})
            return r
            
        except Exception as e:
            duration = time.time() - start_time
            logger.error(f"Security request failed after {duration:.2f}s: {e}",
                        extra={'extra_data': {
                            'duration': duration,
                            'error': str(e),
                            'error_type': type(e).__name__
                        }})
            raise

    def reboot(self):
        """
        Perform a reboot via JSON-based HNAP SetStatusSecuritySettings.
        """
        logger.info("Sending reboot command to modem")
        url = f'https://{self.host}/HNAP1/'
        auth = self.generate_hnap_auth('SetStatusSecuritySettings')
        headers = {
            'HNAP_AUTH': auth,
            'Content-Type': 'application/json; charset=UTF-8',
            'SOAPACTION': '"http://purenetworks.com/HNAP1/SetStatusSecuritySettings"'
        }
        payload = {
            "SetStatusSecuritySettings": {
                "MotoStatusSecurityAction": "1",
                "MotoStatusSecXXX": "XXX"
            }
        }
        
        start_time = time.time()
        try:
            r = self.s.post(url, headers=headers, json=payload)
            duration = time.time() - start_time
            
            logger.info(f"Reboot command completed in {duration:.2f}s (status: {r.status_code})",
                       extra={'extra_data': {
                           'duration': duration,
                           'status_code': r.status_code,
                           'response_length': len(r.text),
                           'reboot_command_sent': True
                       }})
            return r
            
        except Exception as e:
            duration = time.time() - start_time
            logger.error(f"Reboot command failed after {duration:.2f}s: {e}",
                        extra={'extra_data': {
                            'duration': duration,
                            'error': str(e),
                            'error_type': type(e).__name__,
                            'reboot_command_sent': False
                        }})
            raise

def get_arguments():
    """
    Parse command line arguments.
    
    Returns:
        Parsed argument namespace
    """
    # Environment variable defaults
    env_host = os.environ.get('MODEM_HOST', '192.168.100.1')
    env_username = os.environ.get('MODEM_USERNAME', 'admin')
    env_password = os.environ.get('MODEM_PASSWORD', 'motorola')
    env_noverify = os.environ.get('MODEM_NOVERIFY', '').lower() in ('true', '1', 'yes')
    env_log_level = os.environ.get('LOG_LEVEL', 'INFO')
    env_log_file = os.environ.get('LOG_FILE', None)
    
    parser = argparse.ArgumentParser(
        description="Reboot Motorola/Arris Surfboard modems via HNAP API"
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
        '--dryrun', 
        '-d', 
        action='store_true', 
        help="Logs in but doesn't reboot"
    )
    parser.add_argument(
        '--noverify', 
        '-n', 
        action='store_true', 
        default=env_noverify,
        help="Disable SSL certificate verification"
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
    return parser.parse_args()

if __name__ == '__main__':
    # Parse command line arguments
    args = get_arguments()
    
    # Setup logging with improved configuration
    logger = setup_logging(
        log_level=args.log_level,
        log_file=args.log_file,
        log_max_size=args.log_max_size,
        log_backup_count=args.log_backup_count
    )
    
    host = args.host
    password = args.password
    username = args.username

    logger.info("Starting modem reboot process",
               extra={'extra_data': {
                   'host': host,
                   'username': username,
                   'dryrun': args.dryrun,
                   'ssl_verify': not args.noverify,
                   'log_level': args.log_level,
                   'log_file': args.log_file
               }})

    # Initialize client
    h = SurfboardHNAP(username)
    h.host = host
    if args.noverify:
        h.s.verify = False
    
    # Verify modem is reachable
    if not is_host_reachable(host, port=443):
        logger.error(f"Host {host} is unreachable.")
        sys.exit(1)
    
    # Perform login
    logger.info(f"Logging in to {host} as {username}...")
    login_start = time.time()
    
    try:
        r = h.login(host, password, args.noverify)
        login_duration = time.time() - login_start
        
        logger.info(f'Login process completed in {login_duration:.2f}s: {r}')
        if r is None:
            logger.error("HNAP login failed; unable to reboot.")
            sys.exit(1)
    except Exception as e:
        login_duration = time.time() - login_start
        logger.error(f"Login process failed after {login_duration:.2f}s: {e}", exc_info=True)
        sys.exit(1)

    # Optional: quick status check before reboot
    try:
        logger.info("Checking modem status before reboot...")
        status_resp = h.get_status()
        logger.info(f'Pre-reboot status check: {status_resp.status_code}')
        logger.debug(f'Pre-reboot status response: {status_resp.text[:200]}...')
    except Exception as e:
        logger.warning(f"Pre-reboot status check failed: {e}")

    # Handle dry-run mode
    if args.dryrun:
        logger.info("Dry‑run mode: skipping reboot command.")
        logger.info("Dry-run completed successfully - login verified, reboot command would work")
        sys.exit(0)

    # --- Send reboot command ---
    logger.info("Sending reboot command...")
    reboot_start = time.time()
    
    try:
        reboot_resp = h.reboot()
        reboot_duration = time.time() - reboot_start
        
        logger.info(f'Reboot command completed in {reboot_duration:.2f}s: {reboot_resp}')
        if reboot_resp.status_code != 200:
            logger.error(f"Reboot request failed with status {reboot_resp.status_code}: {reboot_resp.text}")
            sys.exit(1)
        logger.info("Reboot command sent successfully.")
    except Exception as e:
        reboot_duration = time.time() - reboot_start
        logger.error(f"Reboot command failed after {reboot_duration:.2f}s: {e}", exc_info=True)
        sys.exit(1)

    # --- Wait for reboot cycle ---
    cycle_start = time.time()
    if not wait_for_reboot_cycle(host, verify=not args.noverify):
        cycle_duration = time.time() - cycle_start
        logger.error(f"Reboot cycle monitoring failed after {cycle_duration:.2f}s")
        sys.exit(1)
    cycle_duration = time.time() - cycle_start

    # Optional post‑reboot sanity check
    logger.info("Performing post-reboot verification check...")
    try:
        post_status = h.get_status()
        logger.info(f"Post-reboot status check: {post_status.status_code}")
        logger.debug(f"Post-reboot status response: {post_status.text[:200]}...")
    except Exception as e:
        logger.warning(f"Post‑reboot status fetch failed: {e}")
    
    total_duration = time.time() - login_start
    logger.info(f"Modem reboot completed successfully in {total_duration:.2f}s "
               f"(login: {login_duration:.2f}s, reboot: {reboot_duration:.2f}s, cycle: {cycle_duration:.2f}s)",
               extra={'extra_data': {
                   'total_duration': total_duration,
                   'login_duration': login_duration,
                   'reboot_duration': reboot_duration,
                   'cycle_duration': cycle_duration,
                   'success': True
               }})
