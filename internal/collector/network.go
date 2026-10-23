package collector

import (
	"bufio"
	"bytes"
	"context"
	"encoding/hex"
	"fmt"
	"net"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"coreip-server-inspector/internal/models"
)

type netSample struct {
	rxBytes   uint64
	txBytes   uint64
	timestamp time.Time
}

// NetworkCollector gathers network interfaces, IPs, rates, gateway, and DNS servers.
type NetworkCollector struct {
	fs          FSReader
	mu          sync.Mutex
	lastSamples map[string]netSample
}

// NewNetworkCollector creates a NetworkCollector.
func NewNetworkCollector(fs FSReader) *NetworkCollector {
	return &NetworkCollector{
		fs:          fs,
		lastSamples: make(map[string]netSample),
	}
}

// Collect reads network interfaces, counters, routing, and DNS configuration.
func (c *NetworkCollector) Collect(ctx context.Context) (models.NetworkInfo, error) {
	var info models.NetworkInfo

	// 1. Collect Interfaces
	info.Interfaces = c.collectInterfaces()

	// 2. Default Gateway from /proc/net/route
	info.DefaultGateway = c.collectDefaultGateway()

	// 3. DNS Servers from /etc/resolv.conf
	info.DNSServers = c.collectDNSServers()

	return info, nil
}

func (c *NetworkCollector) collectInterfaces() []models.NetworkInterface {
	var interfaces []models.NetworkInterface

	// Map IP addresses using standard library
	ipMap := make(map[string]struct {
		ipv4 []string
		ipv6 []string
	})
	if netIfaces, err := net.Interfaces(); err == nil {
		for _, iface := range netIfaces {
			addrs, err := iface.Addrs()
			if err != nil {
				continue
			}
			var v4, v6 []string
			for _, addr := range addrs {
				ip, _, err := net.ParseCIDR(addr.String())
				if err != nil {
					continue
				}
				if ip.To4() != nil {
					v4 = append(v4, ip.String())
				} else if ip.To16() != nil {
					v6 = append(v6, ip.String())
				}
			}
			ipMap[iface.Name] = struct {
				ipv4 []string
				ipv6 []string
			}{ipv4: v4, ipv6: v6}
		}
	}

	netDir := "/sys/class/net"
	if !c.fs.Exists(netDir) {
		// Non-Linux or fallback: synthesize from net.Interfaces()
		for name, ips := range ipMap {
			interfaces = append(interfaces, models.NetworkInterface{
				Name:          name,
				State:         "up",
				SpeedMbps:     1000,
				Duplex:        "full",
				IPv4Addresses: ips.ipv4,
				IPv6Addresses: ips.ipv6,
			})
		}
		return interfaces
	}

	entries, err := c.fs.ReadDir(netDir)
	if err != nil {
		return interfaces
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	now := time.Now()

	for _, entry := range entries {
		name := entry.Name()
		if name == "" {
			continue
		}

		ifPath := filepath.Join(netDir, name)

		mac := SafeReadString(c.fs, filepath.Join(ifPath, "address"))
		operstate := SafeReadString(c.fs, filepath.Join(ifPath, "operstate"))
		if operstate == "" {
			operstate = "unknown"
		}

		mtu := ParseInt(SafeReadString(c.fs, filepath.Join(ifPath, "mtu")))
		speed := int64(ParseInt(SafeReadString(c.fs, filepath.Join(ifPath, "speed"))))
		if speed <= 0 {
			speed = -1
		}
		duplex := SafeReadString(c.fs, filepath.Join(ifPath, "duplex"))
		if duplex == "" {
			duplex = "unknown"
		}

		// Statistics
		statDir := filepath.Join(ifPath, "statistics")
		rxBytes := ParseUint(SafeReadString(c.fs, filepath.Join(statDir, "rx_bytes")))
		txBytes := ParseUint(SafeReadString(c.fs, filepath.Join(statDir, "tx_bytes")))
		rxPackets := ParseUint(SafeReadString(c.fs, filepath.Join(statDir, "rx_packets")))
		txPackets := ParseUint(SafeReadString(c.fs, filepath.Join(statDir, "tx_packets")))
		rxErrors := ParseUint(SafeReadString(c.fs, filepath.Join(statDir, "rx_errors")))
		txErrors := ParseUint(SafeReadString(c.fs, filepath.Join(statDir, "tx_errors")))
		rxDropped := ParseUint(SafeReadString(c.fs, filepath.Join(statDir, "rx_dropped")))
		txDropped := ParseUint(SafeReadString(c.fs, filepath.Join(statDir, "tx_dropped")))

		// Traffic rate calculation
		var rxRate, txRate float64
		last, exists := c.lastSamples[name]
		if exists && !last.timestamp.IsZero() {
			dt := now.Sub(last.timestamp).Seconds()
			if dt > 0.1 && rxBytes >= last.rxBytes && txBytes >= last.txBytes {
				rxRate = float64(rxBytes-last.rxBytes) / dt
				txRate = float64(txBytes-last.txBytes) / dt
			}
		}
		c.lastSamples[name] = netSample{
			rxBytes:   rxBytes,
			txBytes:   txBytes,
			timestamp: now,
		}

		ips := ipMap[name]

		interfaces = append(interfaces, models.NetworkInterface{
			Name:              name,
			MAC:               mac,
			State:             operstate,
			MTU:               mtu,
			SpeedMbps:         speed,
			Duplex:            duplex,
			IPv4Addresses:     ips.ipv4,
			IPv6Addresses:     ips.ipv6,
			RxBytes:           rxBytes,
			TxBytes:           txBytes,
			RxPackets:         rxPackets,
			TxPackets:         txPackets,
			RxErrors:          rxErrors,
			TxErrors:          txErrors,
			RxDropped:         rxDropped,
			TxDropped:         txDropped,
			RxRateBytesPerSec: rxRate,
			TxRateBytesPerSec: txRate,
		})
	}

	return interfaces
}

func (c *NetworkCollector) collectDefaultGateway() string {
	routePath := "/proc/net/route"
	if !c.fs.Exists(routePath) {
		return ""
	}

	data, err := c.fs.ReadFile(routePath)
	if err != nil {
		return ""
	}

	scanner := bufio.NewScanner(bytes.NewReader(data))
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) < 3 {
			continue
		}
		dest := fields[1]
		gwHex := fields[2]

		// Default route destination is 00000000
		if dest == "00000000" && gwHex != "00000000" && len(gwHex) == 8 {
			ipBytes, err := hex.DecodeString(gwHex)
			if err == nil && len(ipBytes) == 4 {
				// /proc/net/route stores IPv4 in little-endian order
				return fmt.Sprintf("%d.%d.%d.%d", ipBytes[3], ipBytes[2], ipBytes[1], ipBytes[0])
			}
		}
	}

	return ""
}

func (c *NetworkCollector) collectDNSServers() []string {
	var dns []string
	resolvPath := "/etc/resolv.conf"
	if !c.fs.Exists(resolvPath) {
		return dns
	}

	data, err := c.fs.ReadFile(resolvPath)
	if err != nil {
		return dns
	}

	scanner := bufio.NewScanner(bytes.NewReader(data))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "nameserver ") {
			parts := strings.Fields(line)
			if len(parts) >= 2 {
				dns = append(dns, parts[1])
			}
		}
	}

	return dns
}

