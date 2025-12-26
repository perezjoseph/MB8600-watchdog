package system

import (
	"net"
	"regexp"
	"strconv"
	"strings"
)

// InterfaceInfo represents network interface information
type InterfaceInfo struct {
	Name  string `json:"name"`
	State string `json:"state"`
	MAC   string `json:"mac,omitempty"`
	MTU   int    `json:"mtu,omitempty"`
}

// ARPEntry represents an ARP table entry
type ARPEntry struct {
	Hostname string `json:"hostname,omitempty"`
	IP       string `json:"ip"`
	MAC      string `json:"mac"`
	Raw      string `json:"raw"`
}

// IPAddress represents an IP address configuration
type IPAddress struct {
	IP      string `json:"ip"`
	CIDR    string `json:"cidr"`
	Network string `json:"network"`
	Raw     string `json:"raw"`
}

// Route represents a routing table entry
type Route struct {
	Destination string `json:"destination"`
	Gateway     string `json:"gateway,omitempty"`
	Interface   string `json:"interface,omitempty"`
	Metric      int    `json:"metric,omitempty"`
	Raw         string `json:"raw"`
}

// PingStats represents ping statistics
type PingStats struct {
	PacketsSent     int     `json:"packets_sent"`
	PacketsReceived int     `json:"packets_received"`
	PacketLoss      float64 `json:"packet_loss_percent"`
	MinTime         float64 `json:"min_time_ms"`
	AvgTime         float64 `json:"avg_time_ms"`
	MaxTime         float64 `json:"max_time_ms"`
	StdDev          float64 `json:"std_dev_ms,omitempty"`
}

// Parser provides parsing utilities for network command outputs
type Parser struct {
	platform string
}

// NewParser creates a new command output parser
func NewParser(platform string) *Parser {
	return &Parser{
		platform: platform,
	}
}

// ParseInterfaceStatus parses network interface status output
func (p *Parser) ParseInterfaceStatus(output string) ([]InterfaceInfo, error) {
	// Linux-only parsing
	return p.parseLinuxInterfaceStatus(output)
}

// parseLinuxInterfaceStatus parses 'ip link show' output on Linux
func (p *Parser) parseLinuxInterfaceStatus(output string) ([]InterfaceInfo, error) {
	var interfaces []InterfaceInfo

	lines := strings.Split(output, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.Contains(line, ":") && (strings.Contains(line, "state UP") || strings.Contains(line, "state DOWN")) {
			// Parse interface line: "2: eth0: <BROADCAST,MULTICAST,UP,LOWER_UP> mtu 1500 qdisc pfifo_fast state UP mode DEFAULT group default qlen 1000"
			parts := strings.Fields(line)
			if len(parts) >= 2 {
				name := strings.TrimSuffix(parts[1], ":")
				state := "DOWN"
				mtu := 0

				// Find state and MTU
				for i, part := range parts {
					if part == "state" && i+1 < len(parts) {
						state = parts[i+1]
					}
					if part == "mtu" && i+1 < len(parts) {
						if mtuVal, err := strconv.Atoi(parts[i+1]); err == nil {
							mtu = mtuVal
						}
					}
				}

				interfaces = append(interfaces, InterfaceInfo{
					Name:  name,
					State: state,
					MTU:   mtu,
				})
			}
		}
	}

	return interfaces, nil
}

