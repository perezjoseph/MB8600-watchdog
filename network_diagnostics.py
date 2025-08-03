#!/usr/bin/env python3
"""
Network diagnostics module for TCP/IP model testing
Tests different layers of the TCP/IP stack to diagnose connectivity issues
"""
import socket
import subprocess
import time
import logging
import requests
import json
import os
from typing import Dict, List, Tuple, Optional
from dataclasses import dataclass
from enum import Enum

logger = logging.getLogger(__name__)

class NetworkLayer(Enum):
    """TCP/IP Model Layers"""
    PHYSICAL = "Physical"
    DATA_LINK = "Data Link" 
    NETWORK = "Network"
    TRANSPORT = "Transport"
    APPLICATION = "Application"

@dataclass
class DiagnosticResult:
    """Result of a network diagnostic test"""
    layer: NetworkLayer
    test_name: str
    success: bool
    duration: float
    details: Dict
    error: Optional[str] = None

class NetworkDiagnostics:
    """Comprehensive network diagnostics for TCP/IP model testing"""
    
    def __init__(self):
        self.results = []
        self.modem_ip = None
        self.gateway_ip = None
        
    def run_full_diagnostics(self, modem_ip: str = "192.168.100.1") -> List[DiagnosticResult]:
        """
        Run comprehensive network diagnostics across all TCP/IP layers
        
        Args:
            modem_ip: IP address of the modem
            
        Returns:
            List of diagnostic results
        """
        self.modem_ip = modem_ip
        self.results = []
        
        logger.info("Starting comprehensive TCP/IP model diagnostics",
                   extra={'extra_data': {'modem_ip': modem_ip}})
        
        # Layer 1: Physical Layer (simulated through interface checks)
        self._test_physical_layer()
        
        # Layer 2: Data Link Layer (ARP, local network)
        self._test_data_link_layer()
        
        # Layer 3: Network Layer (IP, ICMP, routing)
        self._test_network_layer()
        
        # Layer 4: Transport Layer (TCP, UDP)
        self._test_transport_layer()
        
        # Layer 5: Application Layer (HTTP, DNS)
        self._test_application_layer()
        
        # Analyze results
        self._analyze_results()
        
        return self.results
    
    def _test_physical_layer(self):
        """Test Physical Layer - Network interfaces and basic connectivity"""
        logger.debug("Testing Physical Layer")
        
        # Test network interface status
        result = self._test_network_interfaces()
        self.results.append(result)
        
        # Test cable connectivity (simulated through interface stats)
        result = self._test_interface_statistics()
        self.results.append(result)
    
    def _test_data_link_layer(self):
        """Test Data Link Layer - ARP, MAC addresses, local network"""
        logger.debug("Testing Data Link Layer")
        
        # Test ARP table
        result = self._test_arp_table()
        self.results.append(result)
        
        # Test local network connectivity
        result = self._test_local_network()
        self.results.append(result)
    
    def _test_network_layer(self):
        """Test Network Layer - IP, ICMP, routing"""
        logger.debug("Testing Network Layer")
        
        # Test IP configuration
        result = self._test_ip_configuration()
        self.results.append(result)
        
        # Test routing table
        result = self._test_routing_table()
        self.results.append(result)
        
        # Test ICMP (ping) to various targets
        targets = [
            ("Gateway", self._get_gateway_ip()),
            ("Modem", self.modem_ip),
            ("Local DNS", "1.1.1.1"),
            ("Remote DNS", "8.8.8.8"),
            ("Public Server", "google.com")
        ]
        
        for name, target in targets:
            if target:
                result = self._test_icmp_ping(name, target)
                self.results.append(result)
    
    def _test_transport_layer(self):
        """Test Transport Layer - TCP and UDP connectivity"""
        logger.debug("Testing Transport Layer")
        
        # Test TCP connectivity to various ports
        tcp_tests = [
            ("Modem HTTPS", self.modem_ip, 443),
            ("Modem HTTP", self.modem_ip, 80),
            ("DNS TCP", "1.1.1.1", 53),
            ("HTTPS", "google.com", 443),
            ("HTTP", "google.com", 80)
        ]
        
        for name, host, port in tcp_tests:
            result = self._test_tcp_connection(name, host, port)
            self.results.append(result)
        
        # Test UDP connectivity
        udp_tests = [
            ("DNS UDP", "1.1.1.1", 53),
            ("DNS UDP Alt", "8.8.8.8", 53)
        ]
        
        for name, host, port in udp_tests:
            result = self._test_udp_connection(name, host, port)
            self.results.append(result)
    
    def _test_application_layer(self):
        """Test Application Layer - HTTP, DNS, etc."""
        logger.debug("Testing Application Layer")
        
        # Test DNS resolution
        dns_tests = [
            "google.com",
            "cloudflare.com", 
            "amazon.com",
            "github.com"
        ]
        
        for domain in dns_tests:
            result = self._test_dns_resolution(domain)
            self.results.append(result)
        
        # Test HTTP connectivity
        http_tests = [
            "https://www.google.com",
            "https://www.cloudflare.com",
            "https://www.amazon.com",
            "http://httpbin.org/get"
        ]
        
        for url in http_tests:
            result = self._test_http_request(url)
            self.results.append(result)
    
    def _test_network_interfaces(self) -> DiagnosticResult:
        """Test network interface status"""
        start_time = time.time()
        
        try:
            # Get network interface information
            result = subprocess.run(['ip', 'addr', 'show'], 
                                  capture_output=True, text=True, timeout=10)
            
            duration = time.time() - start_time
            
            if result.returncode == 0:
                interfaces = self._parse_interfaces(result.stdout)
                active_interfaces = [iface for iface in interfaces if iface.get('state') == 'UP']
                
                return DiagnosticResult(
                    layer=NetworkLayer.PHYSICAL,
                    test_name="Network Interfaces",
                    success=len(active_interfaces) > 0,
                    duration=duration,
                    details={
                        'total_interfaces': len(interfaces),
                        'active_interfaces': len(active_interfaces),
                        'interfaces': interfaces
                    }
                )
            else:
                return DiagnosticResult(
                    layer=NetworkLayer.PHYSICAL,
                    test_name="Network Interfaces",
                    success=False,
                    duration=duration,
                    details={'error_output': result.stderr},
                    error="Failed to get interface information"
                )
                
        except Exception as e:
            duration = time.time() - start_time
            return DiagnosticResult(
                layer=NetworkLayer.PHYSICAL,
                test_name="Network Interfaces",
                success=False,
                duration=duration,
                details={},
                error=str(e)
            )
    
    def _test_interface_statistics(self) -> DiagnosticResult:
        """Test network interface statistics"""
        start_time = time.time()
        
        try:
            # Get interface statistics
            result = subprocess.run(['cat', '/proc/net/dev'], 
                                  capture_output=True, text=True, timeout=5)
            
            duration = time.time() - start_time
            
            if result.returncode == 0:
                stats = self._parse_interface_stats(result.stdout)
                
                return DiagnosticResult(
                    layer=NetworkLayer.PHYSICAL,
                    test_name="Interface Statistics",
                    success=True,
                    duration=duration,
                    details={'interface_stats': stats}
                )
            else:
                return DiagnosticResult(
                    layer=NetworkLayer.PHYSICAL,
                    test_name="Interface Statistics",
                    success=False,
                    duration=duration,
                    details={},
                    error="Failed to get interface statistics"
                )
                
        except Exception as e:
            duration = time.time() - start_time
            return DiagnosticResult(
                layer=NetworkLayer.PHYSICAL,
                test_name="Interface Statistics",
                success=False,
                duration=duration,
                details={},
                error=str(e)
            )
    
    def _test_arp_table(self) -> DiagnosticResult:
        """Test ARP table entries"""
        start_time = time.time()
        
        try:
            result = subprocess.run(['arp', '-a'], 
                                  capture_output=True, text=True, timeout=10)
            
            duration = time.time() - start_time
            
            if result.returncode == 0:
                arp_entries = self._parse_arp_table(result.stdout)
                modem_in_arp = any(entry.get('ip') == self.modem_ip for entry in arp_entries)
                
                return DiagnosticResult(
                    layer=NetworkLayer.DATA_LINK,
                    test_name="ARP Table",
                    success=len(arp_entries) > 0,
                    duration=duration,
                    details={
                        'arp_entries': len(arp_entries),
                        'modem_in_arp': modem_in_arp,
                        'entries': arp_entries
                    }
                )
            else:
                return DiagnosticResult(
                    layer=NetworkLayer.DATA_LINK,
                    test_name="ARP Table",
                    success=False,
                    duration=duration,
                    details={},
                    error="Failed to get ARP table"
                )
                
        except Exception as e:
            duration = time.time() - start_time
            return DiagnosticResult(
                layer=NetworkLayer.DATA_LINK,
                test_name="ARP Table",
                success=False,
                duration=duration,
                details={},
                error=str(e)
            )
    
    def _test_local_network(self) -> DiagnosticResult:
        """Test local network connectivity"""
        start_time = time.time()
        
        try:
            # Try to ping the network broadcast address or scan local network
            gateway = self._get_gateway_ip()
            if not gateway:
                return DiagnosticResult(
                    layer=NetworkLayer.DATA_LINK,
                    test_name="Local Network",
                    success=False,
                    duration=time.time() - start_time,
                    details={},
                    error="No gateway found"
                )
            
            # Ping gateway
            result = subprocess.run(['ping', '-c', '1', '-W', '2', gateway], 
                                  capture_output=True, text=True, timeout=5)
            
            duration = time.time() - start_time
            success = result.returncode == 0
            
            return DiagnosticResult(
                layer=NetworkLayer.DATA_LINK,
                test_name="Local Network",
                success=success,
                duration=duration,
                details={
                    'gateway': gateway,
                    'ping_result': success
                }
            )
            
        except Exception as e:
            duration = time.time() - start_time
            return DiagnosticResult(
                layer=NetworkLayer.DATA_LINK,
                test_name="Local Network",
                success=False,
                duration=duration,
                details={},
                error=str(e)
            )
    
    def _test_ip_configuration(self) -> DiagnosticResult:
        """Test IP configuration"""
        start_time = time.time()
        
        try:
            # Get IP configuration
            result = subprocess.run(['ip', 'route', 'show'], 
                                  capture_output=True, text=True, timeout=10)
            
            duration = time.time() - start_time
            
            if result.returncode == 0:
                routes = self._parse_routes(result.stdout)
                default_route = any('default' in route.get('destination', '') for route in routes)
                
                return DiagnosticResult(
                    layer=NetworkLayer.NETWORK,
                    test_name="IP Configuration",
                    success=default_route,
                    duration=duration,
                    details={
                        'routes': routes,
                        'has_default_route': default_route
                    }
                )
            else:
                return DiagnosticResult(
                    layer=NetworkLayer.NETWORK,
                    test_name="IP Configuration",
                    success=False,
                    duration=duration,
                    details={},
                    error="Failed to get IP configuration"
                )
                
        except Exception as e:
            duration = time.time() - start_time
            return DiagnosticResult(
                layer=NetworkLayer.NETWORK,
                test_name="IP Configuration",
                success=False,
                duration=duration,
                details={},
                error=str(e)
            )
    
    def _test_routing_table(self) -> DiagnosticResult:
        """Test routing table"""
        start_time = time.time()
        
        try:
            result = subprocess.run(['route', '-n'], 
                                  capture_output=True, text=True, timeout=10)
            
            duration = time.time() - start_time
            
            if result.returncode == 0:
                return DiagnosticResult(
                    layer=NetworkLayer.NETWORK,
                    test_name="Routing Table",
                    success=True,
                    duration=duration,
                    details={'routing_table': result.stdout}
                )
            else:
                return DiagnosticResult(
                    layer=NetworkLayer.NETWORK,
                    test_name="Routing Table",
                    success=False,
                    duration=duration,
                    details={},
                    error="Failed to get routing table"
                )
                
        except Exception as e:
            duration = time.time() - start_time
            return DiagnosticResult(
                layer=NetworkLayer.NETWORK,
                test_name="Routing Table",
                success=False,
                duration=duration,
                details={},
                error=str(e)
            )
    
    def _test_icmp_ping(self, name: str, target: str) -> DiagnosticResult:
        """Test ICMP ping to target"""
        start_time = time.time()
        
        try:
            result = subprocess.run(['ping', '-c', '3', '-W', '5', target], 
                                  capture_output=True, text=True, timeout=20)
            
            duration = time.time() - start_time
            success = result.returncode == 0
            
            # Parse ping statistics
            stats = self._parse_ping_output(result.stdout) if success else {}
            
            return DiagnosticResult(
                layer=NetworkLayer.NETWORK,
                test_name=f"ICMP Ping ({name})",
                success=success,
                duration=duration,
                details={
                    'target': target,
                    'ping_stats': stats,
                    'output': result.stdout if success else result.stderr
                }
            )
            
        except Exception as e:
            duration = time.time() - start_time
            return DiagnosticResult(
                layer=NetworkLayer.NETWORK,
                test_name=f"ICMP Ping ({name})",
                success=False,
                duration=duration,
                details={'target': target},
                error=str(e)
            )
    
    def _test_tcp_connection(self, name: str, host: str, port: int) -> DiagnosticResult:
        """Test TCP connection to host:port"""
        start_time = time.time()
        
        try:
            sock = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
            sock.settimeout(10)
            
            result = sock.connect_ex((host, port))
            sock.close()
            
            duration = time.time() - start_time
            success = result == 0
            
            return DiagnosticResult(
                layer=NetworkLayer.TRANSPORT,
                test_name=f"TCP Connection ({name})",
                success=success,
                duration=duration,
                details={
                    'host': host,
                    'port': port,
                    'connect_result': result
                }
            )
            
        except Exception as e:
            duration = time.time() - start_time
            return DiagnosticResult(
                layer=NetworkLayer.TRANSPORT,
                test_name=f"TCP Connection ({name})",
                success=False,
                duration=duration,
                details={'host': host, 'port': port},
                error=str(e)
            )
    
    def _test_udp_connection(self, name: str, host: str, port: int) -> DiagnosticResult:
        """Test UDP connection to host:port"""
        start_time = time.time()
        
        try:
            sock = socket.socket(socket.AF_INET, socket.SOCK_DGRAM)
            sock.settimeout(5)
            
            # Send a simple DNS query for UDP test
            if port == 53:
                # Simple DNS query for google.com
                query = b'\x12\x34\x01\x00\x00\x01\x00\x00\x00\x00\x00\x00\x06google\x03com\x00\x00\x01\x00\x01'
                sock.sendto(query, (host, port))
                response = sock.recv(512)
                success = len(response) > 0
            else:
                # Generic UDP test
                sock.sendto(b'test', (host, port))
                success = True
            
            sock.close()
            duration = time.time() - start_time
            
            return DiagnosticResult(
                layer=NetworkLayer.TRANSPORT,
                test_name=f"UDP Connection ({name})",
                success=success,
                duration=duration,
                details={
                    'host': host,
                    'port': port
                }
            )
            
        except Exception as e:
            duration = time.time() - start_time
            return DiagnosticResult(
                layer=NetworkLayer.TRANSPORT,
                test_name=f"UDP Connection ({name})",
                success=False,
                duration=duration,
                details={'host': host, 'port': port},
                error=str(e)
            )
    
    def _test_dns_resolution(self, domain: str) -> DiagnosticResult:
        """Test DNS resolution"""
        start_time = time.time()
        
        try:
            ip_address = socket.gethostbyname(domain)
            duration = time.time() - start_time
            
            return DiagnosticResult(
                layer=NetworkLayer.APPLICATION,
                test_name=f"DNS Resolution ({domain})",
                success=True,
                duration=duration,
                details={
                    'domain': domain,
                    'resolved_ip': ip_address
                }
            )
            
        except Exception as e:
            duration = time.time() - start_time
            return DiagnosticResult(
                layer=NetworkLayer.APPLICATION,
                test_name=f"DNS Resolution ({domain})",
                success=False,
                duration=duration,
                details={'domain': domain},
                error=str(e)
            )
    
    def _test_http_request(self, url: str) -> DiagnosticResult:
        """Test HTTP request"""
        start_time = time.time()
        
        try:
            response = requests.get(url, timeout=10)
            duration = time.time() - start_time
            success = response.status_code == 200
            
            return DiagnosticResult(
                layer=NetworkLayer.APPLICATION,
                test_name=f"HTTP Request ({url})",
                success=success,
                duration=duration,
                details={
                    'url': url,
                    'status_code': response.status_code,
                    'response_size': len(response.content)
                }
            )
            
        except Exception as e:
            duration = time.time() - start_time
            return DiagnosticResult(
                layer=NetworkLayer.APPLICATION,
                test_name=f"HTTP Request ({url})",
                success=False,
                duration=duration,
                details={'url': url},
                error=str(e)
            )
    
    def _get_gateway_ip(self) -> Optional[str]:
        """Get the default gateway IP address"""
        if self.gateway_ip:
            return self.gateway_ip
            
        try:
            result = subprocess.run(['ip', 'route', 'show', 'default'], 
                                  capture_output=True, text=True, timeout=5)
            
            if result.returncode == 0:
                # Parse default route
                for line in result.stdout.split('\n'):
                    if 'default via' in line:
                        parts = line.split()
                        if len(parts) >= 3:
                            self.gateway_ip = parts[2]
                            return self.gateway_ip
                            
        except Exception as e:
            logger.debug(f"Failed to get gateway IP: {e}")
            
        return None
    
    def _analyze_results(self):
        """Analyze diagnostic results and provide summary"""
        layer_results = {}
        
        for result in self.results:
            layer = result.layer.value
            if layer not in layer_results:
                layer_results[layer] = {'passed': 0, 'failed': 0, 'total': 0}
            
            layer_results[layer]['total'] += 1
            if result.success:
                layer_results[layer]['passed'] += 1
            else:
                layer_results[layer]['failed'] += 1
        
        logger.info("Network diagnostics summary:",
                   extra={'extra_data': {
                       'layer_results': layer_results,
                       'total_tests': len(self.results),
                       'total_passed': sum(1 for r in self.results if r.success),
                       'total_failed': sum(1 for r in self.results if not r.success)
                   }})
    
    def get_failure_analysis(self) -> Dict:
        """Analyze failures and suggest potential causes"""
        failures_by_layer = {}
        
        for result in self.results:
            if not result.success:
                layer = result.layer.value
                if layer not in failures_by_layer:
                    failures_by_layer[layer] = []
                failures_by_layer[layer].append(result)
        
        analysis = {
            'failures_by_layer': failures_by_layer,
            'likely_causes': [],
            'recommended_actions': []
        }
        
        # Analyze failure patterns
        if NetworkLayer.PHYSICAL.value in failures_by_layer:
            analysis['likely_causes'].append("Physical layer issues (cable, interface)")
            analysis['recommended_actions'].append("Check network cables and interface status")
        
        if NetworkLayer.DATA_LINK.value in failures_by_layer:
            analysis['likely_causes'].append("Local network issues (switch, ARP)")
            analysis['recommended_actions'].append("Check local network connectivity")
        
        if NetworkLayer.NETWORK.value in failures_by_layer:
            analysis['likely_causes'].append("IP/routing issues")
            analysis['recommended_actions'].append("Check IP configuration and routing")
        
        if NetworkLayer.TRANSPORT.value in failures_by_layer:
            analysis['likely_causes'].append("Port/firewall issues")
            analysis['recommended_actions'].append("Check firewall rules and port accessibility")
        
        if NetworkLayer.APPLICATION.value in failures_by_layer:
            analysis['likely_causes'].append("DNS or application-level issues")
            analysis['recommended_actions'].append("Check DNS configuration and application services")
        
        return analysis
    
    # Helper methods for parsing command outputs
    def _parse_interfaces(self, output: str) -> List[Dict]:
        """Parse ip addr show output"""
        interfaces = []
        current_interface = None
        
        for line in output.split('\n'):
            line = line.strip()
            if line and line[0].isdigit():
                # New interface
                parts = line.split(':')
                if len(parts) >= 2:
                    current_interface = {
                        'name': parts[1].strip(),
                        'state': 'UP' if 'UP' in line else 'DOWN',
                        'addresses': []
                    }
                    interfaces.append(current_interface)
            elif current_interface and 'inet ' in line:
                # IP address
                parts = line.split()
                if len(parts) >= 2:
                    current_interface['addresses'].append(parts[1])
        
        return interfaces
    
    def _parse_interface_stats(self, output: str) -> Dict:
        """Parse /proc/net/dev output"""
        stats = {}
        lines = output.split('\n')[2:]  # Skip header lines
        
        for line in lines:
            if ':' in line:
                parts = line.split(':')
                if len(parts) == 2:
                    interface = parts[0].strip()
                    values = parts[1].split()
                    if len(values) >= 8:
                        stats[interface] = {
                            'rx_bytes': int(values[0]),
                            'rx_packets': int(values[1]),
                            'tx_bytes': int(values[8]),
                            'tx_packets': int(values[9])
                        }
        
        return stats
    
    def _parse_arp_table(self, output: str) -> List[Dict]:
        """Parse arp -a output"""
        entries = []
        
        for line in output.split('\n'):
            if '(' in line and ')' in line:
                # Extract IP and MAC
                parts = line.split()
                if len(parts) >= 4:
                    ip = parts[1].strip('()')
                    mac = parts[3] if len(parts) > 3 else 'unknown'
                    entries.append({
                        'ip': ip,
                        'mac': mac,
                        'interface': parts[-1] if len(parts) > 4 else 'unknown'
                    })
        
        return entries
    
    def _parse_routes(self, output: str) -> List[Dict]:
        """Parse ip route show output"""
        routes = []
        
        for line in output.split('\n'):
            if line.strip():
                parts = line.split()
                if parts:
                    route = {
                        'destination': parts[0],
                        'gateway': None,
                        'interface': None
                    }
                    
                    if 'via' in parts:
                        via_index = parts.index('via')
                        if via_index + 1 < len(parts):
                            route['gateway'] = parts[via_index + 1]
                    
                    if 'dev' in parts:
                        dev_index = parts.index('dev')
                        if dev_index + 1 < len(parts):
                            route['interface'] = parts[dev_index + 1]
                    
                    routes.append(route)
        
        return routes
    
    def _parse_ping_output(self, output: str) -> Dict:
        """Parse ping command output"""
        stats = {}
        
        for line in output.split('\n'):
            if 'packets transmitted' in line:
                # Parse packet statistics
                parts = line.split()
                if len(parts) >= 6:
                    stats['transmitted'] = int(parts[0])
                    stats['received'] = int(parts[3])
                    stats['loss_percent'] = float(parts[5].rstrip('%'))
            elif 'min/avg/max' in line:
                # Parse timing statistics
                parts = line.split('=')
                if len(parts) >= 2:
                    times = parts[1].strip().split('/')
                    if len(times) >= 3:
                        stats['min_time'] = float(times[0])
                        stats['avg_time'] = float(times[1])
                        stats['max_time'] = float(times[2])
        
        return stats
