package system

import (
	"testing"
)

func TestNewParser(t *testing.T) {
	parser := NewParser("linux")

	if parser == nil {
		t.Fatal("Expected parser to be created")
	}

	if parser.platform != "linux" {
		t.Errorf("Expected platform to be 'linux', got '%s'", parser.platform)
	}
}

func TestParseLinuxInterfaceStatus(t *testing.T) {
	parser := NewParser("linux")

	output := `1: lo: <LOOPBACK,UP,LOWER_UP> mtu 65536 qdisc noqueue state UP mode DEFAULT group default qlen 1000
    link/loopback 00:00:00:00:00:00 brd 00:00:00:00:00:00
2: eth0: <BROADCAST,MULTICAST,UP,LOWER_UP> mtu 1500 qdisc pfifo_fast state UP mode DEFAULT group default qlen 1000
    link/ether 08:00:27:12:34:56 brd ff:ff:ff:ff:ff:ff
3: wlan0: <BROADCAST,MULTICAST> mtu 1500 qdisc noop state DOWN mode DEFAULT group default qlen 1000
    link/ether 02:00:00:00:00:00 brd ff:ff:ff:ff:ff:ff`

	interfaces, err := parser.ParseInterfaceStatus(output)

	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	if len(interfaces) != 3 {
		t.Fatalf("Expected 3 interfaces, got %d", len(interfaces))
	}

	// Check first interface (lo)
	if interfaces[0].Name != "lo" {
		t.Errorf("Expected first interface name to be 'lo', got '%s'", interfaces[0].Name)
	}
	if interfaces[0].State != "UP" {
		t.Errorf("Expected first interface state to be 'UP', got '%s'", interfaces[0].State)
	}
	if interfaces[0].MTU != 65536 {
		t.Errorf("Expected first interface MTU to be 65536, got %d", interfaces[0].MTU)
	}

	// Check second interface (eth0)
	if interfaces[1].Name != "eth0" {
		t.Errorf("Expected second interface name to be 'eth0', got '%s'", interfaces[1].Name)
	}
	if interfaces[1].State != "UP" {
		t.Errorf("Expected second interface state to be 'UP', got '%s'", interfaces[1].State)
	}
	if interfaces[1].MTU != 1500 {
		t.Errorf("Expected second interface MTU to be 1500, got %d", interfaces[1].MTU)
	}

	// Check third interface (wlan0)
	if interfaces[2].Name != "wlan0" {
		t.Errorf("Expected third interface name to be 'wlan0', got '%s'", interfaces[2].Name)
	}
	if interfaces[2].State != "DOWN" {
		t.Errorf("Expected third interface state to be 'DOWN', got '%s'", interfaces[2].State)
	}
}

func TestParseARPTable(t *testing.T) {
	parser := NewParser("linux")

	output := `gateway (192.168.1.1) at 08:00:27:12:34:56 [ether] on eth0
router (192.168.1.254) at aa:bb:cc:dd:ee:ff [ether] on eth0
? (192.168.1.100) at <incomplete> on eth0`

	entries, err := parser.ParseARPTable(output)

	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	if len(entries) != 2 { // Third entry should be filtered out due to <incomplete>
		t.Fatalf("Expected 2 entries, got %d", len(entries))
	}

	// Check first entry
	if entries[0].Hostname != "gateway" {
		t.Errorf("Expected first hostname to be 'gateway', got '%s'", entries[0].Hostname)
	}
	if entries[0].IP != "192.168.1.1" {
		t.Errorf("Expected first IP to be '192.168.1.1', got '%s'", entries[0].IP)
	}
	if entries[0].MAC != "08:00:27:12:34:56" {
		t.Errorf("Expected first MAC to be '08:00:27:12:34:56', got '%s'", entries[0].MAC)
	}

	// Check second entry
	if entries[1].Hostname != "router" {
		t.Errorf("Expected second hostname to be 'router', got '%s'", entries[1].Hostname)
	}
	if entries[1].IP != "192.168.1.254" {
		t.Errorf("Expected second IP to be '192.168.1.254', got '%s'", entries[1].IP)
	}
	if entries[1].MAC != "aa:bb:cc:dd:ee:ff" {
		t.Errorf("Expected second MAC to be 'aa:bb:cc:dd:ee:ff', got '%s'", entries[1].MAC)
	}
}

