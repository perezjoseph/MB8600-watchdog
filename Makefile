# MB8600 Watchdog Go Build System

# Build variables
BINARY_NAME=watchdog
MAIN_PATH=./cmd/watchdog
BUILD_DIR=./build
VERSION?=dev
COMMIT?=$(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
BUILD_TIME?=$(shell date -u +"%Y-%m-%dT%H:%M:%SZ")

# Installation directories (following FHS)
PREFIX?=/usr/local
BINDIR=$(PREFIX)/bin
SYSCONFDIR=/etc
LOCALSTATEDIR=/var
SYSTEMDDIR=/etc/systemd/system
INSTALL_DIR=/opt/mb8600-watchdog
SERVICE_USER=watchdog

# Go build flags for static linking with size optimization
LDFLAGS=-ldflags "-s -w -X main.version=$(VERSION) -X main.commit=$(COMMIT) -X main.buildTime=$(BUILD_TIME) -extldflags '-static'"
BUILD_FLAGS=CGO_ENABLED=0 GOOS=linux GOARCH=amd64
OPTIMIZATION_FLAGS=-trimpath -buildmode=exe

# Default target
.PHONY: all
all: lint fmt-check test build

# Quality check target - runs linting and formatting before anything else
.PHONY: check
check: lint fmt-check

# Format check (non-modifying)
.PHONY: fmt-check
fmt-check:
	@echo "Checking code formatting..."
	@if [ -n "$$(gofmt -l .)" ]; then \
		echo "Code is not formatted. Please run 'make fmt'"; \
		gofmt -l .; \
		exit 1; \
	fi
	@echo "Code formatting is correct"

# Build the binary with static linking and size optimization
.PHONY: build
build:
	@echo "Building $(BINARY_NAME) for Linux (static, optimized)..."
	@mkdir -p $(BUILD_DIR)
	$(BUILD_FLAGS) go build $(OPTIMIZATION_FLAGS) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME) $(MAIN_PATH)
	@echo "Build complete: $(BUILD_DIR)/$(BINARY_NAME)"
	@echo "Binary size: $$(du -h $(BUILD_DIR)/$(BINARY_NAME) | cut -f1)"

# Install system (requires root)
.PHONY: install
install: build check-root
	@echo "Installing MB8600 Watchdog..."
	
	# Create service user
	@if ! id $(SERVICE_USER) >/dev/null 2>&1; then \
		echo "Creating user: $(SERVICE_USER)"; \
		useradd --system --shell /bin/false --home-dir $(INSTALL_DIR) $(SERVICE_USER); \
	fi
	
	# Create directories
	@echo "Creating directories..."
	@mkdir -p $(INSTALL_DIR)/bin
	@mkdir -p $(INSTALL_DIR)/config
	@mkdir -p $(INSTALL_DIR)/logs
	@mkdir -p $(INSTALL_DIR)/state
	@mkdir -p $(LOCALSTATEDIR)/log/mb8600-watchdog
	@mkdir -p $(SYSCONFDIR)/mb8600-watchdog
	
	# Install binary
	@echo "Installing binary..."
	@install -m 755 $(BUILD_DIR)/$(BINARY_NAME) $(INSTALL_DIR)/bin/
	@ln -sf $(INSTALL_DIR)/bin/$(BINARY_NAME) $(BINDIR)/mb8600-watchdog
	
	# Install configuration
	@echo "Installing configuration..."
	@install -m 640 -o root -g $(SERVICE_USER) config/production.json $(INSTALL_DIR)/config/
	@install -m 640 -o root -g $(SERVICE_USER) config/production.json $(SYSCONFDIR)/mb8600-watchdog/config.json
	
	# Install systemd service
	@echo "Installing systemd service..."
	@install -m 644 systemd/mb8600-watchdog.service $(SYSTEMDDIR)/
	@systemctl daemon-reload
	
	# Set permissions
	@echo "Setting permissions..."
	@chown -R $(SERVICE_USER):$(SERVICE_USER) $(INSTALL_DIR) $(LOCALSTATEDIR)/log/mb8600-watchdog
	@chmod 750 $(INSTALL_DIR)
	@chmod 755 $(INSTALL_DIR)/bin
	@chmod 750 $(INSTALL_DIR)/config
	@chmod 750 $(INSTALL_DIR)/logs
	@chmod 750 $(INSTALL_DIR)/state
	
	@echo ""
	@echo "Installation complete!"
	@echo ""
	@echo "Next steps:"
	@echo "1. Edit configuration: $(SYSCONFDIR)/mb8600-watchdog/config.json"
	@echo "2. Set your modem password in the config file"
	@echo "3. Enable service: systemctl enable mb8600-watchdog"
	@echo "4. Start service: systemctl start mb8600-watchdog"
	@echo "5. Check status: systemctl status mb8600-watchdog"

