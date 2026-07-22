package deviceruntime

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"sync"
	"time"
	"unicode/utf8"
)

const (
	scrcpyInjectKeycode   = byte(0)
	scrcpyInjectText      = byte(1)
	scrcpyInjectTouch     = byte(2)
	scrcpyRotateDevice    = byte(11)
	scrcpyActionDown      = byte(0)
	scrcpyActionUp        = byte(1)
	scrcpyActionMove      = byte(2)
	scrcpyFingerPointerID = ^uint64(1)
	scrcpyTextLimit       = 300
	scrcpySwipeSteps      = 6
	scrcpySwipeStepDelay  = 12 * time.Millisecond
)

// scrcpyControlRegistry connects HTTP input requests to the control socket
// owned by the active scrcpy video producer for the same Android device.
type scrcpyControlRegistry struct {
	mu      sync.RWMutex
	clients map[string]*scrcpyControlClient
}

func newScrcpyControlRegistry() *scrcpyControlRegistry {
	return &scrcpyControlRegistry{clients: make(map[string]*scrcpyControlClient)}
}

func (r *scrcpyControlRegistry) set(serial string, client *scrcpyControlClient) {
	r.mu.Lock()
	previous := r.clients[serial]
	r.clients[serial] = client
	r.mu.Unlock()
	if previous != nil && previous != client {
		previous.close()
	}
}

func (r *scrcpyControlRegistry) get(serial string) *scrcpyControlClient {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.clients[serial]
}

func (r *scrcpyControlRegistry) remove(serial string, client *scrcpyControlClient) {
	r.mu.Lock()
	if r.clients[serial] == client {
		delete(r.clients, serial)
	}
	r.mu.Unlock()
}

type scrcpyControlClient struct {
	connection net.Conn
	mu         sync.Mutex
	closeOnce  sync.Once
}

func newScrcpyControlClient(connection net.Conn) *scrcpyControlClient {
	return &scrcpyControlClient{connection: connection}
}

func (c *scrcpyControlClient) close() {
	c.closeOnce.Do(func() { _ = c.connection.Close() })
}

func (c *scrcpyControlClient) tap(ctx context.Context, x, y, width, height int) error {
	down, err := encodeScrcpyTouch(scrcpyActionDown, x, y, width, height, true)
	if err != nil {
		return err
	}
	up, err := encodeScrcpyTouch(scrcpyActionUp, x, y, width, height, false)
	if err != nil {
		return err
	}
	return c.writeMessages(ctx, [][]byte{down, up}, 0)
}

func (c *scrcpyControlClient) swipe(ctx context.Context, startX, startY, endX, endY, width, height int) error {
	messages := make([][]byte, 0, scrcpySwipeSteps+2)
	down, err := encodeScrcpyTouch(scrcpyActionDown, startX, startY, width, height, true)
	if err != nil {
		return err
	}
	messages = append(messages, down)
	for step := 1; step <= scrcpySwipeSteps; step++ {
		x := startX + (endX-startX)*step/scrcpySwipeSteps
		y := startY + (endY-startY)*step/scrcpySwipeSteps
		move, moveErr := encodeScrcpyTouch(scrcpyActionMove, x, y, width, height, true)
		if moveErr != nil {
			return moveErr
		}
		messages = append(messages, move)
	}
	up, err := encodeScrcpyTouch(scrcpyActionUp, endX, endY, width, height, false)
	if err != nil {
		return err
	}
	messages = append(messages, up)
	return c.writeMessages(ctx, messages, scrcpySwipeStepDelay)
}

func (c *scrcpyControlClient) key(ctx context.Context, keycode uint32) error {
	down := encodeScrcpyKey(scrcpyActionDown, keycode)
	up := encodeScrcpyKey(scrcpyActionUp, keycode)
	return c.writeMessages(ctx, [][]byte{down, up}, 0)
}

func (c *scrcpyControlClient) text(ctx context.Context, text string) error {
	if text == "" {
		return nil
	}
	messages := make([][]byte, 0, (len(text)+scrcpyTextLimit-1)/scrcpyTextLimit)
	for text != "" {
		chunk := truncateUTF8(text, scrcpyTextLimit)
		messages = append(messages, encodeScrcpyText(chunk))
		text = text[len(chunk):]
	}
	return c.writeMessages(ctx, messages, 0)
}

func (c *scrcpyControlClient) rotate(ctx context.Context) error {
	return c.writeMessages(ctx, [][]byte{{scrcpyRotateDevice}}, 0)
}

func (c *scrcpyControlClient) writeMessages(ctx context.Context, messages [][]byte, delay time.Duration) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	deadline := time.Now().Add(2 * time.Second)
	if contextDeadline, ok := ctx.Deadline(); ok && contextDeadline.Before(deadline) {
		deadline = contextDeadline
	}
	if err := c.connection.SetWriteDeadline(deadline); err != nil {
		return err
	}
	defer func() { _ = c.connection.SetWriteDeadline(time.Time{}) }()
	for index, message := range messages {
		if err := ctx.Err(); err != nil {
			return err
		}
		if _, err := io.Copy(c.connection, bytes.NewReader(message)); err != nil {
			return err
		}
		if delay > 0 && index < len(messages)-1 {
			timer := time.NewTimer(delay)
			select {
			case <-ctx.Done():
				timer.Stop()
				return ctx.Err()
			case <-timer.C:
			}
		}
	}
	return nil
}

func encodeScrcpyTouch(action byte, x, y, width, height int, pressed bool) ([]byte, error) {
	if width <= 0 || height <= 0 || width > 0xffff || height > 0xffff {
		return nil, fmt.Errorf("invalid scrcpy input size %dx%d", width, height)
	}
	x = max(0, min(x, width-1))
	y = max(0, min(y, height-1))
	message := make([]byte, 32)
	message[0] = scrcpyInjectTouch
	message[1] = action
	binary.BigEndian.PutUint64(message[2:10], scrcpyFingerPointerID)
	binary.BigEndian.PutUint32(message[10:14], uint32(x))
	binary.BigEndian.PutUint32(message[14:18], uint32(y))
	binary.BigEndian.PutUint16(message[18:20], uint16(width))
	binary.BigEndian.PutUint16(message[20:22], uint16(height))
	if pressed {
		binary.BigEndian.PutUint16(message[22:24], 0xffff)
	}
	return message, nil
}

func encodeScrcpyKey(action byte, keycode uint32) []byte {
	message := make([]byte, 14)
	message[0] = scrcpyInjectKeycode
	message[1] = action
	binary.BigEndian.PutUint32(message[2:6], keycode)
	return message
}

func encodeScrcpyText(text string) []byte {
	text = truncateUTF8(text, scrcpyTextLimit)
	message := make([]byte, 5+len(text))
	message[0] = scrcpyInjectText
	binary.BigEndian.PutUint32(message[1:5], uint32(len(text)))
	copy(message[5:], text)
	return message
}

func truncateUTF8(value string, limit int) string {
	if len(value) <= limit {
		return value
	}
	value = value[:limit]
	for !utf8.ValidString(value) {
		value = value[:len(value)-1]
	}
	return value
}
