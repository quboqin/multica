package deviceruntime

import (
	"bytes"
	"context"
	"fmt"
	"net"
	"net/url"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

type Device struct {
	Serial       string   `json:"serial"`
	State        string   `json:"state"`
	Model        string   `json:"model,omitempty"`
	Kind         string   `json:"kind"`
	Health       string   `json:"health"`
	Reason       string   `json:"reason,omitempty"`
	BatteryLevel *int     `json:"battery_level,omitempty"`
	Charging     bool     `json:"charging"`
	TemperatureC *float64 `json:"temperature_c,omitempty"`
}

type ADB struct {
	Binary  string
	runtime *adbRuntimeState
}

const displaySizeCacheTTL = 30 * time.Second

type adbRuntimeState struct {
	controls     *scrcpyControlRegistry
	displaySizes *displaySizeCache
	health       *deviceHealthCache
}

func newADBRuntimeState() *adbRuntimeState {
	return &adbRuntimeState{
		controls:     newScrcpyControlRegistry(),
		displaySizes: newDisplaySizeCache(displaySizeCacheTTL),
		health:       newDeviceHealthCache(),
	}
}

func (a ADB) withRuntimeState() ADB {
	if a.runtime == nil {
		a.runtime = newADBRuntimeState()
	}
	return a
}

var currentDisplaySizePattern = regexp.MustCompile(`\bcur=(\d+)x(\d+)\b`)

func (a ADB) binary() string {
	if a.Binary != "" {
		return a.Binary
	}
	return "adb"
}

func (a ADB) run(ctx context.Context, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, a.binary(), args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("adb %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
	}
	return out, nil
}

func (a ADB) Devices(ctx context.Context) ([]Device, error) {
	a = a.withRuntimeState()
	out, err := a.run(ctx, "devices", "-l")
	if err != nil {
		return nil, err
	}
	var devices []Device
	for _, line := range strings.Split(string(out), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 || fields[0] == "List" || strings.HasPrefix(fields[0], "*") {
			continue
		}
		device := Device{Serial: fields[0], State: fields[1], Kind: "physical"}
		if strings.HasPrefix(device.Serial, "emulator-") {
			device.Kind = "emulator"
		}
		for _, field := range fields[2:] {
			if value, ok := strings.CutPrefix(field, "model:"); ok {
				device.Model = strings.ReplaceAll(value, "_", " ")
			}
		}
		devices = append(devices, a.enrichDeviceHealth(ctx, device))
	}
	return devices, nil
}

func (a ADB) Install(ctx context.Context, serial, artifact string) error {
	_, err := a.run(ctx, "-s", serial, "install", "-r", "-g", artifact)
	return err
}

func (a ADB) Start(ctx context.Context, serial, component, previewURL string) error {
	args := []string{"-s", serial, "shell", "am", "start", "-W", "-n", component}
	if previewURL != "" {
		args = append(args, "--es", "multica_preview_url", previewURL)
	}
	_, err := a.run(ctx, args...)
	return err
}

func (a ADB) PackageInstalled(ctx context.Context, serial, packageName string) (bool, error) {
	out, err := a.run(ctx, "-s", serial, "shell", "pm", "path", packageName)
	if err != nil {
		if strings.Contains(err.Error(), "Unknown package") {
			return false, nil
		}
		return false, err
	}
	return strings.Contains(string(out), "package:"), nil
}

func (a ADB) InstallAndStart(ctx context.Context, serial, artifact, component, previewURL string) error {
	if err := a.Install(ctx, serial, artifact); err != nil {
		return err
	}
	return a.Start(ctx, serial, component, previewURL)
}

// ReverseWebPreview makes a loopback H5 URL on a USB device resolve to the
// Runtime Host. Emulator URLs normally use 10.0.2.2 and therefore skip this.
func (a ADB) ReverseWebPreview(ctx context.Context, serial, previewURL string) error {
	port, ok := reversePortForWebURL(previewURL)
	if !ok {
		return nil
	}
	endpoint := "tcp:" + port
	_, err := a.run(ctx, "-s", serial, "reverse", endpoint, endpoint)
	return err
}

func reversePortForWebURL(raw string) (string, bool) {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Hostname() == "" {
		return "", false
	}
	host := parsed.Hostname()
	ip := net.ParseIP(host)
	if !strings.EqualFold(host, "localhost") && (ip == nil || !ip.IsLoopback()) {
		return "", false
	}
	if port := parsed.Port(); port != "" {
		return port, true
	}
	if parsed.Scheme == "http" {
		return "80", true
	}
	if parsed.Scheme == "https" {
		return "443", true
	}
	return "", false
}

func (a ADB) Screenshot(ctx context.Context, serial string) ([]byte, error) {
	return a.run(ctx, "-s", serial, "exec-out", "screencap", "-p")
}

func (a ADB) DisplaySize(ctx context.Context, serial string) (int, int, error) {
	if a.runtime != nil {
		if width, height, ok := a.runtime.displaySizes.get(serial, time.Now()); ok {
			return width, height, nil
		}
	}
	out, err := a.run(ctx, "-s", serial, "shell", "dumpsys", "window", "displays")
	if err != nil {
		return 0, 0, err
	}
	width, height, err := parseCurrentDisplaySize(out)
	if err == nil && a.runtime != nil {
		a.runtime.displaySizes.put(serial, width, height, time.Now())
	}
	return width, height, err
}

func parseCurrentDisplaySize(out []byte) (int, int, error) {
	matches := currentDisplaySizePattern.FindAllSubmatch(out, -1)
	if len(matches) == 0 {
		return 0, 0, fmt.Errorf("current Android display size is unavailable")
	}
	var selectedWidth, selectedHeight int
	for _, match := range matches {
		width, widthErr := strconv.Atoi(string(match[1]))
		height, heightErr := strconv.Atoi(string(match[2]))
		if widthErr != nil || heightErr != nil || width <= 0 || height <= 0 {
			continue
		}
		if width*height > selectedWidth*selectedHeight {
			selectedWidth, selectedHeight = width, height
		}
	}
	if selectedWidth == 0 || selectedHeight == 0 {
		return 0, 0, fmt.Errorf("invalid Android display size")
	}
	return selectedWidth, selectedHeight, nil
}

func (a ADB) Tap(ctx context.Context, serial string, x, y int) error {
	return a.TapAt(ctx, serial, x, y, 0, 0)
}

func (a ADB) TapAt(ctx context.Context, serial string, x, y, sourceWidth, sourceHeight int) error {
	width, height, err := a.inputSize(ctx, serial, sourceWidth, sourceHeight)
	if err != nil {
		return err
	}
	if control := a.control(serial); control != nil {
		if err = control.tap(ctx, x, y, width, height); err == nil {
			return nil
		} else if ctx.Err() != nil {
			return ctx.Err()
		}
		a.dropControl(serial, control)
	}
	if sourceWidth > 0 && sourceHeight > 0 {
		displayWidth, displayHeight, sizeErr := a.DisplaySize(ctx, serial)
		if sizeErr != nil {
			return sizeErr
		}
		x = scaleCoordinate(x, sourceWidth, displayWidth)
		y = scaleCoordinate(y, sourceHeight, displayHeight)
	}
	_, err = a.run(ctx, "-s", serial, "shell", "input", "tap", fmt.Sprint(x), fmt.Sprint(y))
	return err
}

func (a ADB) Swipe(ctx context.Context, serial string, startX, startY, endX, endY int) error {
	return a.SwipeAt(ctx, serial, startX, startY, endX, endY, 0, 0)
}

func (a ADB) SwipeAt(ctx context.Context, serial string, startX, startY, endX, endY, sourceWidth, sourceHeight int) error {
	width, height, err := a.inputSize(ctx, serial, sourceWidth, sourceHeight)
	if err != nil {
		return err
	}
	if control := a.control(serial); control != nil {
		if err = control.swipe(ctx, startX, startY, endX, endY, width, height); err == nil {
			return nil
		} else if ctx.Err() != nil {
			return ctx.Err()
		}
		a.dropControl(serial, control)
	}
	if sourceWidth > 0 && sourceHeight > 0 {
		displayWidth, displayHeight, sizeErr := a.DisplaySize(ctx, serial)
		if sizeErr != nil {
			return sizeErr
		}
		startX = scaleCoordinate(startX, sourceWidth, displayWidth)
		startY = scaleCoordinate(startY, sourceHeight, displayHeight)
		endX = scaleCoordinate(endX, sourceWidth, displayWidth)
		endY = scaleCoordinate(endY, sourceHeight, displayHeight)
	}
	_, err = a.run(ctx, "-s", serial, "shell", "input", "swipe", fmt.Sprint(startX), fmt.Sprint(startY), fmt.Sprint(endX), fmt.Sprint(endY), "220")
	return err
}

func (a ADB) Key(ctx context.Context, serial, key string) error {
	keycode, keycodeErr := strconv.ParseUint(key, 10, 32)
	if control := a.control(serial); control != nil && keycodeErr == nil {
		if err := control.key(ctx, uint32(keycode)); err == nil {
			return nil
		} else if ctx.Err() != nil {
			return ctx.Err()
		}
		a.dropControl(serial, control)
	}
	_, err := a.run(ctx, "-s", serial, "shell", "input", "keyevent", key)
	return err
}

func (a ADB) Rotate(ctx context.Context, serial string) error {
	if a.runtime != nil {
		a.runtime.displaySizes.invalidate(serial)
	}
	if control := a.control(serial); control != nil {
		if err := control.rotate(ctx); err == nil {
			return nil
		} else if ctx.Err() != nil {
			return ctx.Err()
		}
		a.dropControl(serial, control)
	}
	// Force a stable orientation, then alternate between portrait and landscape.
	if _, err := a.run(ctx, "-s", serial, "shell", "settings", "put", "system", "accelerometer_rotation", "0"); err != nil {
		return err
	}
	out, err := a.run(ctx, "-s", serial, "shell", "settings", "get", "system", "user_rotation")
	if err != nil {
		return err
	}
	next := "1"
	if strings.TrimSpace(string(out)) == "1" {
		next = "0"
	}
	_, err = a.run(ctx, "-s", serial, "shell", "settings", "put", "system", "user_rotation", next)
	return err
}

func (a ADB) Text(ctx context.Context, serial, text string) error {
	if text == "" {
		return nil
	}
	if control := a.control(serial); control != nil {
		if err := control.text(ctx, text); err == nil {
			return nil
		} else if ctx.Err() != nil {
			return ctx.Err()
		}
		a.dropControl(serial, control)
	}
	_, err := a.run(ctx, "-s", serial, "shell", "input", "text", strings.ReplaceAll(text, " ", "%s"))
	return err
}

func (a ADB) inputSize(ctx context.Context, serial string, sourceWidth, sourceHeight int) (int, int, error) {
	if sourceWidth > 0 && sourceHeight > 0 {
		return sourceWidth, sourceHeight, nil
	}
	return a.DisplaySize(ctx, serial)
}

func (a ADB) control(serial string) *scrcpyControlClient {
	if a.runtime == nil {
		return nil
	}
	return a.runtime.controls.get(serial)
}

func (a ADB) dropControl(serial string, control *scrcpyControlClient) {
	if a.runtime != nil {
		a.runtime.controls.remove(serial, control)
	}
	control.close()
}

func (a ADB) Clear(ctx context.Context, serial, packageName string) error {
	if _, err := a.run(ctx, "-s", serial, "shell", "am", "force-stop", packageName); err != nil {
		return err
	}
	_, err := a.run(ctx, "-s", serial, "shell", "pm", "clear", packageName)
	return err
}

func (a ADB) Logs(ctx context.Context, serial, packageName string) ([]byte, error) {
	return a.run(ctx, "-s", serial, "logcat", "-d", "-t", "160")
}

type displaySize struct {
	width     int
	height    int
	expiresAt time.Time
}

type displaySizeCache struct {
	mu      sync.Mutex
	ttl     time.Duration
	entries map[string]displaySize
}

func newDisplaySizeCache(ttl time.Duration) *displaySizeCache {
	return &displaySizeCache{ttl: ttl, entries: make(map[string]displaySize)}
}

func (c *displaySizeCache) get(serial string, now time.Time) (int, int, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	value, ok := c.entries[serial]
	if !ok || !now.Before(value.expiresAt) {
		delete(c.entries, serial)
		return 0, 0, false
	}
	return value.width, value.height, true
}

func (c *displaySizeCache) put(serial string, width, height int, now time.Time) {
	c.mu.Lock()
	c.entries[serial] = displaySize{width: width, height: height, expiresAt: now.Add(c.ttl)}
	c.mu.Unlock()
}

func (c *displaySizeCache) invalidate(serial string) {
	c.mu.Lock()
	delete(c.entries, serial)
	c.mu.Unlock()
}