# Uninstall system
.PHONY: uninstall
uninstall: check-root
	@echo "Uninstalling MB8600 Watchdog..."
	
	# Stop and disable service
	-@systemctl stop mb8600-watchdog 2>/dev/null || true
	-@systemctl disable mb8600-watchdog 2>/dev/null || true
	
	# Remove systemd service
	@rm -f $(SYSTEMDDIR)/mb8600-watchdog.service
	@systemctl daemon-reload
	
	# Remove binary symlink
	@rm -f $(BINDIR)/mb8600-watchdog
	
	# Remove installation directory
	@rm -rf $(INSTALL_DIR)
	
	# Remove config directory
	@rm -rf $(SYSCONFDIR)/mb8600-watchdog
	
	# Remove log directory
	@rm -rf $(LOCALSTATEDIR)/log/mb8600-watchdog
	
	# Remove user (optional - commented out for safety)
	# @userdel $(SERVICE_USER) 2>/dev/null || true
	
	@echo "Uninstallation complete!"
	@echo "Note: User '$(SERVICE_USER)' was not removed for safety"

# Install user-local version (no root required)
.PHONY: install-user
install-user: build
	@echo "Installing MB8600 Watchdog for current user..."
	
	# Create user directories
	@mkdir -p ~/.local/bin
	@mkdir -p ~/.config/mb8600-watchdog
	@mkdir -p ~/.local/share/mb8600-watchdog/{logs,state}
	
	# Install binary
	@install -m 755 $(BUILD_DIR)/$(BINARY_NAME) ~/.local/bin/mb8600-watchdog
	
	# Install configuration
	@install -m 640 config/production.json ~/.config/mb8600-watchdog/config.json
	
	# Create user systemd service
	@mkdir -p ~/.config/systemd/user
	@sed 's|/opt/mb8600-watchdog|~/.local/share/mb8600-watchdog|g; s|/var/log/mb8600-watchdog|~/.local/share/mb8600-watchdog/logs|g; s|User=watchdog||; s|Group=watchdog||' systemd/mb8600-watchdog.service > ~/.config/systemd/user/mb8600-watchdog.service
	@systemctl --user daemon-reload
	
	@echo ""
	@echo "User installation complete!"
	@echo ""
	@echo "Next steps:"
	@echo "1. Edit configuration: ~/.config/mb8600-watchdog/config.json"
	@echo "2. Set your modem password in the config file"
	@echo "3. Enable service: systemctl --user enable mb8600-watchdog"
	@echo "4. Start service: systemctl --user start mb8600-watchdog"
	@echo "5. Check status: systemctl --user status mb8600-watchdog"
	@echo ""
	@echo "Add ~/.local/bin to your PATH if not already done"

# Uninstall user-local version
.PHONY: uninstall-user
uninstall-user:
	@echo "Uninstalling user MB8600 Watchdog..."
	
	# Stop and disable user service
	-@systemctl --user stop mb8600-watchdog 2>/dev/null || true
	-@systemctl --user disable mb8600-watchdog 2>/dev/null || true
	
	# Remove user systemd service
	@rm -f ~/.config/systemd/user/mb8600-watchdog.service
	@systemctl --user daemon-reload
	
	# Remove binary
	@rm -f ~/.local/bin/mb8600-watchdog
	
	# Remove directories
	@rm -rf ~/.config/mb8600-watchdog
	@rm -rf ~/.local/share/mb8600-watchdog
	
	@echo "User uninstallation complete!"

# Check if running as root
.PHONY: check-root
check-root:
	@if [ "$$(id -u)" != "0" ]; then \
		echo "Error: This target requires root privileges. Run with sudo."; \
		exit 1; \
	fi

# Package for distribution
.PHONY: package
package: build
	@echo "Creating distribution package..."
	@mkdir -p dist/mb8600-watchdog-$(VERSION)
	@cp $(BUILD_DIR)/$(BINARY_NAME) dist/mb8600-watchdog-$(VERSION)/
	@cp config/production.json dist/mb8600-watchdog-$(VERSION)/config.json
	@cp systemd/mb8600-watchdog.service dist/mb8600-watchdog-$(VERSION)/
	@cp scripts/install.sh dist/mb8600-watchdog-$(VERSION)/
	@cp README.md DEPLOYMENT.md LICENSE dist/mb8600-watchdog-$(VERSION)/
	@cd dist && tar -czf mb8600-watchdog-$(VERSION).tar.gz mb8600-watchdog-$(VERSION)/
	@echo "Package created: dist/mb8600-watchdog-$(VERSION).tar.gz"