// parseDarwinInterfaceStatus parses 'ifconfig' output on macOS
func (p *Parser) parseDarwinInterfaceStatus(output string) ([]InterfaceInfo, error) {
	var interfaces []InterfaceInfo

	// Split by interface blocks (lines starting with interface name)
	lines := strings.Split(output, "\n")
	var currentInterface *InterfaceInfo

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		// Check if this is a new interface line (doesn't start with whitespace in original)
		if !strings.HasPrefix(line, " ") && !strings.HasPrefix(line, "\t") && strings.Contains(line, ":") {
			// Save previous interface
			if currentInterface != nil {
				interfaces = append(interfaces, *currentInterface)
			}

			// Parse new interface: "en0: flags=8863<UP,BROADCAST,SMART,RUNNING,SIMPLEX,MULTICAST> mtu 1500"
			parts := strings.Fields(line)
			if len(parts) >= 1 {
				name := strings.TrimSuffix(parts[0], ":")
				state := "DOWN"
				mtu := 0

				if strings.Contains(line, "UP") {
					state = "UP"
				}

				// Find MTU
				for i, part := range parts {
					if part == "mtu" && i+1 < len(parts) {
						if mtuVal, err := strconv.Atoi(parts[i+1]); err == nil {
							mtu = mtuVal
						}
					}
				}

				currentInterface = &InterfaceInfo{
					Name:  name,
					State: state,
					MTU:   mtu,
				}
			}
		}
	}

	// Don't forget the last interface
	if currentInterface != nil {
		interfaces = append(interfaces, *currentInterface)
	}

	return interfaces, nil
}

// ParseARPTable parses ARP table output
func (p *Parser) ParseARPTable(output string) ([]ARPEntry, error) {
	var entries []ARPEntry

	lines := strings.Split(output, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		// Parse Linux ARP: "gateway (192.168.1.1) at aa:bb:cc:dd:ee:ff [ether] on eth0"
		if strings.Contains(line, "(") && strings.Contains(line, ")") && strings.Contains(line, "at") {
			re := regexp.MustCompile(`^(\S+)\s+\(([^)]+)\)\s+at\s+([a-fA-F0-9:]+)`)
			matches := re.FindStringSubmatch(line)

			if len(matches) >= 4 {
				entries = append(entries, ARPEntry{
					Hostname: matches[1],
					IP:       matches[2],
					MAC:      matches[3],
					Raw:      line,
				})
			}
		}
	}

	return entries, nil
}

// ParseIPAddresses parses IP address configuration output
func (p *Parser) ParseIPAddresses(output string) ([]IPAddress, error) {
	// Linux-only parsing
	return p.parseLinuxIPAddresses(output)
}

// parseLinuxIPAddresses parses 'ip addr show' output
func (p *Parser) parseLinuxIPAddresses(output string) ([]IPAddress, error) {
	var addresses []IPAddress

	lines := strings.Split(output, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.Contains(line, "inet ") && !strings.Contains(line, "127.0.0.1") {
			// Parse inet line: "inet 192.168.1.100/24 brd 192.168.1.255 scope global dynamic eth0"
			parts := strings.Fields(line)
			if len(parts) >= 2 {
				cidr := parts[1]
				ip, network, err := net.ParseCIDR(cidr)
				if err == nil {
					addresses = append(addresses, IPAddress{
						IP:      ip.String(),
						CIDR:    cidr,
						Network: network.String(),
						Raw:     line,
					})
				}
			}
		}
	}

	return addresses, nil
}

// parseDarwinIPAddresses parses 'ifconfig' output on macOS
func (p *Parser) parseDarwinIPAddresses(output string) ([]IPAddress, error) {
	var addresses []IPAddress

	lines := strings.Split(output, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "inet ") && !strings.Contains(line, "127.0.0.1") {
			// Parse inet line: "inet 192.168.1.100 netmask 0xffffff00 broadcast 192.168.1.255"
			parts := strings.Fields(line)
			if len(parts) >= 2 {
				ip := parts[1]
				if net.ParseIP(ip) != nil {
					addresses = append(addresses, IPAddress{
						IP:  ip,
						Raw: line,
					})
				}
			}
		}
	}

	return addresses, nil
}

// ParseRoutingTable parses routing table output
func (p *Parser) ParseRoutingTable(output string) ([]Route, string, error) {
	// Linux-only parsing
	return p.parseLinuxRoutingTable(output)
}

// parseLinuxRoutingTable parses 'ip route show' output
func (p *Parser) parseLinuxRoutingTable(output string) ([]Route, string, error) {
	var routes []Route
	var defaultRoute string

	lines := strings.Split(output, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		routes = append(routes, Route{
			Raw: line,
		})

		if strings.HasPrefix(line, "default") {
			defaultRoute = line
		}
	}

	return routes, defaultRoute, nil
}

