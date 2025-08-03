#!/usr/bin/env python3
"""
Test script to demonstrate outage duration tracking
"""
import sys
import os
import time
from pathlib import Path
from datetime import datetime

# Add current directory to path to import our modules
sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))

from monitor_internet_improved import setup_logging, log_periodic_outage_report
import logging

def simulate_outage_scenario():
    """Simulate various outage scenarios to test tracking"""
    
    # Create logs directory
    log_dir = Path("./logs")
    log_dir.mkdir(exist_ok=True)
    
    # Setup logging
    logger = setup_logging(
        log_level='INFO',
        log_file='./logs/outage_test.log',
        log_max_size=1024*1024,
        log_backup_count=3
    )
    
    logger.info("=== Testing Outage Duration Tracking ===")
    
    print("Simulating various outage scenarios...")
    print("This will demonstrate how outage duration is tracked and logged.")
    print()
    
    # Scenario 1: Short outage (30 seconds)
    print("Scenario 1: Short outage (30 seconds)")
    outage_start = time.time()
    
    logger.warning("Internet outage started at {}".format(
        datetime.now().strftime('%Y-%m-%d %H:%M:%S')),
        extra={'extra_data': {
            'outage_started': True,
            'outage_start_time': datetime.now().isoformat(),
            'failure_count': 1
        }})
    
    # Simulate checking during outage
    for i in range(3):
        time.sleep(10)  # Wait 10 seconds
        current_duration = time.time() - outage_start
        logger.debug(f"Internet outage continues - duration: {current_duration:.1f}s "
                    f"({current_duration/60:.1f} minutes)",
                    extra={'extra_data': {
                        'outage_ongoing': True,
                        'current_outage_duration_seconds': current_duration,
                        'current_outage_duration_minutes': current_duration / 60,
                        'failure_count': i + 2
                    }})
        print(f"  Outage ongoing for {current_duration:.1f} seconds...")
    
    # Outage resolved
    outage_duration = time.time() - outage_start
    logger.warning(f"Internet outage resolved after {outage_duration:.1f} seconds "
                  f"({outage_duration/60:.1f} minutes)",
                  extra={'extra_data': {
                      'outage_duration_seconds': outage_duration,
                      'outage_duration_minutes': outage_duration / 60,
                      'outage_duration_hours': outage_duration / 3600,
                      'outage_start_time': datetime.fromtimestamp(outage_start).isoformat(),
                      'outage_end_time': datetime.now().isoformat(),
                      'failure_count_during_outage': 4,
                      'total_outage_duration_today': outage_duration,
                      'outage_resolved': True
                  }})
    
    print(f"  Outage resolved after {outage_duration:.1f} seconds")
    print()
    
    # Scenario 2: Reboot-triggered outage
    print("Scenario 2: Outage leading to reboot")
    outage_start_2 = time.time()
    
    logger.warning("Internet outage started at {}".format(
        datetime.now().strftime('%Y-%m-%d %H:%M:%S')),
        extra={'extra_data': {
            'outage_started': True,
            'outage_start_time': datetime.now().isoformat(),
            'failure_count': 1
        }})
    
    # Simulate longer outage leading to reboot
    time.sleep(15)  # Wait 15 seconds
    
    outage_duration_at_reboot = time.time() - outage_start_2
    logger.warning(f"Reboot initiated after {outage_duration_at_reboot:.1f} seconds "
                  f"({outage_duration_at_reboot/60:.1f} minutes) of internet outage",
                  extra={'extra_data': {
                      'reboot_trigger_outage_duration_seconds': outage_duration_at_reboot,
                      'reboot_trigger_outage_duration_minutes': outage_duration_at_reboot / 60,
                      'failure_count_at_reboot': 5
                  }})
    
    print(f"  Reboot triggered after {outage_duration_at_reboot:.1f} seconds of outage")
    
    # Simulate reboot recovery time
    time.sleep(10)  # Simulate reboot time
    
    total_outage_2 = time.time() - outage_start_2
    logger.warning(f"Internet outage resolved after {total_outage_2:.1f} seconds "
                  f"({total_outage_2/60:.1f} minutes) including reboot time",
                  extra={'extra_data': {
                      'outage_duration_seconds': total_outage_2,
                      'outage_duration_minutes': total_outage_2 / 60,
                      'outage_resolved_after_reboot': True,
                      'reboot_recovery_included': True
                  }})
    
    print(f"  Total outage including reboot: {total_outage_2:.1f} seconds")
    print()
    
    # Scenario 3: Periodic report
    print("Scenario 3: Periodic outage report")
    total_outage_time = outage_duration + total_outage_2
    total_uptime = 300  # Simulate 5 minutes of total uptime
    
    log_periodic_outage_report(total_outage_time, total_uptime)
    
    availability = ((total_uptime - total_outage_time) / total_uptime * 100)
    print(f"  Total outage time: {total_outage_time:.1f} seconds ({total_outage_time/60:.1f} minutes)")
    print(f"  Total uptime: {total_uptime} seconds ({total_uptime/60:.1f} minutes)")
    print(f"  Availability: {availability:.2f}%")
    print()
    
    # Scenario 4: Ongoing outage at shutdown
    print("Scenario 4: Ongoing outage at shutdown")
    outage_start_3 = time.time()
    
    logger.warning("Internet outage started at {}".format(
        datetime.now().strftime('%Y-%m-%d %H:%M:%S')),
        extra={'extra_data': {
            'outage_started': True,
            'outage_start_time': datetime.now().isoformat(),
            'failure_count': 1
        }})
    
    time.sleep(5)  # Short ongoing outage
    
    # Simulate shutdown during outage
    final_outage_duration = time.time() - outage_start_3
    total_outage_final = total_outage_time + final_outage_duration
    final_uptime = total_uptime + 30  # Add 30 seconds
    
    logger.warning(f"Monitoring stopped during internet outage - final outage duration: "
                  f"{final_outage_duration:.1f} seconds ({final_outage_duration/60:.1f} minutes)",
                  extra={'extra_data': {
                      'final_outage_duration_seconds': final_outage_duration,
                      'final_outage_duration_minutes': final_outage_duration / 60,
                      'outage_ongoing_at_shutdown': True
                  }})
    
    outage_percentage = (total_outage_final / final_uptime * 100)
    logger.warning(f"Total internet outage time during monitoring: "
                  f"{total_outage_final:.1f} seconds ({total_outage_final/60:.1f} minutes, "
                  f"{total_outage_final/3600:.2f} hours) - {outage_percentage:.1f}% of uptime",
                  extra={'extra_data': {
                      'total_outage_summary': True,
                      'total_outage_duration_seconds': total_outage_final,
                      'total_outage_duration_minutes': total_outage_final / 60,
                      'total_outage_duration_hours': total_outage_final / 3600,
                      'uptime_seconds': final_uptime,
                      'outage_percentage': outage_percentage
                  }})
    
    print(f"  Final outage ongoing for {final_outage_duration:.1f} seconds")
    print(f"  Total outage time: {total_outage_final:.1f} seconds ({total_outage_final/60:.1f} minutes)")
    print(f"  Final availability: {100-outage_percentage:.2f}%")
    print()
    
    print("=== Outage Tracking Test Complete ===")
    print(f"Check the logs: {log_dir.absolute()}/outage_test.log")
    print("JSON logs: outage_test.json")
    
    print("\nExample log queries:")
    print("# Find all outage start events:")
    print("cat logs/outage_test.json | jq 'select(.extra.outage_started == true)'")
    print()
    print("# Find all outage resolution events:")
    print("cat logs/outage_test.json | jq 'select(.extra.outage_resolved == true)'")
    print()
    print("# Find outage duration summaries:")
    print("cat logs/outage_test.json | jq 'select(.extra.total_outage_summary == true)'")
    print()
    print("# Calculate average outage duration:")
    print("cat logs/outage_test.json | jq -r 'select(.extra.outage_duration_seconds) | .extra.outage_duration_seconds' | awk '{sum+=$1; count++} END {print \"Average outage:\", sum/count, \"seconds\"}'")

