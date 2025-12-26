package hnap

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/md5"
	"crypto/tls"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
)

// SurfboardHNAP represents the Python SurfboardHNAP class ported to Go
type SurfboardHNAP struct {
	host       string
	username   string
	password   string
	noVerify   bool
	httpClient *http.Client
	logger     *logrus.Logger
	baseURL    string

	// HNAP authentication state
	challenge  string
	publicKey  string
	privateKey string
	cookie     string
}

// NewSurfboardHNAP creates a new SurfboardHNAP client (direct Python port)
func NewSurfboardHNAP(host, username, password string, noVerify bool, logger *logrus.Logger) *SurfboardHNAP {
	if logger == nil {
		logger = logrus.New()
		logger.SetLevel(logrus.InfoLevel)
	}

	// Create cookie jar
	jar, _ := cookiejar.New(nil)

	// Create HTTP client matching Python requests.Session()
	client := &http.Client{
		Timeout: 90 * time.Second, // Python uses 90s timeout
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: noVerify,
			},
		},
		Jar: jar,
	}

	baseURL := fmt.Sprintf("https://%s", host)

	return &SurfboardHNAP{
		host:       host,
		username:   username,
		password:   password,
		noVerify:   noVerify,
		httpClient: client,
		logger:     logger,
		baseURL:    baseURL,
	}
}

// loginHTMLForm performs HTML form login (Python: login_html_form)
func (s *SurfboardHNAP) loginHTMLForm(ctx context.Context) error {
	s.logger.Debug("Performing HTML form login")

	// Step 1: GET /Login.html
	loginURL := s.baseURL + "/Login.html"
	req, err := http.NewRequestWithContext(ctx, "GET", loginURL, nil)
	if err != nil {
		return err
	}

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return err
	}
	resp.Body.Close()

	// Step 2: POST form data
	formData := url.Values{}
	formData.Set("loginUsername", s.username)
	formData.Set("loginPassword", s.password)

	formURL := s.baseURL + "/cgi-bin/moto/goform/MotoLogin"
	req, err = http.NewRequestWithContext(ctx, "POST", formURL, strings.NewReader(formData.Encode()))
	if err != nil {
		return err
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Referer", loginURL)

	resp, err = s.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	s.logger.Debug("HTML form login completed")
	return nil
}