# Build for current platform (development)
.PHONY: build-dev
build-dev:
	@echo "Building $(BINARY_NAME) for development..."
	@mkdir -p $(BUILD_DIR)
	go build -o $(BUILD_DIR)/$(BINARY_NAME)-dev $(MAIN_PATH)
	@echo "Development build complete: $(BUILD_DIR)/$(BINARY_NAME)-dev"

# Cross-compile for multiple platforms
.PHONY: build-all
build-all: clean
	@echo "Building for multiple platforms..."
	@mkdir -p $(BUILD_DIR)
	
	# Linux AMD64
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build $(OPTIMIZATION_FLAGS) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-linux-amd64 $(MAIN_PATH)
	
	# Linux ARM64
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build $(OPTIMIZATION_FLAGS) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-linux-arm64 $(MAIN_PATH)
	
	# Linux ARM
	CGO_ENABLED=0 GOOS=linux GOARCH=arm GOARM=7 go build $(OPTIMIZATION_FLAGS) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-linux-arm7 $(MAIN_PATH)
	
	@echo "Cross-compilation complete"
	@ls -lah $(BUILD_DIR)/ | grep $(BINARY_NAME)

# Run tests
.PHONY: test
test:
	@echo "Running tests..."
	go test -v ./...

# Run tests with coverage
.PHONY: test-coverage
test-coverage:
	@echo "Running tests with coverage..."
	go test -v -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report generated: coverage.html"

# Format code
.PHONY: fmt
fmt:
	@echo "Formatting code..."
	go fmt ./...

# Parallel linting configuration
LINT_TIMEOUT?=5m
LINT_CONCURRENCY?=4
LINT_SUBAGENT_TIMEOUT?=30s

# Run linting with parallel system and fallback
.PHONY: lint
lint: build-lint-coordinator
	@echo "Running parallel linting system..."
	@if $(BUILD_DIR)/lint-coordinator -timeout=$(LINT_TIMEOUT) -concurrency=$(LINT_CONCURRENCY) -subagent-timeout=$(LINT_SUBAGENT_TIMEOUT) 2>/dev/null; then \
		echo "Parallel linting completed successfully"; \
	else \
		echo "Parallel linting failed, falling back to standard golangci-lint..."; \
		which golangci-lint > /dev/null || (echo "Installing golangci-lint..." && go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest); \
		export PATH=$$PATH:$$(go env GOPATH)/bin && golangci-lint run; \
	fi

# Build the parallel linting coordinator
.PHONY: build-lint-coordinator
build-lint-coordinator:
	@echo "Building parallel linting coordinator..."
	@mkdir -p $(BUILD_DIR)
	@go build -o $(BUILD_DIR)/lint-coordinator ./cmd/lint-coordinator

# Build linting subagents
.PHONY: build-lint-subagents
build-lint-subagents:
	@echo "Building linting subagents..."
	@mkdir -p $(BUILD_DIR)
	@go build -o $(BUILD_DIR)/lint-subagent ./cmd/lint-subagent

# Build all linting tools
.PHONY: build-lint-tools
build-lint-tools: build-lint-coordinator build-lint-subagents

# Run parallel linting with bash coordinator
.PHONY: lint-parallel-bash
lint-parallel-bash:
	@echo "Running parallel linting with bash coordinator..."
	@./scripts/run-parallel-linting.sh

# Test parallel linting implementation
.PHONY: test-lint-parallel
test-lint-parallel:
	@echo "Testing parallel linting implementation..."
	@./scripts/test-parallel-linting.sh

# Run parallel linting with unified reporting
.PHONY: lint-parallel
lint-parallel: build-lint-coordinator
	@echo "Running parallel linting with unified reporting..."
	@$(BUILD_DIR)/lint-coordinator -timeout=$(LINT_TIMEOUT) -concurrency=$(LINT_CONCURRENCY) -subagent-timeout=$(LINT_SUBAGENT_TIMEOUT)

# Build the parallel linting tool
.PHONY: build-lint-tool
build-lint-tool:
	@echo "Building parallel linting tool..."
	@mkdir -p $(BUILD_DIR)
	@go build -o $(BUILD_DIR)/lint-parallel ./cmd/lint-parallel

# Run parallel linting with JSON output
.PHONY: lint-json
lint-json: build-lint-coordinator
	@echo "Running parallel linting with JSON output..."
	@$(BUILD_DIR)/lint-coordinator -timeout=$(LINT_TIMEOUT) -concurrency=$(LINT_CONCURRENCY) -subagent-timeout=$(LINT_SUBAGENT_TIMEOUT) -format json -output lint-report.json
	@echo "Report saved to lint-report.json"

# Build linting subagents (legacy - kept for compatibility)
.PHONY: build-linters
build-linters: build-lint-tools
	@echo "Built parallel linting tools"

