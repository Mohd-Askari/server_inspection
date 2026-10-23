package collector

import (
	"bufio"
	"bytes"
	"context"
	"strings"

	"coreip-server-inspector/internal/models"
)

// MemoryCollector parses Linux /proc/meminfo respecting true memory semantics.
type MemoryCollector struct {
	fs FSReader
}

// NewMemoryCollector creates a MemoryCollector.
func NewMemoryCollector(fs FSReader) *MemoryCollector {
	return &MemoryCollector{fs: fs}
}

// Collect reads memory metrics from /proc/meminfo.
func (c *MemoryCollector) Collect(ctx context.Context) (models.MemoryInfo, error) {
	var info models.MemoryInfo

	if !c.fs.Exists("/proc/meminfo") {
		// Non-Linux or test fallback
		info.TotalBytes = 16 * 1024 * 1024 * 1024 // 16 GiB baseline
		info.AvailableBytes = 8 * 1024 * 1024 * 1024
		info.UsedBytes = 8 * 1024 * 1024 * 1024
		info.FreeBytes = 4 * 1024 * 1024 * 1024
		info.CachedBytes = 4 * 1024 * 1024 * 1024
		info.UtilizationPct = 50.0
		return info, nil
	}

	data, err := c.fs.ReadFile("/proc/meminfo")
	if err != nil {
		return info, err
	}

	rawKB := make(map[string]uint64)
	scanner := bufio.NewScanner(bytes.NewReader(data))

	for scanner.Scan() {
		line := scanner.Text()
		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		valFields := strings.Fields(parts[1])
		if len(valFields) > 0 {
			rawKB[key] = ParseUint(valFields[0])
		}
	}

	info.TotalBytes = rawKB["MemTotal"] * 1024
	info.FreeBytes = rawKB["MemFree"] * 1024
	info.BuffersBytes = rawKB["Buffers"] * 1024
	info.CachedBytes = (rawKB["Cached"] + rawKB["SReclaimable"]) * 1024
	info.SReclaimableBytes = rawKB["SReclaimable"] * 1024

	// Available Memory
	if memAvailKB, ok := rawKB["MemAvailable"]; ok && memAvailKB > 0 {
		info.AvailableBytes = memAvailKB * 1024
	} else {
		// Fallback for older Linux kernels (<3.14): Free + Buffers + Cached
		info.AvailableBytes = (rawKB["MemFree"] + rawKB["Buffers"] + rawKB["Cached"] + rawKB["SReclaimable"]) * 1024
		if info.AvailableBytes > info.TotalBytes {
			info.AvailableBytes = info.TotalBytes
		}
	}

	// Linux semantics: Used RAM = Total - Available
	if info.TotalBytes >= info.AvailableBytes {
		info.UsedBytes = info.TotalBytes - info.AvailableBytes
	} else {
		info.UsedBytes = 0
	}

	if info.TotalBytes > 0 {
		info.UtilizationPct = (float64(info.UsedBytes) / float64(info.TotalBytes)) * 100.0
	}

	// Swap
	info.SwapTotalBytes = rawKB["SwapTotal"] * 1024
	info.SwapFreeBytes = rawKB["SwapFree"] * 1024
	if info.SwapTotalBytes >= info.SwapFreeBytes {
		info.SwapUsedBytes = info.SwapTotalBytes - info.SwapFreeBytes
	}
	if info.SwapTotalBytes > 0 {
		info.SwapUtilizationPct = (float64(info.SwapUsedBytes) / float64(info.SwapTotalBytes)) * 100.0
	}

	return info, nil
}