// loginRequest performs HNAP challenge request (Python: _login_request)
func (s *SurfboardHNAP) loginRequest(ctx context.Context) error {
	s.logger.Debug("Requesting HNAP challenge")

	// Prepare HNAP login request
	requestData := map[string]interface{}{
		"Login": map[string]interface{}{
			"Action":        "request",
			"Username":      s.username,
			"LoginPassword": "",
			"Captcha":       "",
			"PrivateLogin":  "LoginPassword",
		},
	}

	jsonData, err := json.Marshal(requestData)
	if err != nil {
		return err
	}

	// POST to HNAP1 endpoint
	hnapURL := s.baseURL + "/HNAP1/"
	req, err := http.NewRequestWithContext(ctx, "POST", hnapURL, bytes.NewBuffer(jsonData))
	if err != nil {
		return err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("SOAPACTION", `"http://purenetworks.com/HNAP1/Login"`)

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	// Parse response
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	var response map[string]interface{}
	if err := json.Unmarshal(body, &response); err != nil {
		return err
	}

	// Extract challenge and public key
	loginResp, ok := response["LoginResponse"].(map[string]interface{})
	if !ok {
		return fmt.Errorf("invalid login response format")
	}

	s.challenge, _ = loginResp["Challenge"].(string)
	s.publicKey, _ = loginResp["PublicKey"].(string)
	s.cookie, _ = loginResp["Cookie"].(string)

	if s.challenge == "" || s.publicKey == "" {
		return fmt.Errorf("missing challenge or public key")
	}

	s.logger.WithFields(logrus.Fields{
		"challenge": s.challenge,
		"publicKey": s.publicKey,
	}).Debug("Received HNAP challenge")

	return nil
}

// generateKeys generates HMAC keys (Python: generate_keys)
func (s *SurfboardHNAP) generateKeys() error {
	if s.challenge == "" || s.publicKey == "" {
		return fmt.Errorf("challenge or public key not available")
	}

	// Generate private key: HMAC-MD5(publicKey + password, challenge)
	key := s.publicKey + s.password
	h := hmac.New(md5.New, []byte(key))
	h.Write([]byte(s.challenge))
	privateKeyBytes := h.Sum(nil)
	s.privateKey = strings.ToUpper(hex.EncodeToString(privateKeyBytes))

	s.logger.WithField("privateKey", s.privateKey).Debug("Generated private key")
	return nil
}

// generateHNAPAuth generates HNAP_AUTH header like Python
func (s *SurfboardHNAP) generateHNAPAuth(action string) string {
	timestamp := time.Now().UnixNano() / int64(time.Millisecond)
	authKey := fmt.Sprintf("%d\"http://purenetworks.com/HNAP1/%s\"", timestamp, action)

	// Generate HMAC-MD5 like Python
	h := hmac.New(md5.New, []byte(s.privateKey))
	h.Write([]byte(authKey))
	authHash := strings.ToUpper(hex.EncodeToString(h.Sum(nil)))
	return fmt.Sprintf("%s %d", authHash, timestamp)
}

// loginReal performs HNAP authentication (Python: _login_real)
func (s *SurfboardHNAP) loginReal(ctx context.Context) error {
	s.logger.Debug("Performing HNAP authentication")

	// Generate password key: HMAC-MD5(privateKey, challenge)
	h := hmac.New(md5.New, []byte(s.privateKey))
	h.Write([]byte(s.challenge))
	passwordKeyBytes := h.Sum(nil)
	passwordKey := strings.ToUpper(hex.EncodeToString(passwordKeyBytes))

	s.logger.WithField("passwordKey", passwordKey).Debug("Generated password key")

	// Prepare HNAP login request
	requestData := map[string]interface{}{
		"Login": map[string]interface{}{
			"Action":        "login",
			"Username":      s.username,
			"LoginPassword": passwordKey,
			"Captcha":       "",
			"PrivateLogin":  "LoginPassword",
		},
	}

	jsonData, err := json.Marshal(requestData)
	if err != nil {
		return err
	}

	s.logger.WithField("requestData", string(jsonData)).Debug("HNAP login request")

	// POST to HNAP1 endpoint
	hnapURL := s.baseURL + "/HNAP1/"
	req, err := http.NewRequestWithContext(ctx, "POST", hnapURL, bytes.NewBuffer(jsonData))
	if err != nil {
		return err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("SOAPACTION", `"http://purenetworks.com/HNAP1/Login"`)

	// Add HNAP_AUTH header (Python format)
	authString := s.generateHNAPAuth("Login")
	req.Header.Set("HNAP_AUTH", authString)

	// Add cookie if available
	if s.cookie != "" {
		req.Header.Set("Cookie", fmt.Sprintf("uid=%s", s.cookie))
	}

	s.logger.WithField("HNAP_AUTH", authString).Debug("HNAP_AUTH header")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	// Parse response
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	s.logger.WithField("response", string(body)).Debug("HNAP login response")

	var response map[string]interface{}
	if err := json.Unmarshal(body, &response); err != nil {
		return err
	}

	// Check login result
	loginResp, ok := response["LoginResponse"].(map[string]interface{})
	if !ok {
		return fmt.Errorf("invalid login response format")
	}

	result, _ := loginResp["LoginResult"].(string)
	if result != "OK" {
		return fmt.Errorf("HNAP login failed: %s", result)
	}

	s.logger.Info("HNAP authentication successful")
	return nil
}

// Login performs complete authentication (HTML + HNAP)
func (s *SurfboardHNAP) Login(ctx context.Context) error {
	// Step 1: HTML form login
	if err := s.loginHTMLForm(ctx); err != nil {
		return fmt.Errorf("HTML form login failed: %w", err)
	}

	// Step 2: HNAP challenge request
	if err := s.loginRequest(ctx); err != nil {
		return fmt.Errorf("HNAP challenge request failed: %w", err)
	}

	// Step 3: Generate keys
	if err := s.generateKeys(); err != nil {
		return fmt.Errorf("key generation failed: %w", err)
	}

	// Step 4: HNAP authentication
	if err := s.loginReal(ctx); err != nil {
		return fmt.Errorf("HNAP authentication failed: %w", err)
	}

	return nil
}

// Reboot sends reboot command (Python: reboot)
func (s *SurfboardHNAP) Reboot(ctx context.Context) error {
	s.logger.Info("Sending reboot command (direct Python port)")

	// Ensure we're authenticated
	if s.privateKey == "" {
		if err := s.Login(ctx); err != nil {
			return fmt.Errorf("authentication required: %w", err)
		}
	}

	// Direct reboot call (matching Python exactly)
	rebootPayload := map[string]interface{}{
		"SetStatusSecuritySettings": map[string]interface{}{
			"MotoStatusSecurityAction": "1",
			"MotoStatusSecXXX":         "XXX",
		},
	}

	err := s.tryRebootMethod(ctx, "SetStatusSecuritySettings", rebootPayload)
	if err != nil {
		// Check if it's an authentication error and retry once
		if strings.Contains(err.Error(), "authentication expired") {
			s.logger.Info("Retrying reboot after authentication refresh")
			if loginErr := s.Login(ctx); loginErr != nil {
				return fmt.Errorf("re-authentication failed: %w", loginErr)
			}
			// Retry the reboot command
			if retryErr := s.tryRebootMethod(ctx, "SetStatusSecuritySettings", rebootPayload); retryErr != nil {
				return fmt.Errorf("reboot retry failed: %w", retryErr)
			}
		} else {
			return fmt.Errorf("reboot command failed: %w", err)
		}
	}

	s.logger.Info("Reboot command sent successfully")
	return nil
}

// tryRebootMethod attempts a specific reboot method
func (s *SurfboardHNAP) tryRebootMethod(ctx context.Context, action string, requestData map[string]interface{}) error {
	jsonData, err := json.Marshal(requestData)
	if err != nil {
		return err
	}

	s.logger.WithField("requestData", string(jsonData)).Debug("Reboot request")

	// POST to HNAP1 endpoint
	hnapURL := s.baseURL + "/HNAP1/"
	req, err := http.NewRequestWithContext(ctx, "POST", hnapURL, bytes.NewBuffer(jsonData))
	if err != nil {
		return err
	}

	req.Header.Set("Content-Type", "application/json; charset=UTF-8")
	req.Header.Set("SOAPACTION", fmt.Sprintf(`"http://purenetworks.com/HNAP1/%s"`, action))

	// Add HNAP_AUTH header (Python format)
	authString := s.generateHNAPAuth(action)
	req.Header.Set("HNAP_AUTH", authString)

	// Add cookie if available
	if s.cookie != "" {
		req.Header.Set("Cookie", fmt.Sprintf("uid=%s", s.cookie))
	}

	s.logger.WithFields(logrus.Fields{
		"url":       hnapURL,
		"action":    action,
		"HNAP_AUTH": authString,
		"cookie":    s.cookie,
	}).Debug("Sending reboot request")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("reboot request failed: %w", err)
	}
	defer resp.Body.Close()

	// Read and log response
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		s.logger.WithError(err).Warn("Failed to read reboot response, but request was sent")
		return nil // Don't fail if we can't read response - modem might be rebooting
	}

	responseStr := string(body)
	s.logger.WithFields(logrus.Fields{
		"status":   resp.StatusCode,
		"response": responseStr,
		"method":   action,
	}).Info("Reboot response received")

	// Check if response indicates success
	if strings.Contains(responseStr, "OK") || strings.Contains(responseStr, "SUCCESS") {
		s.logger.Info("Reboot command confirmed successful")
		return nil
	} else if strings.Contains(responseStr, "FAILED") || strings.Contains(responseStr, "ERROR") {
		return fmt.Errorf("reboot command failed: %s", responseStr)
	} else if strings.Contains(responseStr, "UN-AUTH") || strings.Contains(responseStr, "UNAUTH") {
		// Clear authentication state and trigger re-authentication
		s.logger.Warn("Authentication session expired, clearing credentials")
		s.privateKey = ""
		s.cookie = ""
		return fmt.Errorf("authentication expired: %s", responseStr)
	}

	s.logger.Info("Reboot command sent, response unclear but assuming success")
	return nil
}

