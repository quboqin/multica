package deviceruntime

import (
	"bytes"
	"context"
	"encoding/binary"
	"io"
	"sync"
	"testing"
	"time"

	"github.com/pion/webrtc/v4"
)

type oneByteReader struct {
	reader io.Reader
}

func TestWebRTCSessionStoreKeepsMultipleDeviceViewers(t *testing.T) {
	store := newWebRTCSessionStore()
	firstContext, firstCancel := context.WithCancel(context.Background())
	first := &webRTCSession{id: "first", serial: "device-1", cancel: firstCancel, done: make(chan struct{})}
	store.add(first)

	secondContext, secondCancel := context.WithCancel(context.Background())
	second := &webRTCSession{id: "second", serial: "device-1", cancel: secondCancel, done: make(chan struct{})}
	store.add(second)

	if firstContext.Err() != nil {
		t.Fatal("first viewer was canceled by second viewer")
	}
	if secondContext.Err() != nil {
		t.Fatal("second viewer was canceled")
	}
	if store.count() != 2 {
		t.Fatalf("session count = %d", store.count())
	}
	store.remove(first)
	firstCancel()
	if !store.close(second.id) {
		t.Fatal("active session was not found")
	}
	select {
	case <-secondContext.Done():
	case <-time.After(time.Second):
		t.Fatal("explicitly closed session was not canceled")
	}
	store.remove(second)
	if store.count() != 0 {
		t.Fatalf("session count after cleanup = %d", store.count())
	}
}

func TestBroadcasterWaitsForKeyFrameAfterQueueDrop(t *testing.T) {
	subscriber := &videoSubscriber{
		samples:            make(chan encodedVideoSample, 1),
		stop:               make(chan struct{}),
		waitingForKeyFrame: true,
	}
	source := &scrcpyVideoSource{subscribers: map[string]*videoSubscriber{"viewer": subscriber}}

	source.broadcast(encodedVideoSample{data: []byte{1}})
	if len(subscriber.samples) != 0 {
		t.Fatal("delta frame was delivered before a key frame")
	}
	source.broadcast(encodedVideoSample{data: []byte{2}, keyFrame: true})
	if len(subscriber.samples) != 1 || subscriber.waitingForKeyFrame {
		t.Fatal("key frame did not start subscriber playback")
	}
	source.broadcast(encodedVideoSample{data: []byte{3}})
	if len(subscriber.samples) != 0 || !subscriber.waitingForKeyFrame {
		t.Fatal("queue overflow did not reset subscriber to key frame wait")
	}
	source.broadcast(encodedVideoSample{data: []byte{4}})
	if len(subscriber.samples) != 0 {
		t.Fatal("delta frame was delivered after queue overflow")
	}
	source.broadcast(encodedVideoSample{data: []byte{5}, keyFrame: true})
	if len(subscriber.samples) != 1 || subscriber.waitingForKeyFrame {
		t.Fatal("subscriber did not recover on the next key frame")
	}
}

func TestBroadcasterDropsStaleVideoBeforeChannelIsFull(t *testing.T) {
	subscriber := &videoSubscriber{
		samples:            make(chan encodedVideoSample, 10),
		stop:               make(chan struct{}),
		waitingForKeyFrame: true,
	}
	source := &scrcpyVideoSource{subscribers: map[string]*videoSubscriber{"viewer": subscriber}}
	frame := func(value byte, keyFrame bool) encodedVideoSample {
		return encodedVideoSample{data: []byte{value}, duration: 20 * time.Millisecond, keyFrame: keyFrame}
	}

	source.broadcast(frame(1, true))
	source.broadcast(frame(2, false))
	if len(subscriber.samples) != 2 {
		t.Fatalf("queued samples before latency limit = %d", len(subscriber.samples))
	}
	source.broadcast(frame(3, false))
	if len(subscriber.samples) != 0 || !subscriber.waitingForKeyFrame {
		t.Fatal("stale queued video was not dropped at the latency limit")
	}
	source.broadcast(frame(4, false))
	if len(subscriber.samples) != 0 {
		t.Fatal("delta frame was delivered while waiting for recovery")
	}
	source.broadcast(frame(5, true))
	if len(subscriber.samples) != 1 || subscriber.waitingForKeyFrame {
		t.Fatal("subscriber did not recover on a fresh key frame")
	}
}

func TestScrcpyVideoSourceKeepsProducerForRemainingSubscriber(t *testing.T) {
	broadcaster := newScrcpyBroadcaster(ADB{}, "server.jar", "test")
	broadcaster.idleGrace = 10 * time.Millisecond
	source := broadcaster.source("device-1")
	source.running = true
	stopped := make(chan struct{}, 1)
	source.stopProducer = func() { stopped <- struct{}{} }

	unsubscribeFirst := source.subscribe("first", testVideoTrack(t), func() {})
	unsubscribeSecond := source.subscribe("second", testVideoTrack(t), func() {})
	unsubscribeSecond()
	t.Cleanup(func() {
		source.mu.Lock()
		source.running = false
		source.mu.Unlock()
		unsubscribeFirst()
	})

	time.Sleep(3 * broadcaster.idleGrace)
	select {
	case <-stopped:
		t.Fatal("producer stopped while one subscriber remained")
	default:
	}
	source.mu.Lock()
	remaining := len(source.subscribers)
	source.mu.Unlock()
	if remaining != 1 {
		t.Fatalf("remaining subscribers = %d", remaining)
	}
}

