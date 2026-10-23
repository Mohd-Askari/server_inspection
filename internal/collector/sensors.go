package collector

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"coreip-server-inspector/internal/models"
)

// SensorsCollector inspects hwmon and thermal_zone sysfs trees for thermal and fan telemetry.
type SensorsCollector struct {
	fs FSReader
}

// NewSensorsCollector creates a SensorsCollector.
func NewSensorsCollector(fs FSReader) *SensorsCollector {
	return &SensorsCollector{fs: fs}
}

// Collect reads temperature sensors and cooling fans.
func (c *SensorsCollector) Collect(ctx context.Context) (models.SensorInfo, error) {
	var info models.SensorInfo

	// 1. Scan /sys/class/hwmon/
	c.scanHwmon(&info)

	// 2. If no temperatures found, try /sys/class/thermal/
	if len(info.Temperatures) == 0 {
		c.scanThermalZones(&info)
	}

	return info, nil
}

func (c *SensorsCollector) scanHwmon(info *models.SensorInfo) {
	hwmonBase := "/sys/class/hwmon"
	if !c.fs.Exists(hwmonBase) {
		return
	}

	entries, err := c.fs.ReadDir(hwmonBase)
	if err != nil {
		return
	}

	for _, entry := range entries {
		hwmonDir := filepath.Join(hwmonBase, entry.Name())
		moduleName := SafeReadString(c.fs, filepath.Join(hwmonDir, "name"))
		if moduleName == "" {
			moduleName = entry.Name()
		}

		files, err := c.fs.ReadDir(hwmonDir)
		if err != nil {
			continue
		}

		for _, f := range files {
			fName := f.Name()

			// Temperature inputs: temp*_input
			if strings.HasPrefix(fName, "temp") && strings.HasSuffix(fName, "_input") {
				rawVal := ParseFloat(SafeReadString(c.fs, filepath.Join(hwmonDir, fName)))
				if rawVal <= 0 {
					continue
				}
				tempC := rawVal / 1000.0 // millidegrees to Celsius

				prefix := strings.TrimSuffix(fName, "_input")
				label := SafeReadString(c.fs, filepath.Join(hwmonDir, prefix+"_label"))
				if label == "" {
					label = fmt.Sprintf("%s (%s)", moduleName, prefix)
				}

				critVal := ParseFloat(SafeReadString(c.fs, filepath.Join(hwmonDir, prefix+"_crit"))) / 1000.0
				maxVal := ParseFloat(SafeReadString(c.fs, filepath.Join(hwmonDir, prefix+"_max"))) / 1000.0

				info.Temperatures = append(info.Temperatures, models.TemperatureSensor{
					Name:       fName,
					Label:      label,
					CurrentC:   tempC,
					CriticalC:  critVal,
					HighC:      maxVal,
					SensorType: moduleName,
				})
			}

			// Fan inputs: fan*_input
			if strings.HasPrefix(fName, "fan") && strings.HasSuffix(fName, "_input") {
				rpm := ParseInt(SafeReadString(c.fs, filepath.Join(hwmonDir, fName)))
				prefix := strings.TrimSuffix(fName, "_input")
				label := SafeReadString(c.fs, filepath.Join(hwmonDir, prefix+"_label"))
				if label == "" {
					label = fmt.Sprintf("%s (%s)", moduleName, prefix)
				}

				info.Fans = append(info.Fans, models.FanSensor{
					Name:       fName,
					Label:      label,
					CurrentRPM: rpm,
				})
			}
		}
	}
}

func (c *SensorsCollector) scanThermalZones(info *models.SensorInfo) {
	thermalBase := "/sys/class/thermal"
	if !c.fs.Exists(thermalBase) {
		return
	}

	entries, err := c.fs.ReadDir(thermalBase)
	if err != nil {
		return
	}

	for _, entry := range entries {
		if !strings.HasPrefix(entry.Name(), "thermal_zone") {
			continue
		}
		zDir := filepath.Join(thermalBase, entry.Name())
		tType := SafeReadString(c.fs, filepath.Join(zDir, "type"))
		rawVal := ParseFloat(SafeReadString(c.fs, filepath.Join(zDir, "temp")))
		if rawVal <= 0 {
			continue
		}
		tempC := rawVal / 1000.0

		if tType == "" {
			tType = entry.Name()
		}

		info.Temperatures = append(info.Temperatures, models.TemperatureSensor{
			Name:       entry.Name(),
			Label:      tType,
			CurrentC:   tempC,
			SensorType: "thermal_zone",
		})
	}
}

