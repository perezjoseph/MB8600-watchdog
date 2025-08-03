#!/usr/bin/env python3
"""
Test script to demonstrate the improved logging capabilities
"""
import sys
import os
import time
from pathlib import Path

# Add current directory to path to import our modules
sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))

from monitor_internet_improved import setup_logging, JsonFormatter
import logging

def test_logging_features():
    """Test various logging features"""
    
    # Create logs directory
    log_dir = Path("./logs")
    log_dir.mkdir(exist_ok=True)
    
    # Setup logging with file output
    logger = setup_logging(
        log_level='DEBUG',
        log_file='./logs/test.log',
        log_max_size=1024*1024,  # 1MB for testing
        log_backup_count=3
    )
    
    logger.info("=== Testing Enhanced Logging System ===")
    
    # Test different log levels
    logger.debug("This is a DEBUG message - detailed information for developers")
    logger.info("This is an INFO message - general information")
    logger.warning("This is a WARNING message - something might be wrong")
    logger.error("This is an ERROR message - something went wrong")
    
    # Test structured logging with extra data
    logger.info("Testing structured logging", 
               extra={'extra_data': {
                   'test_type': 'structured_logging',
                   'timestamp': time.time(),
                   'test_data': {'key1': 'value1', 'key2': 42}
               }})
    
    # Test performance logging
    start_time = time.time()
    time.sleep(0.1)  # Simulate some work
    duration = time.time() - start_time
    
    logger.info(f"Simulated operation completed in {duration:.3f}s",
               extra={'extra_data': {
                   'operation': 'test_operation',
                   'duration': duration,
                   'success': True
               }})
    
    # Test error logging with exception
    try:
        raise ValueError("This is a test exception")
    except Exception as e:
        logger.error(f"Caught test exception: {e}",
                    extra={'extra_data': {
                        'error_type': type(e).__name__,
                        'error_message': str(e),
                        'test_exception': True
                    }}, exc_info=True)
    
    # Test connection-like logging
    for host in ['1.1.1.1', '8.8.8.8', 'invalid.host']:
        success = host != 'invalid.host'
        duration = 0.05 if success else 2.0
        
        if success:
            logger.info(f"Successfully connected to {host}",
                       extra={'extra_data': {
                           'host': host,
                           'duration': duration,
                           'result': 'success',
                           'connection_type': 'test'
                       }})
        else:
            logger.warning(f"Failed to connect to {host}",
                          extra={'extra_data': {
                              'host': host,
                              'duration': duration,
                              'result': 'failed',
                              'connection_type': 'test',
                              'error': 'Host not reachable'
                          }})
    
    # Test reboot-like logging
    logger.warning("Simulating reboot scenario",
                  extra={'extra_data': {
                      'action': 'reboot_initiated',
                      'failure_count': 5,
                      'failure_threshold': 5,
                      'reason': 'connectivity_failure'
                  }})
    
    logger.info("Reboot simulation completed",
               extra={'extra_data': {
                   'action': 'reboot_completed',
                   'total_duration': 120.5,
                   'success': True
               }})
    
    logger.info("=== Logging Test Complete ===")
    
    # Show what files were created
    print("\nLog files created:")
    for log_file in log_dir.glob("test.*"):
        size = log_file.stat().st_size
        print(f"  {log_file.name}: {size} bytes")
    
    print(f"\nCheck the logs directory: {log_dir.absolute()}")
    print("- test.log: Human-readable detailed logs")
    print("- test.json: Machine-readable structured logs")
    
    print("\nTo view JSON logs in a readable format:")
    print("  cat logs/test.json | jq .")
    
    print("\nTo filter JSON logs by level:")
    print("  cat logs/test.json | jq 'select(.level == \"ERROR\")'")
    
    print("\nTo extract performance data:")
    print("  cat logs/test.json | jq 'select(.extra.duration) | {message, duration: .extra.duration}'")

if __name__ == "__main__":
    test_logging_features()
