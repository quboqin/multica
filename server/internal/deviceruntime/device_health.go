package deviceruntime

import (
	"context"
	"strconv"
	"strings"
	"sync"
	"time"
)

const deviceHealthCacheTTL = 10 * time.Second

type deviceHealthCacheEntry struct {
	device    Device
	expiresAt time.Time
}

type deviceHealthCache struct {
	mu      sync.Mutex
	entries map[string]deviceHealthCacheEntry
}

func newDeviceHealthCache() *deviceHealthCache {
	return &deviceHealthCache{entries: make(map[string]deviceHealthCacheEntry)}
}

func (c *deviceHealthCache) get(serial string, now time.Time) (Device, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	entry, ok := c.entries[serial]
	return entry.device, ok && now.Before(entry.expiresAt)
}

func (c *deviceHealthCache) put(device Device, now time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries[device.Serial] = deviceHealthCacheEntry{device: device, expiresAt: now.Add(deviceHealthCacheTTL)}
}

func (a ADB) enrichDeviceHealth(ctx context.Context, device Device) Device {
	switch device.State {
	case "unauthorized":
		device.Health = "unavailable"
		device.Reason = "USB debugging authorization required"
		return device
	case "offline":
		device.Health = "unavailable"
		device.Reason = "ADB transport is offline"
		return device
	case "device":
		device.Health = "ready"
	default:
		device.Health = "unavailable"
		device.Reason = "ADB state: " + device.State
		return device
	}
	now := time.Now()
	if cached, ok := a.runtime.health.get(device.Serial, now); ok {
		cached.State = device.State
		cached.Kind = device.Kind
		cached.Model = device.Model
		return cached
	}
	output, err := a.run(ctx, "-s", device.Serial, "shell", "dumpsys", "battery")
	if err != nil {
		device.Health = "warning"
		device.Reason = "Device health check failed"
		a.runtime.health.put(device, now)
		return device
	}
	device = parseBatteryHealth(device, string(output))
	a.runtime.health.put(device, now)
	return device
}

func parseBatteryHealth(device Device, output string) Device {
	values := make(map[string]string)
	for _, line := range strings.Split(output, "\n") {
		key, value, ok := strings.Cut(strings.TrimSpace(line), ":")
		if ok {
			values[strings.TrimSpace(key)] = strings.TrimSpace(value)
		}
	}
	if level, err := strconv.Atoi(values["level"]); err == nil {
		device.BatteryLevel = &level
	}
	device.Charging = values["AC powered"] == "true" || values["USB powered"] == "true" || values["Wireless powered"] == "true"
	if raw, err := strconv.Atoi(values["temperature"]); err == nil {
		temperature := float64(raw) / 10
		device.TemperatureC = &temperature
	}
	device.Health = "ready"
	if device.BatteryLevel != nil && *device.BatteryLevel <= 15 && !device.Charging {
		device.Health = "warning"
		device.Reason = "Battery is low"
	}
	if device.TemperatureC != nil && *device.TemperatureC >= 45 {
		device.Health = "warning"
		device.Reason = "Device temperature is high"
	}
	return device
}
