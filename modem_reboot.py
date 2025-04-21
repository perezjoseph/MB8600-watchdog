#!/usr/bin/env python3
# Script to reboot Motorola/Arris Surfboard modems via the HNAP API
# Supports models like MB8600, MB8611 and others with similar interfaces
import hmac
import time
import argparse
import requests
import json
import hashlib

import socket
import sys
from requests.adapters import HTTPAdapter
from urllib3.util.retry import Retry

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
        socket.create_connection((host, port), timeout=timeout)
        return True
    except OSError:
        return False

def wait_for_reboot_cycle(host, verify=False,
                          poll_seconds=5,
                          max_time_seconds=480):
    """
    Waits for the modem to (a) drop off the network at least once,
    then (b) come back online.  Returns True on success, False otherwise.
    """
    print(f"Waiting for reboot cycle (up to {max_time_seconds}s)...")
    drop_detected = False
    t_start = time.time()
    while time.time() - t_start < max_time_seconds:
        try:
            # we hit a very lightweight endpoint; timeout aggressively
            requests.head(f"https://{host}/HNAP1/",
                          timeout=3, verify=verify)
            reachable = True
        except requests.exceptions.RequestException:
            reachable = False

        if not drop_detected:
            if not reachable:
                drop_detected = True
                print("Modem appears offline – good sign.")
        else:
            if reachable:
                print("Modem is back online.")
                return True
        time.sleep(poll_seconds)

    print("Modem did not complete a reboot cycle within expected time.")
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

    def login_html_form(self, password: str) -> bool:
        """
        Perform HTML form login to establish session for HNAP JSON API.

        Some MB8600 firmware builds take a moment to bring the HTTPS
        management interface online after a reboot. We therefore:
        1. Retry the initial GET /Login.html for up to 90 s.
        2. Fall back to plain‑HTTP on port 80 if HTTPS is unreachable.
        3. Retry the credential POST the same way.
        """

        def _attempt_fetch_login_page(scheme):
            url = f'{scheme}://{self.host}/Login.html'
            try:
                return self.s.get(url, timeout=5)
            except requests.exceptions.RequestException as e:
                print(f"{scheme.upper()} fetch to /Login.html failed: {e}")
                return None

        # --- Step 1: fetch the login page with retries ---
        fetch_ok = None
        deadline = time.time() + 90      # 90‑second window
        while time.time() < deadline:
            for scheme in ("https", "http"):
                resp = _attempt_fetch_login_page(scheme)
                if resp is not None and resp.status_code == 200:
                    fetch_ok = (scheme, resp)
                    break
            if fetch_ok:
                break
            time.sleep(2)

        if not fetch_ok:
            print("Unable to reach /Login.html after 90 s.")
            return False

        scheme, rp = fetch_ok
        login_url = f'{scheme}://{self.host}/cgi-bin/moto/goform/MotoLogin'

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

        deadline = time.time() + 30      # 30‑second window for POST retry
        while time.time() < deadline:
            try:
                r = self.s.post(login_url, data=data, headers=headers, timeout=5)
                print(f"HTML form login status: {r.status_code}")
                if r.status_code == 200:
                    print("Session cookies after HTML login:", r.cookies.get_dict())
                    return True
            except requests.exceptions.RequestException as e:
                print(f"Credential POST failed: {e}")
            time.sleep(2)

        print("HTML form login ultimately failed.")
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
        privatekey = hmac.new(pubkey+password, challenge, hashlib.md5).hexdigest().upper()
        passkey = hmac.new(privatekey.encode(), challenge, hashlib.md5).hexdigest().upper()
        self.privatekey = privatekey
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
        return auth.hexdigest().upper() + ' ' + curtime

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
        r = self.s.post(url, headers=headers, json=payload)
        return r

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
        r = self.s.post(url, headers=headers, json=payload)
        return r

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
        self.host = host
        if noverify:
            self.s.verify = False
        if not is_host_reachable(host, port=443):
            print(f"Host {host} is unreachable.")
            return None
            
        # Step 1: HTML form login for session cookies
        if not self.login_html_form(password):
            return None
            
        # Step 2: First HNAP login request to get challenge
        r = self._login_request(host)
        if r.status_code != 200:
            print(f"Login failed with status code {r.status_code}")
            return None

        # Handle JSON-based HNAP login request
        try:
            data1 = r.json()
        except ValueError:
            print("First login request returned non‑JSON (likely HTML). Snippet:")
            print(r.text[:400])
            return None
            
        # Extract authentication parameters
        lr1 = data1.get("LoginResponse", {})
        cookie_id = lr1.get("Cookie")
        pubkey = lr1.get("PublicKey")
        challenge = lr1.get("Challenge")
        if not cookie_id or not pubkey or not challenge:
            print(f"LoginResponse missing fields: {lr1}")
            return None

        # Step 3: Perform real login with challenge response
        privkey, passkey = self.generate_keys(challenge.encode(), pubkey.encode(), password.encode())
        self.s.cookies.update({'uid': cookie_id, 'PrivateKey': privkey})
        r2 = self._login_real(host, privkey, passkey)
        if r2.status_code != 200:
            print(f"Login (real) failed with status {r2.status_code}")
            return None
            
        # Verify login success
        try:
            data2 = r2.json()
        except ValueError:
            print("Second login request returned non‑JSON. Snippet:")
            print(r2.text[:400])
            return None
        if data2.get("LoginResponse", {}).get("LoginResult") != "OK":
            print(f"LoginResult not OK: {data2.get('LoginResponse')}")
            return None

        self.cookie_id = cookie_id
        return r2

    def get_status(self):
        """
        Get modem status information.
        
        Returns:
            Response from status request
        """
        host = self.host
        privatekey = self.privatekey

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

        r = self.s.post(url, headers=headers, json=payload)
        return r

    def get_security(self):
        """
        Get modem security settings.
        
        Returns:
            Response from security settings request
        """
        host = self.host
        privatekey = self.privatekey

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

        r = self.s.post(url, headers=headers, json=payload)
        return r

    def reboot(self):
        """
        Perform a reboot via JSON-based HNAP SetStatusSecuritySettings.
        """
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
        r = self.s.post(url, headers=headers, json=payload)
        return r