def test_outage_metrics():
    """Test outage metrics calculation"""
    
    print("\n=== Testing Outage Metrics Calculation ===")
    
    # Test scenarios
    scenarios = [
        {"uptime": 3600, "outage": 60, "desc": "1 minute outage in 1 hour"},
        {"uptime": 86400, "outage": 300, "desc": "5 minute outage in 24 hours"},
        {"uptime": 3600, "outage": 1800, "desc": "30 minute outage in 1 hour"},
        {"uptime": 86400, "outage": 3600, "desc": "1 hour outage in 24 hours"},
    ]
    
    for scenario in scenarios:
        uptime = scenario["uptime"]
        outage = scenario["outage"]
        desc = scenario["desc"]
        
        availability = ((uptime - outage) / uptime * 100)
        outage_percentage = (outage / uptime * 100)
        
        print(f"{desc}:")
        print(f"  Uptime: {uptime/3600:.1f} hours")
        print(f"  Outage: {outage/60:.1f} minutes ({outage/3600:.2f} hours)")
        print(f"  Availability: {availability:.3f}%")
        print(f"  Downtime: {outage_percentage:.3f}%")
        print()

if __name__ == "__main__":
    if len(sys.argv) > 1 and sys.argv[1] == "metrics":
        test_outage_metrics()
    else:
        simulate_outage_scenario()
        test_outage_metrics()