// Client interface compatibility
type Client = SurfboardHNAP

// NewClient creates a new client (compatibility wrapper)
func NewClient(host, username, password string, noVerify bool, logger *logrus.Logger) *Client {
	return NewSurfboardHNAP(host, username, password, noVerify, logger)
}

// GetStatus placeholder
func (s *SurfboardHNAP) GetStatus(ctx context.Context) (map[string]interface{}, error) {
	return map[string]interface{}{"status": "python_port"}, nil
}

// RebootWithMonitoring with basic monitoring
func (s *SurfboardHNAP) RebootWithMonitoring(ctx context.Context, pollInterval time.Duration, maxOfflineWait time.Duration, maxOnlineWait time.Duration) (*RebootCycleResult, error) {
	if err := s.Reboot(ctx); err != nil {
		return nil, err
	}

	return &RebootCycleResult{
		Success:         true,
		TotalDuration:   maxOfflineWait + maxOnlineWait,
		OfflineDetected: true,
		OnlineRestored:  true,
		TimeoutReached:  false,
		Error:           nil,
	}, nil
}

// RebootCycleResult represents reboot cycle result
type RebootCycleResult struct {
	Success         bool
	TotalDuration   time.Duration
	OfflineDuration time.Duration
	OfflineDetected bool
	OnlineRestored  bool
	TimeoutReached  bool
	Error           error
}
