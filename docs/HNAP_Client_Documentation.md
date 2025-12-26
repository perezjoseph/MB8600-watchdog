# HNAP Client Documentation

## Overview

The HNAP (Home Network Administration Protocol) client is a Go implementation that provides authentication and control capabilities for Motorola/ARRIS cable modems (specifically MB8600 series). This client is a direct port from Python and implements the complete HNAP authentication sequence and reboot functionality.

## Architecture

### Core Components

1. **SurfboardHNAP Struct** - Main client implementation
2. **Authentication State** - Challenge/response mechanism
3. **HTTP Client** - Session management with cookies
4. **HMAC-MD5 Cryptography** - Authentication key generation

### Key Files

- `internal/hnap/client.go` - Main client implementation
- `internal/hnap/client_test.go` - Unit tests
- `internal/monitor/service.go` - Integration with monitoring system
- `internal/integration/integration_test.go` - End-to-end tests

## Complete HNAP Authentication Sequence

### Phase 1: HTML Form Login
```
Client → Modem: GET /Login.html
Client ← Modem: HTML login page

Client → Modem: POST /cgi-bin/moto/goform/MotoLogin
                Content-Type: application/x-www-form-urlencoded
                Body: loginUsername=admin&loginPassword=motorola
Client ← Modem: Session cookies established
```

**Purpose**: Establishes initial HTTP session and cookies required for HNAP communication.

### Phase 2: HNAP Challenge Request
```
Client → Modem: POST /HNAP1/
                Content-Type: application/json
                SOAPACTION: "http://purenetworks.com/HNAP1/Login"
                Body: {
                  "Login": {
                    "Action": "request",
                    "Username": "admin",
                    "LoginPassword": "",
                    "Captcha": "",
                    "PrivateLogin": "LoginPassword"
                  }
                }

Client ← Modem: {
                  "LoginResponse": {
                    "Challenge": "random_challenge_string",
                    "PublicKey": "public_key_string",
                    "Cookie": "session_cookie",
                    "LoginResult": "CHALLENGE"
                  }
                }
```

**Purpose**: Obtains cryptographic challenge and public key for secure authentication.

### Phase 3: Key Generation
```go
// Generate private key: HMAC-MD5(publicKey + password, challenge)
key := publicKey + password
privateKey := HMAC_MD5(key, challenge)

// Generate password key: HMAC-MD5(privateKey, challenge)  
passwordKey := HMAC_MD5(privateKey, challenge)
```

**Purpose**: Creates cryptographic keys for secure authentication using HMAC-MD5.

### Phase 4: HNAP Authentication
```
Client → Modem: POST /HNAP1/
                Content-Type: application/json
                SOAPACTION: "http://purenetworks.com/HNAP1/Login"
                HNAP_AUTH: {hash} {timestamp}
                Cookie: uid={session_cookie}
                Body: {
                  "Login": {
                    "Action": "login",
                    "Username": "admin", 
                    "LoginPassword": "{passwordKey}",
                    "Captcha": "",
                    "PrivateLogin": "LoginPassword"
                  }
                }

Client ← Modem: {
                  "LoginResponse": {
                    "LoginResult": "OK"
                  }
                }
```

**HNAP_AUTH Header Generation**:
```go
timestamp := currentTimeMillis()
authKey := timestamp + "\"http://purenetworks.com/HNAP1/Login\""
authHash := HMAC_MD5(privateKey, authKey)
HNAP_AUTH := authHash + " " + timestamp
```

**Purpose**: Completes secure authentication using generated keys and establishes authenticated session.

## Reboot Sequence

### HNAP Reboot Command
```
Client → Modem: POST /HNAP1/
                Content-Type: application/json; charset=UTF-8
                SOAPACTION: "http://purenetworks.com/HNAP1/SetStatusSecuritySettings"
                HNAP_AUTH: {hash} {timestamp}
                Cookie: uid={session_cookie}
                Body: {
                  "SetStatusSecuritySettings": {
                    "MotoStatusSecurityAction": "1",
                    "MotoStatusSecXXX": "XXX"
                  }
                }

Client ← Modem: Response (may be empty if modem reboots immediately)
```

**Purpose**: Triggers immediate modem reboot using proprietary Motorola HNAP action.

## Implementation Details

### Client Structure
```go
type SurfboardHNAP struct {
    host       string        // Modem IP address
    username   string        // Login username (typically "admin")
    password   string        // Login password (typically "motorola")
    noVerify   bool          // Skip TLS certificate verification
    httpClient *http.Client  // HTTP client with cookie jar
    logger     *logrus.Logger // Structured logging
    baseURL    string        // Base HTTPS URL
    
    // HNAP authentication state
    challenge  string        // Server challenge
    publicKey  string        // Server public key
    privateKey string        // Generated private key
    cookie     string         // Session cookie
}
```

### Key Methods

#### Authentication Flow
```go
func (s *SurfboardHNAP) Login(ctx context.Context) error
├── loginHTMLForm(ctx)     // Phase 1: HTML form login
├── loginRequest(ctx)      // Phase 2: HNAP challenge request  
├── generateKeys()         // Phase 3: Key generation
└── loginReal(ctx)         // Phase 4: HNAP authentication
```

#### Reboot Operations
```go
func (s *SurfboardHNAP) Reboot(ctx context.Context) error
├── Authentication check
└── tryRebootMethod(ctx, "SetStatusSecuritySettings", payload)

func (s *SurfboardHNAP) RebootWithMonitoring(ctx context.Context, ...) (*RebootCycleResult, error)
├── Reboot(ctx)
└── Monitor offline/online cycle
```

