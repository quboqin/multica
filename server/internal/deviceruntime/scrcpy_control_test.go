package deviceruntime

import (
	"bytes"
	"context"
	"encoding/binary"
	"io"
	"net"
	"strings"
	"testing"
	"time"
)

func TestEncodeScrcpyTouchMatchesVersion334Protocol(t *testing.T) {
	message, err := encodeScrcpyTouch(scrcpyActionDown, 123, 456, 576, 1280, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(message) != 32 || message[0] != scrcpyInjectTouch || message[1] != scrcpyActionDown {
		t.Fatalf("unexpected touch header: %x", message)
	}
	if got := binary.BigEndian.Uint64(message[2:10]); got != scrcpyFingerPointerID {
		t.Fatalf("pointer id = %#x", got)
	}
	if got := binary.BigEndian.Uint32(message[10:14]); got != 123 {
		t.Fatalf("x = %d", got)
	}
	if got := binary.BigEndian.Uint32(message[14:18]); got != 456 {
		t.Fatalf("y = %d", got)
	}
	if got := binary.BigEndian.Uint16(message[18:20]); got != 576 {
		t.Fatalf("width = %d", got)
	}
	if got := binary.BigEndian.Uint16(message[20:22]); got != 1280 {
		t.Fatalf("height = %d", got)
	}
	if got := binary.BigEndian.Uint16(message[22:24]); got != 0xffff {
		t.Fatalf("pressure = %#x", got)
	}
	if !bytes.Equal(message[24:], make([]byte, 8)) {
		t.Fatalf("button fields are not empty: %x", message[24:])
	}
}

func TestADBUsesPersistentScrcpyControlForTap(t *testing.T) {
	runtime := newADBRuntimeState()
	host, device := net.Pipe()
	t.Cleanup(func() {
		_ = host.Close()
		_ = device.Close()
	})
	control := newScrcpyControlClient(host)
	runtime.controls.set("device-1", control)
	adb := ADB{Binary: "binary-that-must-not-run", runtime: runtime}

	received := make(chan []byte, 1)
	go func() {
		messages := make([]byte, 64)
		_, _ = io.ReadFull(device, messages)
		received <- messages
	}()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := adb.TapAt(ctx, "device-1", 200, 300, 576, 1280); err != nil {
		t.Fatal(err)
	}

	messages := <-received
	if messages[1] != scrcpyActionDown || messages[33] != scrcpyActionUp {
		t.Fatalf("tap actions = %d, %d", messages[1], messages[33])
	}
	if got := binary.BigEndian.Uint16(messages[18:20]); got != 576 {
		t.Fatalf("source width = %d", got)
	}
	if got := binary.BigEndian.Uint16(messages[20:22]); got != 1280 {
		t.Fatalf("source height = %d", got)
	}
}

func TestScrcpyControlTextSplitsAtUTF8Boundaries(t *testing.T) {
	host, device := net.Pipe()
	t.Cleanup(func() {
		_ = host.Close()
		_ = device.Close()
	})
	control := newScrcpyControlClient(host)
	text := strings.Repeat("a", 299) + "界b"
	expectedSize := 5 + 299 + 5 + len("界b")
	received := make(chan []byte, 1)
	go func() {
		messages := make([]byte, expectedSize)
		_, _ = io.ReadFull(device, messages)
		received <- messages
	}()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := control.text(ctx, text); err != nil {
		t.Fatal(err)
	}

	messages := <-received
	firstLength := int(binary.BigEndian.Uint32(messages[1:5]))
	if firstLength != 299 {
		t.Fatalf("first text length = %d", firstLength)
	}
	second := messages[5+firstLength:]
	if second[0] != scrcpyInjectText || binary.BigEndian.Uint32(second[1:5]) != uint32(len("界b")) {
		t.Fatalf("unexpected second text header: %x", second[:5])
	}
	if got := string(second[5:]); got != "界b" {
		t.Fatalf("second text = %q", got)
	}
}

func TestDisplaySizeCacheExpiresAndInvalidates(t *testing.T) {
	cache := newDisplaySizeCache(time.Minute)
	now := time.Unix(100, 0)
	cache.put("device-1", 576, 1280, now)
	if width, height, ok := cache.get("device-1", now.Add(time.Second)); !ok || width != 576 || height != 1280 {
		t.Fatalf("cached size = %dx%d, %v", width, height, ok)
	}
	cache.invalidate("device-1")
	if _, _, ok := cache.get("device-1", now.Add(time.Second)); ok {
		t.Fatal("invalidated display size remained cached")
	}
	cache.put("device-1", 576, 1280, now)
	if _, _, ok := cache.get("device-1", now.Add(time.Minute)); ok {
		t.Fatal("expired display size remained cached")
	}
}

func TestScrcpyServerCommandEnablesPersistentControl(t *testing.T) {
	command := scrcpyServerCommand("3.3.4", 0x1234abcd)
	for _, option := range []string{
		"scid=1234abcd",
		"control=true",
		"clipboard_autosync=false",
		"max_fps=60",
		"video_codec_options=i-frame-interval=1,repeat-previous-frame-after=16666,priority=0,operating-rate=60",
	} {
		if !strings.Contains(command, option) {
			t.Fatalf("command does not contain %q: %s", option, command)
		}
	}
	if strings.Contains(command, "control=false") {
		t.Fatalf("control was disabled: %s", command)
	}
}