# Individual module linting targets (using new parallel system)
.PHONY: lint-core
lint-core: build-lint-subagents
	@echo "Linting core modules..."
	@$(BUILD_DIR)/lint-subagent -timeout=$(LINT_SUBAGENT_TIMEOUT) ./internal/config ./internal/app ./internal/logger

.PHONY: lint-network
lint-network: build-lint-subagents
	@echo "Linting network modules..."
	@$(BUILD_DIR)/lint-subagent -timeout=$(LINT_SUBAGENT_TIMEOUT) ./internal/connectivity ./internal/hnap

.PHONY: lint-monitoring
lint-monitoring: build-lint-subagents
	@echo "Linting monitoring modules..."
	@$(BUILD_DIR)/lint-subagent -timeout=$(LINT_SUBAGENT_TIMEOUT) ./internal/monitor ./internal/outage ./internal/performance

.PHONY: lint-system
lint-system: build-lint-subagents
	@echo "Linting system modules..."
	@$(BUILD_DIR)/lint-subagent -timeout=$(LINT_SUBAGENT_TIMEOUT) ./internal/system ./internal/diagnostics

.PHONY: lint-integration
lint-integration: build-lint-subagents
	@echo "Linting integration modules..."
	@$(BUILD_DIR)/lint-subagent -timeout=$(LINT_SUBAGENT_TIMEOUT) ./internal/integration ./internal/circuitbreaker

.PHONY: lint-entry
lint-entry: build-lint-subagents
	@echo "Linting entry module..."
	@$(BUILD_DIR)/lint-subagent -timeout=$(LINT_SUBAGENT_TIMEOUT) ./cmd/watchdog

# Tidy dependencies
.PHONY: tidy
tidy:
	@echo "Tidying dependencies..."
	go mod tidy

# Clean build artifacts
.PHONY: clean
clean:
	@echo "Cleaning build artifacts..."
	@rm -rf $(BUILD_DIR) dist/
	@rm -f coverage.out coverage.html lint-report.json

# Run the development binary
.PHONY: run
run: build-dev
	@echo "Running $(BINARY_NAME)..."
	$(BUILD_DIR)/$(BINARY_NAME)-dev

# Help
.PHONY: help
help:
	@echo "MB8600 Watchdog Build System"
	@echo ""
	@echo "Build targets:"
	@echo "  build        - Build static binary for Linux"
	@echo "  build-dev    - Build binary for development"
	@echo "  build-all    - Cross-compile for multiple platforms"
	@echo "  clean        - Clean build artifacts"
	@echo ""
	@echo "Quality targets:"
	@echo "  check        - Run linting and format check"
	@echo "  lint         - Run parallel linting with fallback to golangci-lint"
	@echo "  lint-parallel- Run parallel linting with unified reporting"
	@echo "  lint-json    - Run parallel linting with JSON output"
	@echo "  fmt-check    - Check code formatting"
	@echo "  fmt          - Format code"
	@echo ""
	@echo "Linting configuration (environment variables):"
	@echo "  LINT_TIMEOUT      - Overall timeout (default: 5m)"
	@echo "  LINT_CONCURRENCY  - Number of parallel workers (default: 4)"
	@echo "  LINT_SUBAGENT_TIMEOUT - Individual subagent timeout (default: 30s)"
	@echo ""
	@echo "Installation targets:"
	@echo "  install      - Install system-wide (requires sudo)"
	@echo "  uninstall    - Uninstall system-wide (requires sudo)"
	@echo "  install-user - Install for current user only"
	@echo "  uninstall-user - Uninstall user installation"
	@echo ""
	@echo "Development targets:"
	@echo "  test         - Run tests"
	@echo "  test-coverage- Run tests with coverage"
	@echo "  lint         - Run parallel linting with fallback"
	@echo "  lint-parallel- Run specialized parallel linting"
	@echo "  build-lint-tools - Build all linting tools"
	@echo "  lint-core    - Lint core modules (config, app, logger)"
	@echo "  lint-network - Lint network modules (connectivity, hnap)"
	@echo "  lint-monitoring - Lint monitoring modules (monitor, outage, performance)"
	@echo "  lint-system  - Lint system modules (system, diagnostics)"
	@echo "  lint-integration - Lint integration modules (integration, circuitbreaker)"
	@echo "  lint-entry   - Lint entry module (cmd/watchdog)"
	@echo "  fmt          - Format code"
	@echo "  tidy         - Tidy dependencies"
	@echo "  run          - Build and run development binary"
	@echo ""
	@echo "Distribution targets:"
	@echo "  package      - Create distribution package"
	@echo ""
	@echo "Usage examples:"
	@echo "  make build && sudo make install"
	@echo "  make install-user"
	@echo "  make package"
