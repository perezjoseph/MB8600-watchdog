#!/bin/bash

# MB8600-Watchdog Deployment Script
# This script helps you deploy the right version of MB8600-watchdog

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Function to print colored output
print_color() {
    printf "${1}${2}${NC}\n"
}

print_header() {
    echo
    print_color $BLUE "=================================="
    print_color $BLUE "$1"
    print_color $BLUE "=================================="
    echo
}

print_success() {
    print_color $GREEN "✓ $1"
}

print_warning() {
    print_color $YELLOW "⚠ $1"
}

print_error() {
    print_color $RED "✗ $1"
}

# Check if Docker and Docker Compose are installed
check_dependencies() {
    print_header "Checking Dependencies"
    
    if ! command -v docker &> /dev/null; then
        print_error "Docker is not installed. Please install Docker first."
        exit 1
    fi
    print_success "Docker is installed"
    
    if ! command -v docker-compose &> /dev/null; then
        print_error "Docker Compose is not installed. Please install Docker Compose first."
        exit 1
    fi
    print_success "Docker Compose is installed"
    
    # Check if Docker daemon is running
    if ! docker info &> /dev/null; then
        print_error "Docker daemon is not running. Please start Docker first."
        exit 1
    fi
    print_success "Docker daemon is running"
}

# Function to get user input with default
get_input() {
    local prompt="$1"
    local default="$2"
    local var_name="$3"
    
    if [ -n "$default" ]; then
        read -p "$prompt [$default]: " input
        if [ -z "$input" ]; then
            input="$default"
        fi
    else
        read -p "$prompt: " input
    fi
    
    eval "$var_name='$input'"
}

# Function to validate IP address
validate_ip() {
    local ip=$1
    if [[ $ip =~ ^[0-9]{1,3}\.[0-9]{1,3}\.[0-9]{1,3}\.[0-9]{1,3}$ ]]; then
        return 0
    else
        return 1
    fi
}

# Main deployment function
deploy_watchdog() {
    print_header "MB8600-Watchdog Deployment"
    
    echo "This script will help you deploy MB8600-watchdog with the right configuration."
    echo
    
    # Choose version
    echo "Available versions:"
    echo "1) Enhanced (Recommended) - TCP/IP diagnostics, outage tracking, advanced logging"
    echo "2) Standard - Basic internet monitoring and modem rebooting"
    echo
    
    while true; do
        read -p "Choose version (1 or 2) [1]: " version_choice
        version_choice=${version_choice:-1}
        
        case $version_choice in
            1)
                VERSION="enhanced"
                PROFILE="enhanced"
                break
                ;;
            2)
                VERSION="standard"
                PROFILE="standard"
                break
                ;;
            *)
                print_error "Please choose 1 or 2"
                ;;
        esac
    done
    
    print_success "Selected: $VERSION version"
    
    # Get configuration
    print_header "Configuration"
    
    get_input "Modem IP address" "192.168.100.1" MODEM_HOST
    while ! validate_ip "$MODEM_HOST"; do
        print_error "Invalid IP address format"
        get_input "Modem IP address" "192.168.100.1" MODEM_HOST
    done
    
    get_input "Modem admin username" "admin" MODEM_USERNAME
    get_input "Modem admin password" "motorola" MODEM_PASSWORD
    
    if [ "$MODEM_PASSWORD" = "motorola" ]; then
        print_warning "You're using the default password. Consider changing it for security."
    fi
    
    get_input "Check interval (seconds)" "60" CHECK_INTERVAL
    get_input "Failure threshold" "5" FAILURE_THRESHOLD
    get_input "Recovery wait time (seconds)" "600" RECOVERY_WAIT
    
    # Enhanced version specific settings
    if [ "$VERSION" = "enhanced" ]; then
        echo
        print_color $BLUE "Enhanced Version Settings:"
        
        get_input "Log level (DEBUG/INFO/WARNING/ERROR)" "INFO" LOG_LEVEL
        
        while true; do
            read -p "Enable TCP/IP diagnostics? (y/n) [y]: " enable_diag
            enable_diag=${enable_diag:-y}
            case $enable_diag in
                [Yy]*)
                    ENABLE_DIAGNOSTICS="true"
                    get_input "Diagnostics timeout (seconds)" "120" DIAGNOSTICS_TIMEOUT
                    break
                    ;;
                [Nn]*)
                    ENABLE_DIAGNOSTICS="false"
                    DIAGNOSTICS_TIMEOUT="120"
                    break
                    ;;
                *)
                    print_error "Please answer y or n"
                    ;;
            esac
        done
        
        get_input "Outage report interval (seconds)" "3600" OUTAGE_REPORT_INTERVAL
        
        while true; do
            read -p "Include log viewer web interface? (y/n) [n]: " include_logs
            include_logs=${include_logs:-n}
            case $include_logs in
                [Yy]*)
                    PROFILE="enhanced logs"
                    LOG_VIEWER_PORT="8080"
                    break
                    ;;
                [Nn]*)
                    break
                    ;;
                *)
                    print_error "Please answer y or n"
                    ;;
            esac
        done
    fi
    
    # Create directories
    print_header "Setting Up Environment"
    
    if [ "$VERSION" = "enhanced" ]; then
        mkdir -p logs config
        print_success "Created logs and config directories"
    fi
    
    # Create or update docker-compose override
    print_success "Creating docker-compose configuration..."
    
    cat > docker-compose.override.yml << EOF
version: '3.8'