### Cryptographic Operations

#### HMAC-MD5 Key Generation
```go
// Private key generation
key := publicKey + password
h := hmac.New(md5.New, []byte(key))
h.Write([]byte(challenge))
privateKey := strings.ToUpper(hex.EncodeToString(h.Sum(nil)))

// Password key generation  
h = hmac.New(md5.New, []byte(privateKey))
h.Write([]byte(challenge))
passwordKey := strings.ToUpper(hex.EncodeToString(h.Sum(nil)))

// HNAP_AUTH generation
timestamp := time.Now().UnixNano() / int64(time.Millisecond)
authKey := fmt.Sprintf("%d\"http://purenetworks.com/HNAP1/%s\"", timestamp, action)
h = hmac.New(md5.New, []byte(privateKey))
h.Write([]byte(authKey))
authHash := strings.ToUpper(hex.EncodeToString(h.Sum(nil)))
hnapAuth := fmt.Sprintf("%s %d", authHash, timestamp)
```

## Integration with Monitoring System

### Service Integration
The HNAP client integrates with the monitoring service (`internal/monitor/service.go`):

```go
type Service struct {
    hnapClient *hnap.Client  // HNAP client instance
    // ... other components
}

// Reboot trigger in monitoring loop
func (s *Service) triggerReboot(ctx context.Context) error {
    if s.config.EnableRebootMonitoring {
        result, err := s.hnapClient.RebootWithMonitoring(...)
        // Handle monitored reboot cycle
    } else {
        err := s.hnapClient.Reboot(ctx)
        // Handle basic reboot
    }
}
```

### Failure Handling
- **Authentication Errors**: Client re-authenticates automatically on next request
- **Network Errors**: Graceful error handling with context cancellation
- **Timeout Management**: Configurable timeouts for all operations
- **Retry Logic**: Built into monitoring service for failed operations

## Configuration

### Client Configuration
```go
client := hnap.NewClient(
    "192.168.100.1",  // Modem IP
    "admin",          // Username  
    "motorola",       // Password
    true,             // Skip TLS verification
    logger,           // Logger instance
)
```

### Monitoring Integration
```go
// In config.json
{
    "modem_host": "192.168.100.1",
    "modem_username": "admin", 
    "modem_password": "motorola",
    "modem_no_verify": true,
    "enable_reboot_monitoring": true,
    "reboot_poll_interval": "5s",
    "reboot_offline_timeout": "2m",
    "reboot_online_timeout": "5m"
}
```

## Error Handling

### Common Error Scenarios
1. **Network Connectivity**: Connection refused, timeouts
2. **Authentication Failures**: Invalid credentials, challenge/response mismatch
3. **HNAP Protocol Errors**: Invalid responses, missing fields
4. **Reboot Failures**: Command rejected, modem unresponsive

### Error Recovery
- Automatic re-authentication on auth failures
- Context-based timeout management
- Graceful degradation when monitoring unavailable
- Detailed error logging with structured fields

## Testing

### Unit Tests (`client_test.go`)
- Client initialization validation
- HNAP_AUTH header generation
- Authentication flow testing
- Error handling verification

### Integration Tests (`integration_test.go`)
- End-to-end authentication flow
- Reboot command execution
- Network error handling
- Mock server interactions

### Test Coverage
- Authentication sequence validation
- Cryptographic key generation
- HTTP request/response handling
- Error condition testing

## Security Considerations

### Cryptographic Security
- Uses HMAC-MD5 (legacy but required by modem firmware)
- Challenge/response prevents replay attacks
- Session cookies provide state management
- TLS encryption for transport security

### Network Security
- Configurable TLS certificate verification
- Session-based authentication
- Timeout-based security (prevents hanging connections)
- Structured logging avoids credential exposure

## Performance Characteristics

### Timing
- HTML login: ~100-200ms
- HNAP challenge: ~100-200ms  
- Key generation: <1ms
- HNAP authentication: ~100-200ms
- Reboot command: ~100-500ms
- **Total authentication time**: ~400-800ms

### Resource Usage
- Memory: Minimal (session state only)
- CPU: Low (cryptographic operations are lightweight)
- Network: 4-5 HTTP requests per authentication cycle
- Connections: Single persistent HTTP client with connection pooling

## Troubleshooting

### Common Issues
1. **"HNAP login failed"**: Check credentials, network connectivity
2. **"Challenge or public key not available"**: HTML login may have failed
3. **"Authentication required"**: Session expired, re-authentication needed
4. **"Reboot command failed"**: Check authentication state, network connectivity

### Debug Logging
Enable debug logging to trace the complete sequence:
```go
logger.SetLevel(logrus.DebugLevel)
```

Debug output includes:
- HTTP request/response details
- Challenge/response values
- Generated keys (private key only)
- HNAP_AUTH headers
- Reboot command payloads

## Future Enhancements

### Potential Improvements
1. **Additional HNAP Commands**: Status queries, configuration changes
2. **Enhanced Monitoring**: Detailed reboot cycle metrics
3. **Connection Pooling**: Optimize for high-frequency operations
4. **Async Operations**: Non-blocking reboot with callbacks
5. **Protocol Validation**: Stricter HNAP response parsing

### Compatibility
- Designed for Motorola MB8600 series modems
- May work with other ARRIS/Motorola HNAP-enabled devices
- Python compatibility maintained for easy migration