// parseDarwinRoutingTable parses 'netstat -rn' output on macOS
func (p *Parser) parseDarwinRoutingTable(output string) ([]Route, string, error) {
	var routes []Route
	var defaultRoute string

	lines := strings.Split(output, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "Destination") {
			continue
		}

		parts := strings.Fields(line)
		if len(parts) >= 2 {
			destination := parts[0]
			gateway := parts[1]

			route := Route{
				Destination: destination,
				Gateway:     gateway,
				Raw:         line,
			}

			routes = append(routes, route)

			if destination == "default" || destination == "0.0.0.0" {
				defaultRoute = line
			}
		}
	}

	return routes, defaultRoute, nil
}

// parseWindowsRoutingTable parses 'route print' output on Windows
func (p *Parser) parseWindowsRoutingTable(output string) ([]Route, string, error) {
	var routes []Route
	var defaultRoute string

	lines := strings.Split(output, "\n")
	inRouteTable := false

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		// Look for the IPv4 route table section
		if strings.Contains(line, "IPv4 Route Table") {
			inRouteTable = true
			continue
		}

		if inRouteTable && strings.Contains(line, "Network Destination") {
			continue
		}

		if inRouteTable {
			parts := strings.Fields(line)
			if len(parts) >= 4 {
				destination := parts[0]
				gateway := parts[2]

				route := Route{
					Destination: destination,
					Gateway:     gateway,
					Raw:         line,
				}

				routes = append(routes, route)

				if destination == "0.0.0.0" {
					defaultRoute = line
				}
			}
		}
	}

	return routes, defaultRoute, nil
}

// ParsePingOutput parses ping command output to extract statistics
func (p *Parser) ParsePingOutput(output string) (*PingStats, error) {
	stats := &PingStats{}

	lines := strings.Split(output, "\n")

	for _, line := range lines {
		line = strings.TrimSpace(line)

		// Look for packet loss line: "3 packets transmitted, 3 received, 0% packet loss, time 2003ms"
		if strings.Contains(line, "packet loss") {
			parts := strings.Fields(line)
			for i, part := range parts {
				if part == "packets" && i > 0 {
					if sent, err := strconv.Atoi(parts[i-1]); err == nil {
						stats.PacketsSent = sent
					}
				}
				if part == "received," && i > 0 {
					if received, err := strconv.Atoi(parts[i-1]); err == nil {
						stats.PacketsReceived = received
					}
				}
				if strings.HasSuffix(part, "%") && strings.Contains(line, "packet loss") {
					if lossStr := strings.TrimSuffix(part, "%"); lossStr != "" {
						if loss, err := strconv.ParseFloat(lossStr, 64); err == nil {
							stats.PacketLoss = loss
						}
					}
				}
			}
		}

		// Look for timing line: "rtt min/avg/max/mdev = 1.234/2.345/3.456/0.123 ms"
		if strings.Contains(line, "rtt") && strings.Contains(line, "=") {
			parts := strings.Split(line, "=")
			if len(parts) >= 2 {
				timings := strings.TrimSpace(parts[1])
				timings = strings.TrimSuffix(timings, " ms")
				timingParts := strings.Split(timings, "/")
				if len(timingParts) >= 3 {
					if min, err := strconv.ParseFloat(timingParts[0], 64); err == nil {
						stats.MinTime = min
					}
					if avg, err := strconv.ParseFloat(timingParts[1], 64); err == nil {
						stats.AvgTime = avg
					}
					if max, err := strconv.ParseFloat(timingParts[2], 64); err == nil {
						stats.MaxTime = max
					}
					if len(timingParts) >= 4 {
						if stddev, err := strconv.ParseFloat(timingParts[3], 64); err == nil {
							stats.StdDev = stddev
						}
					}
				}
			}
		}
	}

	return stats, nil
}
