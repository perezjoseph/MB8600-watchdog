# MB8600 Watchdog - Quick Start (Linux Only)

**Platform Support**: This application only runs on Linux systems. It will exit with an error on Windows, macOS, or other operating systems.

## Installation Options

### Option 1: Standard Make Install (Recommended)
```bash
# Build and install system-wide
make build
sudo make install

# Configure and start
sudo nano /etc/mb8600-watchdog/config.json  # Set your modem password
sudo systemctl enable mb8600-watchdog
sudo systemctl start mb8600-watchdog
```

### Option 2: User Installation (No Root Required)
```bash
# Install for current user only
make build
make install-user

# Configure and start
nano ~/.config/mb8600-watchdog/config.json  # Set your modem password
systemctl --user enable mb8600-watchdog
systemctl --user start mb8600-watchdog
```

### Option 3: Legacy Script Install
```bash
# Alternative installation using the legacy script
sudo ./scripts/install.sh
```

## Key Configuration Settings

Copy the example config and customize:

```bash
cp config/config.example.json config/config.json
nano config/config.json  # Set your modem password
```

Edit the config file and update:

```json
{
  "ModemHost": "192.168.100.1",
  "ModemUsername": "admin", 
  "ModemPassword": "YOUR_MODEM_PASSWORD_HERE",
  "CheckInterval": "2m",
  "FailureThreshold": 3,
  "LogLevel": "INFO"
}
```

## Service Management

```bash
# System-wide installation
sudo systemctl status mb8600-watchdog
sudo systemctl start|stop|restart mb8600-watchdog
sudo journalctl -u mb8600-watchdog -f

# User installation  
systemctl --user status mb8600-watchdog
systemctl --user start|stop|restart mb8600-watchdog
journalctl --user -u mb8600-watchdog -f

# Health check
mb8600-watchdog status
```

## Available Commands

```bash
# Show service status and statistics
mb8600-watchdog status

# Reload service configuration (sends SIGHUP)
mb8600-watchdog reload

# Stop the running service (sends SIGTERM)
mb8600-watchdog stop

# Generate shell completion scripts
mb8600-watchdog completion bash
mb8600-watchdog completion zsh
mb8600-watchdog completion fish

# Show help for any command
mb8600-watchdog help [command]
```

## Uninstallation

```bash
# System-wide
sudo make uninstall

# User installation
make uninstall-user
```

## Build Targets

```bash
make build        # Production build
make build-dev    # Development build
make build-all    # Cross-platform builds
make test         # Run tests
make test-coverage # Run tests with coverage
make package      # Create distribution package
make clean        # Clean build artifacts
```

## How It Works

1. **Monitors Connectivity**: Tests internet connectivity every 2 minutes
2. **Smart Analysis**: Uses network diagnostics to determine if modem reboot would help
3. **Automatic Reboot**: Reboots modem via HNAP protocol when necessary
4. **Prevents Unnecessary Reboots**: Won't reboot if problem is external to modem

## Logs and Reports

- **System logs**: `/var/log/mb8600-watchdog/` or `~/.local/share/mb8600-watchdog/logs/`
- **Service logs**: `journalctl -u mb8600-watchdog`
- **Outage reports**: Auto-generated in logs directory
