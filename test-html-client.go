package main

import (
	"context"
	"flag"
	"fmt"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/perezjoseph/mb8600-watchdog/internal/hnap"
)

func main() {
	var (
		host     = flag.String("host", "192.168.100.1", "Modem host")
		username = flag.String("username", "admin", "Modem username")
		password = flag.String("password", "motorola", "Modem password")
		timeout  = flag.Duration("timeout", 60*time.Second, "Request timeout")
	)
	flag.Parse()

	// Setup logger
	logger := logrus.New()
	logger.SetLevel(logrus.DebugLevel)

	fmt.Printf("Testing HTML-only modem client:\n")
	fmt.Printf("  Host: %s\n", *host)
	fmt.Printf("  Username: %s\n", *username)
	fmt.Printf("  Password: %s\n", *password)
	fmt.Printf("  Timeout: %v\n", *timeout)
	fmt.Println()

	// Create HTML-only client
	client := hnap.NewClient(*host, *username, *password, true, logger)
	if client == nil {
		fmt.Println("❌ Failed to create client")
		return
	}
	fmt.Println("✅ Client created successfully")

	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()

	// Test 1: Login
	fmt.Println("\n=== Test 1: HTML Form Login ===")
	err := client.Login(ctx)
	if err != nil {
		fmt.Printf("❌ Login failed: %v\n", err)
		return
	}
	fmt.Println("✅ Login succeeded")

	// Test 2: Reboot
	fmt.Println("\n=== Test 2: HTML Form Reboot ===")
	err = client.Reboot(ctx)
	if err != nil {
		fmt.Printf("❌ Reboot failed: %v\n", err)
		return
	}
	fmt.Println("✅ Reboot command sent successfully")

	// Test 3: Status check
	fmt.Println("\n=== Test 3: Status Check ===")
	status, err := client.GetStatus(ctx)
	if err != nil {
		fmt.Printf("❌ Status check failed: %v\n", err)
		return
	}
	fmt.Printf("✅ Status: %v\n", status)

	fmt.Println("\n🎉 All tests passed! HTML-only client is working.")
}
