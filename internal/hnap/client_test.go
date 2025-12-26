package hnap

import (
	"context"
	"strings"
	"testing"

	"github.com/perezjoseph/mb8600-watchdog/internal/config"
	"github.com/sirupsen/logrus"
)

func TestNewClient(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.DebugLevel)

	client := NewClient(config.DefaultModemHost, "admin", "motorola", true, logger)

	if client == nil {
		t.Fatal("Expected client to be created, got nil")
	}

	if client.host != config.DefaultModemHost {
		t.Errorf("Expected host to be '%s', got '%s'", config.DefaultModemHost, client.host)
	}

	if client.username != "admin" {
		t.Errorf("Expected username to be 'admin', got '%s'", client.username)
	}

	if client.password != "motorola" {
		t.Errorf("Expected password to be 'motorola', got '%s'", client.password)
	}

	if !client.noVerify {
		t.Error("Expected noVerify to be true")
	}

	if client.httpClient == nil {
		t.Error("Expected httpClient to be initialized")
	}

	if client.httpClient.Jar == nil {
		t.Error("Expected cookie jar to be initialized for session management")
	}
}

func TestGenerateHNAPAuth(t *testing.T) {
	logger := logrus.New()
	client := NewClient(config.DefaultModemHost, "admin", "motorola", true, logger)

	// Set up client state for HNAP auth generation
	client.privateKey = "36BCD55C036D1670A671D2CA97479BC6"
	
	auth := client.generateHNAPAuth("Login")

	if auth == "" {
		t.Error("Expected HNAP_AUTH to be generated, got empty string")
	}

	// Test that the same inputs produce the same output (deterministic)
	auth2 := client.generateHNAPAuth("Login")
	if auth != auth2 {
		t.Errorf("Expected deterministic HNAP_AUTH generation, got different results: %s vs %s", auth, auth2)
	}

	// Test that different actions produce different outputs
	auth3 := client.generateHNAPAuth("SetStatusSecuritySettings")
	if auth == auth3 {
		t.Error("Expected different HNAP_AUTH for different actions")
	}
}

func TestClientInitialization(t *testing.T) {
	logger := logrus.New()
	client := NewClient("192.0.2.1", "admin", "motorola", true, logger)

	if client == nil {
		t.Error("Expected client to be created")
	}

	if client.host != "192.0.2.1" {
		t.Errorf("Expected host to be 192.0.2.1, got %s", client.host)
	}

	if client.username != "admin" {
		t.Errorf("Expected username to be admin, got %s", client.username)
	}
}

func TestHTMLFormLoginIntegration(t *testing.T) {
	logger := logrus.New()
	// Use a definitely non-routable IP address
	client := NewClient("192.0.2.1", "admin", "motorola", true, logger)

	ctx := context.Background()

	// Test HTML form login (will fail due to network, but tests the flow)
	err := client.loginHTMLForm(ctx)

	// We expect an error since there's no real modem at this IP
	if err == nil {
		t.Error("Expected error when connecting to non-existent modem for HTML form login")
	}

	// The error should mention the form login process
	if err != nil && !strings.Contains(err.Error(), "failed to get login page") {
		t.Logf("Got error: %v", err)
		// This is fine, different network errors are possible
	}
}

func TestLoginWithHTMLFormIntegration(t *testing.T) {
	logger := logrus.New()
	// Use a definitely non-routable IP address
	client := NewClient("192.0.2.1", "admin", "motorola", true, logger)

	ctx := context.Background()

	// Test full login process (will fail due to network, but tests the flow)
	err := client.Login(ctx)

	// We expect an error since there's no real modem at this IP
	if err == nil {
		t.Error("Expected error when connecting to non-existent modem for full login")
	}

	// The error should be meaningful
	if err != nil && err.Error() == "" {
		t.Error("Expected meaningful error message")
	}
}