def get_arguments():
    """
    Parse command line arguments.
    
    Returns:
        Parsed argument namespace
    """
    parser = argparse.ArgumentParser(
        description="Reboot Motorola/Arris Surfboard modems via HNAP API"
    )
    parser.add_argument(
        '--host', 
        default='192.168.100.1', 
        help='Hostname or IP of your modem (Default: 192.168.100.1)'
    )
    parser.add_argument(
        '--username', 
        '-u', 
        default='admin',
        help='Admin username (Default: admin)'
    )
    parser.add_argument(
        '--password', 
        default='motorola', 
        help='Admin password (Default: motorola)'
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
        help="Disable SSL certificate verification"
    )
    return parser.parse_args()

if __name__ == '__main__':
    # Parse command line arguments
    args = get_arguments()
    host = args.host
    password = args.password
    username = args.username

    # Initialize client
    h = SurfboardHNAP(username)
    h.host = host
    if args.noverify:
        h.s.verify = False
    
    # Verify modem is reachable
    if not is_host_reachable(host, port=443):
        print(f"Host {host} is unreachable.")
        sys.exit(1)
    
    # Perform login
    print(f"Logging in to {host} as {username}...")
    r = h.login(host, password, args.noverify)
    print(f'login (HNAP): {r}')
    if r is None:
        # Fallback to HTML interface if HNAP login fails
        print("HNAP login failed; unable to reboot.")
        sys.exit(1)

    # Optional: quick status check before reboot
    status_resp = h.get_status()
    print(f'status (pre‑reboot): {status_resp}')

    # Handle dry-run mode
    if args.dryrun:
        print("Dry‑run mode: skipping reboot command.")
        sys.exit(0)

    # --- Send reboot command ---
    print("Sending reboot command...")
    reboot_resp = h.reboot()
    print(f'reboot: {reboot_resp}')
    if reboot_resp.status_code != 200:
        print(f"Reboot request failed with status {reboot_resp.status_code}: {reboot_resp.text}")
        sys.exit(1)
    print("Reboot command sent successfully.")

    # --- Wait for reboot cycle ---
    if not wait_for_reboot_cycle(host, verify=not args.noverify):
        sys.exit(1)

    # Optional post‑reboot sanity check
    print("Performing post-reboot verification check...")
    try:
        post_status = h.get_status()
        print(f"status (post‑reboot): {post_status}")
    except Exception as e:
        print(f"Post‑reboot status fetch failed: {e}")
    
    print("Modem reboot completed successfully.")