func TestScrcpyVideoSourceStopsAfterLastSubscriber(t *testing.T) {
	broadcaster := newScrcpyBroadcaster(ADB{}, "server.jar", "test")
	broadcaster.idleGrace = 10 * time.Millisecond
	source := broadcaster.source("device-1")
	source.running = true
	stopped := make(chan struct{})
	var stopOnce sync.Once
	source.stopProducer = func() { stopOnce.Do(func() { close(stopped) }) }

	unsubscribe := source.subscribe("only", testVideoTrack(t), func() {})
	unsubscribe()
	select {
	case <-stopped:
	case <-time.After(time.Second):
		t.Fatal("producer did not stop after the last subscriber left")
	}
}

func TestScrcpyVideoSourceCancelsIdleStopOnResubscribe(t *testing.T) {
	broadcaster := newScrcpyBroadcaster(ADB{}, "server.jar", "test")
	broadcaster.idleGrace = 30 * time.Millisecond
	source := broadcaster.source("device-1")
	source.running = true
	stopped := make(chan struct{}, 1)
	source.stopProducer = func() { stopped <- struct{}{} }

	unsubscribeFirst := source.subscribe("first", testVideoTrack(t), func() {})
	unsubscribeFirst()
	time.Sleep(5 * time.Millisecond)
	unsubscribeSecond := source.subscribe("second", testVideoTrack(t), func() {})
	t.Cleanup(func() {
		source.mu.Lock()
		source.running = false
		source.mu.Unlock()
		unsubscribeSecond()
	})

	time.Sleep(2 * broadcaster.idleGrace)
	select {
	case <-stopped:
		t.Fatal("producer stopped after a subscriber rejoined")
	default:
	}
}

func testVideoTrack(t *testing.T) *webrtc.TrackLocalStaticSample {
	t.Helper()
	track, err := webrtc.NewTrackLocalStaticSample(webrtc.RTPCodecCapability{
		MimeType: webrtc.MimeTypeH264,
	}, "video", "test")
	if err != nil {
		t.Fatal(err)
	}
	return track
}

func (r oneByteReader) Read(buffer []byte) (int, error) {
	if len(buffer) > 1 {
		buffer = buffer[:1]
	}
	return r.reader.Read(buffer)
}

func TestScrcpyPacketReaderPreservesFrameBoundaries(t *testing.T) {
	var stream bytes.Buffer
	writeScrcpyPacket(t, &stream, scrcpyConfigFlag, []byte{0, 0, 0, 1, 0x67})
	writeScrcpyPacket(t, &stream, scrcpyKeyFlag|123456, []byte{0, 0, 0, 1, 0x65, 0xaa})

	reader := newScrcpyPacketReader(oneByteReader{reader: &stream})
	config, err := reader.Next()
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	if !config.Config || config.KeyFrame || config.PTS != 0 {
		t.Fatalf("unexpected config metadata: %+v", config)
	}

	frame, err := reader.Next()
	if err != nil {
		t.Fatalf("read key frame: %v", err)
	}
	if frame.Config || !frame.KeyFrame || frame.PTS != 123456 {
		t.Fatalf("unexpected frame metadata: %+v", frame)
	}
	if !bytes.Equal(frame.Data, []byte{0, 0, 0, 1, 0x65, 0xaa}) {
		t.Fatalf("unexpected frame data: %x", frame.Data)
	}
}

func TestScrcpyPacketReaderRejectsOversizedPacket(t *testing.T) {
	var header [12]byte
	binary.BigEndian.PutUint32(header[8:], maxScrcpyPacket+1)
	_, err := newScrcpyPacketReader(bytes.NewReader(header[:])).Next()
	if err == nil {
		t.Fatal("expected oversized packet to fail")
	}
}

func TestScrcpySampleDurationPreservesRealtimeFrameCadence(t *testing.T) {
	tests := []struct {
		name        string
		previousPTS int64
		currentPTS  int64
		want        time.Duration
	}{
		{name: "sixty fps", previousPTS: 1_000_000, currentPTS: 1_016_667, want: 16_667 * time.Microsecond},
		{name: "thirty fps", previousPTS: 1_000_000, currentPTS: 1_033_333, want: 33_333 * time.Microsecond},
		{name: "first frame", previousPTS: -1, currentPTS: 1_000_000, want: scrcpyNominalFrameDuration},
		{name: "non monotonic", previousPTS: 1_000_000, currentPTS: 900_000, want: scrcpyNominalFrameDuration},
		{name: "static screen gap", previousPTS: 1_000_000, currentPTS: 1_600_000, want: scrcpyNominalFrameDuration},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := scrcpySampleDuration(test.previousPTS, test.currentPTS); got != test.want {
				t.Fatalf("duration = %s, want %s", got, test.want)
			}
		})
	}
}

func writeScrcpyPacket(t *testing.T, stream *bytes.Buffer, ptsAndFlags uint64, data []byte) {
	t.Helper()
	var header [12]byte
	binary.BigEndian.PutUint64(header[:8], ptsAndFlags)
	binary.BigEndian.PutUint32(header[8:], uint32(len(data)))
	if _, err := stream.Write(header[:]); err != nil {
		t.Fatal(err)
	}
	if _, err := stream.Write(data); err != nil {
		t.Fatal(err)
	}
}
