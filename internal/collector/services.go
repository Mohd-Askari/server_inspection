package collector

import (
	"bufio"
	"bytes"
	"context"
	"strings"

	"coreip-server-inspector/internal/models"
)

// ServicesCollector monitors systemd units with filtering and pagination.
type ServicesCollector struct {
	runner   CommandRunner
	maxItems int
}

// NewServicesCollector creates a ServicesCollector.
func NewServicesCollector(runner CommandRunner, maxItems int) *ServicesCollector {
	if maxItems <= 0 {
		maxItems = 200
	}
	return &ServicesCollector{
		runner:   runner,
		maxItems: maxItems,
	}
}

// CollectAll queries systemctl for all systemd service units.
func (c *ServicesCollector) CollectAll(ctx context.Context) (models.ServicesInfo, error) {
	var info models.ServicesInfo
	info.PageSize = 25
	info.Page = 1

	// Check if systemctl exists
	if _, err := c.runner.LookPath("systemctl"); err != nil {
		info.SystemdAvailable = false
		return info, nil
	}

	out, err := c.runner.Run(ctx, "systemctl", "list-units", "--type=service", "--all", "--no-pager", "--no-legend")
	if err != nil {
		info.SystemdAvailable = false
		return info, nil
	}

	info.SystemdAvailable = true
	scanner := bufio.NewScanner(bytes.NewReader(out))

	var allServices []models.ServiceItem
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		fields := strings.Fields(line)
		if len(fields) < 4 {
			continue
		}

		name := fields[0]
		loadState := fields[1]
		activeState := fields[2]
		subState := fields[3]

		var desc string
		if len(fields) > 4 {
			desc = strings.Join(fields[4:], " ")
		}

		isFailed := (activeState == "failed" || subState == "failed")

		info.TotalServices++
		if isFailed {
			info.FailedServices++
		} else if activeState == "active" {
			info.ActiveServices++
		} else {
			info.InactiveServices++
		}

		allServices = append(allServices, models.ServiceItem{
			Name:        name,
			LoadState:   loadState,
			ActiveState: activeState,
			SubState:    subState,
			Description: desc,
			IsFailed:    isFailed,
		})
	}

	// Truncate to max items if unbounded
	if len(allServices) > c.maxItems {
		allServices = allServices[:c.maxItems]
	}
	info.Services = allServices

	return info, nil
}

// FilterAndPaginate applies query matching and page slicing to a service list.
func (c *ServicesCollector) FilterAndPaginate(info models.ServicesInfo, filter string, page, pageSize int) models.ServicesInfo {
	if pageSize <= 0 {
		pageSize = 25
	}
	if page <= 0 {
		page = 1
	}

	filter = strings.TrimSpace(strings.ToLower(filter))
	var filtered []models.ServiceItem

	for _, s := range info.Services {
		// Status filter matching
		if filter == "failed" && !s.IsFailed {
			continue
		} else if filter == "active" && s.ActiveState != "active" {
			continue
		} else if filter == "inactive" && s.ActiveState == "active" {
			continue
		} else if filter != "" && filter != "failed" && filter != "active" && filter != "inactive" {
			// Substring search on name or description
			nameLower := strings.ToLower(s.Name)
			descLower := strings.ToLower(s.Description)
			if !strings.Contains(nameLower, filter) && !strings.Contains(descLower, filter) {
				continue
			}
		}
		filtered = append(filtered, s)
	}

	totalItems := len(filtered)
	totalPages := (totalItems + pageSize - 1) / pageSize
	if totalPages == 0 {
		totalPages = 1
	}
	if page > totalPages {
		page = totalPages
	}

	start := (page - 1) * pageSize
	end := start + pageSize
	if start > totalItems {
		start = totalItems
	}
	if end > totalItems {
		end = totalItems
	}

	result := info
	result.Services = filtered[start:end]
	result.Page = page
	result.PageSize = pageSize
	result.TotalPages = totalPages
	result.Filter = filter

	return result
}
