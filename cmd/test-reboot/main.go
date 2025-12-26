package main

import (
	"context"
	"crypto/tls"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
)

func main() {
	var (
		host     = flag.String("host", "192.168.100.1", "Modem host")
		username = flag.String("username", "admin", "Modem username")
		password = flag.String("password", "motorola", "Modem password")
		timeout  = flag.Duration("timeout", 120*time.Second, "Request timeout")
	)
	flag.Parse()

	// Setup logger
	logger := logrus.New()
	logger.SetLevel(logrus.DebugLevel)

	fmt.Printf("Testing modem reboot with:\n")
	fmt.Printf("  Host: %s\n", *host)
	fmt.Printf("  Username: %s\n", *username)
	fmt.Printf("  Password: %s\n", *password)
	fmt.Printf("  Timeout: %v\n", *timeout)
	fmt.Println()

	// Create HTTP client with no SSL verification
	client := &http.Client{
		Timeout: *timeout,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: true,
			},
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()

	// Test 1: Basic HTTPS connectivity
	fmt.Println("=== Test 1: Basic HTTPS Connectivity ===")
	err := testHTTPS(ctx, client, *host)
	if err != nil {
		fmt.Printf("HTTPS test failed: %v\n", err)
		
		// Test HTTP fallback
		fmt.Println("\n=== Test 1b: HTTP Fallback ===")
		err = testHTTP(ctx, client, *host)
		if err != nil {
			fmt.Printf("HTTP fallback failed: %v\n", err)
		} else {
			fmt.Println("HTTP fallback succeeded")
		}
	} else {
		fmt.Println("HTTPS connectivity succeeded")
	}

	// Test 2: HTML Form Login
	fmt.Println("\n=== Test 2: HTML Form Login ===")
	err = testHTMLFormLogin(ctx, client, *host, *username, *password)
	if err != nil {
		fmt.Printf("HTML form login failed: %v\n", err)
	} else {
		fmt.Println("HTML form login succeeded")
	}

	// Test 3: Direct reboot via web form
	fmt.Println("\n=== Test 3: Direct Web Reboot ===")
	err = testWebReboot(ctx, client, *host, *username, *password)
	if err != nil {
		fmt.Printf("Web reboot failed: %v\n", err)
	} else {
		fmt.Println("Web reboot succeeded")
	}
}

func testHTTPS(ctx context.Context, client *http.Client, host string) error {
	url := fmt.Sprintf("https://%s/", host)
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return err
	}

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	fmt.Printf("HTTPS Status: %d\n", resp.StatusCode)
	return nil
}

func testHTTP(ctx context.Context, client *http.Client, host string) error {
	url := fmt.Sprintf("http://%s/", host)
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return err
	}

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	fmt.Printf("HTTP Status: %d\n", resp.StatusCode)
	return nil
}

func testHTMLFormLogin(ctx context.Context, client *http.Client, host, username, password string) error {
	// Try HTTPS first
	baseURLs := []string{
		fmt.Sprintf("https://%s", host),
		fmt.Sprintf("http://%s", host),
	}

	for _, baseURL := range baseURLs {
		fmt.Printf("Trying HTML login with %s\n", baseURL)
		
		// Get login page
		loginURL := baseURL + "/Login.html"
		req, err := http.NewRequestWithContext(ctx, "GET", loginURL, nil)
		if err != nil {
			continue
		}

		resp, err := client.Do(req)
		if err != nil {
			fmt.Printf("  Login page failed: %v\n", err)
			continue
		}
		resp.Body.Close()
		fmt.Printf("  Login page status: %d\n", resp.StatusCode)

		// Submit login form
		formData := url.Values{}
		formData.Set("loginUsername", username)
		formData.Set("loginPassword", password)

		formURL := baseURL + "/cgi-bin/moto/goform/MotoLogin"
		req, err = http.NewRequestWithContext(ctx, "POST", formURL, strings.NewReader(formData.Encode()))
		if err != nil {
			continue
		}

		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set("Referer", loginURL)

		resp, err = client.Do(req)
		if err != nil {
			fmt.Printf("  Form submit failed: %v\n", err)
			continue
		}
		defer resp.Body.Close()

		fmt.Printf("  Form submit status: %d\n", resp.StatusCode)
		
		// Read response body for debugging
		body, _ := io.ReadAll(resp.Body)
		fmt.Printf("  Response length: %d bytes\n", len(body))
		
		return nil
	}

	return fmt.Errorf("all login attempts failed")
}

func testWebReboot(ctx context.Context, client *http.Client, host, username, password string) error {
	// First login
	err := testHTMLFormLogin(ctx, client, host, username, password)
	if err != nil {
		return fmt.Errorf("login required for reboot: %w", err)
	}

	// Try different reboot methods
	rebootMethods := []struct {
		name string
		url  string
		data url.Values
	}{
		{
			name: "MotoReboot",
			url:  fmt.Sprintf("https://%s/cgi-bin/moto/goform/MotoReboot", host),
			data: url.Values{"reboot": {"1"}, "action": {"reboot"}},
		},
		{
			name: "MotoStatusSecurity", 
			url:  fmt.Sprintf("https://%s/cgi-bin/moto/goform/MotoStatusSecurity", host),
			data: url.Values{"MotoStatusSecurityAction": {"1"}},
		},
		{
			name: "Direct Reboot Form",
			url:  fmt.Sprintf("https://%s/cgi-bin/moto/goform/MotoReboot", host),
			data: url.Values{"RestartCM": {"Restart Cable Modem"}},
		},
	}

	for _, method := range rebootMethods {
		fmt.Printf("Trying %s with %s\n", method.name, method.url)
		
		req, err := http.NewRequestWithContext(ctx, "POST", method.url, strings.NewReader(method.data.Encode()))
		if err != nil {
			fmt.Printf("  Request creation failed: %v\n", err)
			continue
		}

		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set("Referer", fmt.Sprintf("https://%s/", host))

		resp, err := client.Do(req)
		if err != nil {
			fmt.Printf("  Request failed: %v\n", err)
			continue
		}
		defer resp.Body.Close()

		fmt.Printf("  Status: %d\n", resp.StatusCode)
		
		// Read and analyze response
		body, _ := io.ReadAll(resp.Body)
		bodyStr := string(body)
		fmt.Printf("  Response length: %d bytes\n", len(body))
		
		// Check for success indicators
		if strings.Contains(bodyStr, "reboot") || strings.Contains(bodyStr, "restart") || 
		   strings.Contains(bodyStr, "success") || len(bodyStr) < 100 {
			fmt.Printf("  Response content: %q\n", bodyStr)
		}
		
		// Try HTTP fallback
		httpURL := strings.Replace(method.url, "https://", "http://", 1)
		fmt.Printf("Trying %s with HTTP fallback: %s\n", method.name, httpURL)
		
		req, err = http.NewRequestWithContext(ctx, "POST", httpURL, strings.NewReader(method.data.Encode()))
		if err != nil {
			continue
		}

		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set("Referer", fmt.Sprintf("http://%s/", host))

		resp, err = client.Do(req)
		if err != nil {
			fmt.Printf("  HTTP fallback failed: %v\n", err)
			continue
		}
		defer resp.Body.Close()

		fmt.Printf("  HTTP Status: %d\n", resp.StatusCode)
		body, _ = io.ReadAll(resp.Body)
		fmt.Printf("  HTTP Response: %q\n", string(body))
	}

	return nil
}