func TestParseLinuxIPAddresses(t *testing.T) {
	parser := NewParser("linux")

	output := `1: lo: <LOOPBACK,UP,LOWER_UP> mtu 65536 qdisc noqueue state UP group default qlen 1000
    link/loopback 00:00:00:00:00:00 brd 00:00:00:00:00:00
    inet 127.0.0.1/8 scope host lo
       valid_lft forever preferred_lft forever
2: eth0: <BROADCAST,MULTICAST,UP,LOWER_UP> mtu 1500 qdisc pfifo_fast state UP group default qlen 1000
    link/ether 08:00:27:12:34:56 brd ff:ff:ff:ff:ff:ff
    inet 192.168.1.100/24 brd 192.168.1.255 scope global dynamic eth0
       valid_lft 86400sec preferred_lft 86400sec`

	addresses, err := parser.ParseIPAddresses(output)

	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	// Should only get one address (127.0.0.1 is filtered out)
	if len(addresses) != 1 {
		t.Fatalf("Expected 1 address, got %d", len(addresses))
	}

	// Check the address
	if addresses[0].IP != "192.168.1.100" {
		t.Errorf("Expected IP to be '192.168.1.100', got '%s'", addresses[0].IP)
	}
	if addresses[0].CIDR != "192.168.1.100/24" {
		t.Errorf("Expected CIDR to be '192.168.1.100/24', got '%s'", addresses[0].CIDR)
	}
	if addresses[0].Network != "192.168.1.0/24" {
		t.Errorf("Expected network to be '192.168.1.0/24', got '%s'", addresses[0].Network)
	}
}

func TestParseLinuxRoutingTable(t *testing.T) {
	parser := NewParser("linux")

	output := `default via 192.168.1.1 dev eth0 proto dhcp metric 100
192.168.1.0/24 dev eth0 proto kernel scope link src 192.168.1.100 metric 100
169.254.0.0/16 dev eth0 scope link metric 1000`

	routes, defaultRoute, err := parser.ParseRoutingTable(output)

	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	if len(routes) != 3 {
		t.Fatalf("Expected 3 routes, got %d", len(routes))
	}

	if defaultRoute != "default via 192.168.1.1 dev eth0 proto dhcp metric 100" {
		t.Errorf("Expected default route to be set correctly, got '%s'", defaultRoute)
	}

	// Check that all routes have raw data
	for i, route := range routes {
		if route.Raw == "" {
			t.Errorf("Expected route %d to have raw data", i)
		}
	}
}

func TestParsePingOutputLinux(t *testing.T) {
	parser := NewParser("linux")

	output := `PING google.com (142.250.191.14) 56(84) bytes of data.
64 bytes from lga25s62-in-f14.1e100.net (142.250.191.14): icmp_seq=1 ttl=117 time=12.3 ms
64 bytes from lga25s62-in-f14.1e100.net (142.250.191.14): icmp_seq=2 ttl=117 time=11.8 ms
64 bytes from lga25s62-in-f14.1e100.net (142.250.191.14): icmp_seq=3 ttl=117 time=13.1 ms

--- google.com ping statistics ---
3 packets transmitted, 3 received, 0% packet loss, time 2003ms
rtt min/avg/max/mdev = 11.8/12.4/13.1/0.5 ms`

	stats, err := parser.ParsePingOutput(output)

	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	if stats.PacketsSent != 3 {
		t.Errorf("Expected 3 packets sent, got %d", stats.PacketsSent)
	}

	if stats.PacketsReceived != 3 {
		t.Errorf("Expected 3 packets received, got %d", stats.PacketsReceived)
	}

	if stats.PacketLoss != 0.0 {
		t.Errorf("Expected 0%% packet loss, got %.1f%%", stats.PacketLoss)
	}

	if stats.MinTime != 11.8 {
		t.Errorf("Expected min time 11.8ms, got %.1fms", stats.MinTime)
	}

	if stats.AvgTime != 12.4 {
		t.Errorf("Expected avg time 12.4ms, got %.1fms", stats.AvgTime)
	}

	if stats.MaxTime != 13.1 {
		t.Errorf("Expected max time 13.1ms, got %.1fms", stats.MaxTime)
	}

	if stats.StdDev != 0.5 {
		t.Errorf("Expected std dev 0.5ms, got %.1fms", stats.StdDev)
	}
}