services:
EOF
    
    if [ "$VERSION" = "enhanced" ]; then
        cat >> docker-compose.override.yml << EOF
  internet-monitor-enhanced:
    environment:
      - MODEM_HOST=$MODEM_HOST
      - MODEM_USERNAME=$MODEM_USERNAME
      - MODEM_PASSWORD=$MODEM_PASSWORD
      - CHECK_INTERVAL=$CHECK_INTERVAL
      - FAILURE_THRESHOLD=$FAILURE_THRESHOLD
      - RECOVERY_WAIT=$RECOVERY_WAIT
      - LOG_LEVEL=$LOG_LEVEL
      - ENABLE_DIAGNOSTICS=$ENABLE_DIAGNOSTICS
      - DIAGNOSTICS_TIMEOUT=$DIAGNOSTICS_TIMEOUT
      - OUTAGE_REPORT_INTERVAL=$OUTAGE_REPORT_INTERVAL
EOF
        
        if [ -n "$LOG_VIEWER_PORT" ]; then
            cat >> docker-compose.override.yml << EOF
  log-viewer:
    ports:
      - "$LOG_VIEWER_PORT:8080"
EOF
        fi
    else
        cat >> docker-compose.override.yml << EOF
  internet-monitor:
    environment:
      - MODEM_HOST=$MODEM_HOST
      - MODEM_USERNAME=$MODEM_USERNAME
      - MODEM_PASSWORD=$MODEM_PASSWORD
      - CHECK_INTERVAL=$CHECK_INTERVAL
      - FAILURE_THRESHOLD=$FAILURE_THRESHOLD
      - RECOVERY_WAIT=$RECOVERY_WAIT
EOF
    fi
    
    print_success "Configuration saved to docker-compose.override.yml"
    
    # Deploy
    print_header "Deploying MB8600-Watchdog"
    
    echo "Starting deployment with profile: $PROFILE"
    
    if [ "$PROFILE" = "enhanced logs" ]; then
        docker-compose --profile enhanced --profile logs up -d
    else
        docker-compose --profile $PROFILE up -d
    fi
    
    if [ $? -eq 0 ]; then
        print_success "Deployment completed successfully!"
        
        echo
        print_header "Deployment Summary"
        echo "Version: $VERSION"
        echo "Modem: $MODEM_HOST"
        echo "Check interval: $CHECK_INTERVAL seconds"
        echo "Failure threshold: $FAILURE_THRESHOLD"
        
        if [ "$VERSION" = "enhanced" ]; then
            echo "Log level: $LOG_LEVEL"
            echo "Diagnostics: $ENABLE_DIAGNOSTICS"
            echo "Outage reports: Every $OUTAGE_REPORT_INTERVAL seconds"
            
            if [ -n "$LOG_VIEWER_PORT" ]; then
                echo
                print_color $GREEN "Log viewer available at: http://localhost:$LOG_VIEWER_PORT"
            fi
            
            echo
            print_color $BLUE "Useful commands:"
            echo "  View logs: docker-compose logs -f internet-monitor-enhanced"
            echo "  View JSON logs: tail -f logs/watchdog.json | jq ."
            echo "  Test diagnostics: docker exec mb8600-watchdog-enhanced python3 test_tcp_ip_diagnostics.py"
            echo "  Check outages: cat logs/watchdog.json | jq 'select(.extra.outage_resolved)'"
        else
            echo
            print_color $BLUE "Useful commands:"
            echo "  View logs: docker-compose logs -f internet-monitor"
            echo "  Check status: docker-compose ps"
        fi
        
        echo
        print_color $BLUE "Management commands:"
        echo "  Stop: docker-compose down"
        echo "  Restart: docker-compose restart"
        echo "  Update: docker-compose pull && docker-compose up -d"
        
    else
        print_error "Deployment failed. Check the logs above for details."
        exit 1
    fi
}

# Function to show status
show_status() {
    print_header "MB8600-Watchdog Status"
    
    echo "Container Status:"
    docker-compose ps
    
    echo
    echo "Recent Logs:"
    if docker-compose ps | grep -q "mb8600-watchdog-enhanced"; then
        docker-compose logs --tail=10 internet-monitor-enhanced
    elif docker-compose ps | grep -q "mb8600-watchdog-standard"; then
        docker-compose logs --tail=10 internet-monitor
    else
        print_warning "No MB8600-watchdog containers found"
    fi
}

# Function to stop services
stop_services() {
    print_header "Stopping MB8600-Watchdog"
    
    docker-compose down
    
    if [ $? -eq 0 ]; then
        print_success "Services stopped successfully"
    else
        print_error "Failed to stop services"
    fi
}

# Function to show help
show_help() {
    echo "MB8600-Watchdog Deployment Script"
    echo
    echo "Usage: $0 [COMMAND]"
    echo
    echo "Commands:"
    echo "  deploy    Deploy MB8600-watchdog (default)"
    echo "  status    Show current status"
    echo "  stop      Stop all services"
    echo "  help      Show this help message"
    echo
    echo "Examples:"
    echo "  $0                # Interactive deployment"
    echo "  $0 deploy         # Interactive deployment"
    echo "  $0 status         # Show status"
    echo "  $0 stop           # Stop services"
}

# Main script logic
main() {
    case "${1:-deploy}" in
        deploy)
            check_dependencies
            deploy_watchdog
            ;;
        status)
            show_status
            ;;
        stop)
            stop_services
            ;;
        help|--help|-h)
            show_help
            ;;
        *)
            print_error "Unknown command: $1"
            show_help
            exit 1
            ;;
    esac
}

# Run main function
main "$@"
