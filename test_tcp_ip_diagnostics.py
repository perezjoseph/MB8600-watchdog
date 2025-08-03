#!/usr/bin/env python3
"""
Test script to demonstrate TCP/IP model diagnostics
"""
import sys
import os
import time
from pathlib import Path

# Add current directory to path to import our modules
sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))

from monitor_internet_improved import setup_logging
from network_diagnostics import NetworkDiagnostics, NetworkLayer
import logging

def test_tcp_ip_diagnostics():
    """Test the TCP/IP model diagnostics system"""
    
    # Create logs directory
    log_dir = Path("./logs")
    log_dir.mkdir(exist_ok=True)
    
    # Setup logging
    logger = setup_logging(
        log_level='DEBUG',
        log_file='./logs/diagnostics_test.log',
        log_max_size=1024*1024,  # 1MB for testing
        log_backup_count=3
    )
    
    logger.info("=== Testing TCP/IP Model Diagnostics ===")
    
    # Test with default modem IP
    modem_ip = "192.168.100.1"
    
    print(f"Running comprehensive TCP/IP diagnostics for modem: {modem_ip}")
    print("This will test all layers of the TCP/IP model...")
    print()
    
    # Initialize diagnostics
    network_diag = NetworkDiagnostics()
    
    # Run diagnostics
    start_time = time.time()
    results = network_diag.run_full_diagnostics(modem_ip)
    total_duration = time.time() - start_time
    
    # Display results by layer
    layer_results = {}
    for result in results:
        layer = result.layer.value
        if layer not in layer_results:
            layer_results[layer] = []
        layer_results[layer].append(result)
    
    print(f"Diagnostics completed in {total_duration:.2f} seconds")
    print(f"Total tests run: {len(results)}")
    print()
    
    # Display results by layer
    for layer in NetworkLayer:
        layer_name = layer.value
        if layer_name in layer_results:
            tests = layer_results[layer_name]
            passed = sum(1 for t in tests if t.success)
            failed = len(tests) - passed
            
            print(f"=== {layer_name} Layer ===")
            print(f"Tests: {len(tests)}, Passed: {passed}, Failed: {failed}")
            
            for test in tests:
                status = "✓ PASS" if test.success else "✗ FAIL"
                duration = f"{test.duration:.3f}s"
                print(f"  {status} {test.test_name} ({duration})")
                
                if not test.success and test.error:
                    print(f"    Error: {test.error}")
                
                # Show some interesting details
                if test.details:
                    if 'ping_stats' in test.details and test.details['ping_stats']:
                        stats = test.details['ping_stats']
                        if 'avg_time' in stats:
                            print(f"    Avg ping time: {stats['avg_time']:.1f}ms")
                    
                    if 'status_code' in test.details:
                        print(f"    HTTP status: {test.details['status_code']}")
                    
                    if 'resolved_ip' in test.details:
                        print(f"    Resolved to: {test.details['resolved_ip']}")
            
            print()
    
    # Get failure analysis
    failure_analysis = network_diag.get_failure_analysis()
    
    if failure_analysis['failures_by_layer']:
        print("=== Failure Analysis ===")
        for layer, failures in failure_analysis['failures_by_layer'].items():
            print(f"{layer} Layer failures: {len(failures)}")
            for failure in failures:
                print(f"  - {failure.test_name}: {failure.error}")
        
        if failure_analysis['likely_causes']:
            print("\nLikely causes:")
            for cause in failure_analysis['likely_causes']:
                print(f"  - {cause}")
        
        if failure_analysis['recommended_actions']:
            print("\nRecommended actions:")
            for action in failure_analysis['recommended_actions']:
                print(f"  - {action}")
        print()
    
    # Test the reboot decision logic
    from monitor_internet_improved import _analyze_reboot_necessity
    
    should_reboot = _analyze_reboot_necessity(results, failure_analysis)
    
    print("=== Reboot Decision ===")
    print(f"Should reboot modem: {'YES' if should_reboot else 'NO'}")
    
    if should_reboot:
        print("Reason: Network diagnostics indicate connectivity issues that may be resolved by modem reboot")
    else:
        print("Reason: Network diagnostics suggest issues may not be resolved by modem reboot")
        if failure_analysis.get('recommended_actions'):
            print("Alternative actions recommended:")
            for action in failure_analysis['recommended_actions']:
                print(f"  - {action}")
    
    print()
    print("=== Summary ===")
    total_tests = len(results)
    passed_tests = sum(1 for r in results if r.success)
    failed_tests = total_tests - passed_tests
    success_rate = (passed_tests / total_tests * 100) if total_tests > 0 else 0
    
    print(f"Total tests: {total_tests}")
    print(f"Passed: {passed_tests}")
    print(f"Failed: {failed_tests}")
    print(f"Success rate: {success_rate:.1f}%")
    print(f"Total duration: {total_duration:.2f} seconds")
    
    print(f"\nDetailed logs written to: {log_dir.absolute()}/diagnostics_test.log")
    print("JSON structured logs: diagnostics_test.json")
    
    return results, failure_analysis, should_reboot

def test_specific_layer(layer_name):
    """Test a specific layer of the TCP/IP model"""
    
    logger = setup_logging(log_level='INFO')
    
    print(f"Testing {layer_name} layer specifically...")
    
    network_diag = NetworkDiagnostics()
    
    if layer_name.lower() == 'physical':
        network_diag._test_physical_layer()
    elif layer_name.lower() == 'datalink':
        network_diag._test_data_link_layer()
    elif layer_name.lower() == 'network':
        network_diag._test_network_layer()
    elif layer_name.lower() == 'transport':
        network_diag._test_transport_layer()
    elif layer_name.lower() == 'application':
        network_diag._test_application_layer()
    else:
        print(f"Unknown layer: {layer_name}")
        return
    
    # Display results
    for result in network_diag.results:
        status = "✓ PASS" if result.success else "✗ FAIL"
        print(f"{status} {result.test_name} ({result.duration:.3f}s)")
        if not result.success:
            print(f"  Error: {result.error}")

if __name__ == "__main__":
    if len(sys.argv) > 1:
        # Test specific layer
        test_specific_layer(sys.argv[1])
    else:
        # Run full diagnostics test
        test_tcp_ip_diagnostics()