func TestParseLinuxPingOutput(t *testing.T) {
	parser := NewParser("linux")

	output := `
Pinging google.com [142.250.191.14] with 32 bytes of data:
Reply from 142.250.191.14: bytes=32 time=12ms TTL=117
Reply from 142.250.191.14: bytes=32 time=11ms TTL=117
Reply from 142.250.191.14: bytes=32 time=13ms TTL=117
Reply from 142.250.191.14: bytes=32 time=12ms TTL=117

Ping statistics for 142.250.191.14:
    Packets: Sent = 4, Received = 4, Lost = 0 (0% loss),
Approximate round trip times in milli-seconds:
    Minimum = 11ms, Maximum = 13ms, Average = 12ms`

	stats, err := parser.ParsePingOutput(output)

	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	if stats.PacketsSent != 4 {
		t.Errorf("Expected 4 packets sent, got %d", stats.PacketsSent)
	}

	if stats.PacketsReceived != 4 {
		t.Errorf("Expected 4 packets received, got %d", stats.PacketsReceived)
	}

	if stats.PacketLoss != 0.0 {
		t.Errorf("Expected 0%% packet loss, got %.1f%%", stats.PacketLoss)
	}

	if stats.MinTime != 11.0 {
		t.Errorf("Expected min time 11ms, got %.1fms", stats.MinTime)
	}

	if stats.AvgTime != 12.0 {
		t.Errorf("Expected avg time 12ms, got %.1fms", stats.AvgTime)
	}

	if stats.MaxTime != 13.0 {
		t.Errorf("Expected max time 13ms, got %.1fms", stats.MaxTime)
	}
}

func TestParseEmptyOutput(t *testing.T) {
	parser := NewParser("linux")

	// Test empty interface status
	interfaces, err := parser.ParseInterfaceStatus("")
	if err != nil {
		t.Errorf("Expected no error for empty interface status, got %v", err)
	}
	if len(interfaces) != 0 {
		t.Errorf("Expected 0 interfaces for empty output, got %d", len(interfaces))
	}

	// Test empty ARP table
	entries, err := parser.ParseARPTable("")
	if err != nil {
		t.Errorf("Expected no error for empty ARP table, got %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("Expected 0 ARP entries for empty output, got %d", len(entries))
	}

	// Test empty IP addresses
	addresses, err := parser.ParseIPAddresses("")
	if err != nil {
		t.Errorf("Expected no error for empty IP addresses, got %v", err)
	}
	if len(addresses) != 0 {
		t.Errorf("Expected 0 IP addresses for empty output, got %d", len(addresses))
	}

	// Test empty routing table
	routes, defaultRoute, err := parser.ParseRoutingTable("")
	if err != nil {
		t.Errorf("Expected no error for empty routing table, got %v", err)
	}
	if len(routes) != 0 {
		t.Errorf("Expected 0 routes for empty output, got %d", len(routes))
	}
	if defaultRoute != "" {
		t.Errorf("Expected empty default route for empty output, got '%s'", defaultRoute)
	}

	// Test empty ping output
	stats, err := parser.ParsePingOutput("")
	if err != nil {
		t.Errorf("Expected no error for empty ping output, got %v", err)
	}
	if stats == nil {
		t.Error("Expected ping stats to be non-nil")
	}
}

func TestUnsupportedPlatform(t *testing.T) {
	parser := NewParser("unsupported")

	// Should return empty results for unsupported platforms
	interfaces, err := parser.ParseInterfaceStatus("some output")
	if err != nil {
		t.Errorf("Expected no error for unsupported platform, got %v", err)
	}
	if interfaces != nil && len(interfaces) != 0 {
		t.Errorf("Expected empty interfaces for unsupported platform, got %d", len(interfaces))
	}
}
