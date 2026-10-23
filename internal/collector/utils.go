package collector

import (
	"fmt"
	"strconv"
	"strings"
)

// FormatBytes converts raw byte count into human-readable binary format (KiB, MiB, GiB, TiB).
func FormatBytes(b uint64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := uint64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	prefix := "KMGTPE"[exp]
	return fmt.Sprintf("%.2f %ciB", float64(b)/float64(div), prefix)
}

// FormatUptime turns seconds into "X days, Y hrs, Z mins".
func FormatUptime(seconds uint64) string {
	days := seconds / 86400
	hours := (seconds % 86400) / 3600
	mins := (seconds % 3600) / 60
	secs := seconds % 60

	if days > 0 {
		return fmt.Sprintf("%dd %dh %dm", days, hours, mins)
	}
	if hours > 0 {
		return fmt.Sprintf("%dh %dm %ds", hours, mins, secs)
	}
	return fmt.Sprintf("%dm %ds", mins, secs)
}

// ParseUint safely parses an unsigned integer string or returns 0.
func ParseUint(s string) uint64 {
	s = strings.TrimSpace(s)
	val, err := strconv.ParseUint(s, 10, 64)
	if err != nil {
		return 0
	}
	return val
}

// ParseFloat safely parses a float string or returns 0.0.
func ParseFloat(s string) float64 {
	s = strings.TrimSpace(s)
	val, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0
	}
	return val
}

// ParseInt safely parses an int string or returns 0.
func ParseInt(s string) int {
	s = strings.TrimSpace(s)
	val, err := strconv.Atoi(s)
	if err != nil {
		return 0
	}
	return val
}

