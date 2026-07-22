package deviceruntime

import (
	"context"
	"log"
	"sync"
	"sync/atomic"
	"time"

	"github.com/pion/webrtc/v4"
	"github.com/pion/webrtc/v4/pkg/media"
)

const (
	scrcpyIdleGrace            = 3 * time.Second
	scrcpyNominalFrameDuration = time.Second / 60
	scrcpyMaxRealtimeFrameGap  = 50 * time.Millisecond
	subscriberQueueLength      = 64
	maxSubscriberQueuedVideo   = 50 * time.Millisecond
	maxCachedGOPSamples        = 60
)

type encodedVideoSample struct {
	data     []byte
	duration time.Duration
	keyFrame bool
}

type videoSubscriber struct {
	id                 string
	track              *webrtc.TrackLocalStaticSample
	samples            chan encodedVideoSample
	stop               chan struct{}
	done               chan struct{}
	stopOnce           sync.Once
	queuedDuration     atomic.Int64
	waitingForKeyFrame bool
	cancelSession      context.CancelFunc
}

func newVideoSubscriber(id string, track *webrtc.TrackLocalStaticSample, cancelSession context.CancelFunc) *videoSubscriber {
	subscriber := &videoSubscriber{
		id:                 id,
		track:              track,
		samples:            make(chan encodedVideoSample, subscriberQueueLength),
		stop:               make(chan struct{}),
		done:               make(chan struct{}),
		waitingForKeyFrame: true,
		cancelSession:      cancelSession,
	}
	go subscriber.writeLoop()
	return subscriber
}

func (s *videoSubscriber) writeLoop() {
	defer close(s.done)
	for {
		select {
		case <-s.stop:
			return
		default:
		}
		select {
		case <-s.stop:
			return
		case sample := <-s.samples:
			s.queuedDuration.Add(-int64(sampleDuration(sample)))
			if err := s.track.WriteSample(media.Sample{Data: sample.data, Duration: sample.duration}); err != nil {
				log.Printf("device runtime WebRTC subscriber %s stopped writing: %v", s.id, err)
				s.cancelSession()
				return
			}
		}
	}
}

func sampleDuration(sample encodedVideoSample) time.Duration {
	if sample.duration <= 0 {
		return time.Second / 30
	}
	return sample.duration
}

func (s *videoSubscriber) drain() {
	for {
		select {
		case sample := <-s.samples:
			s.queuedDuration.Add(-int64(sampleDuration(sample)))
		default:
			return
		}
	}
}

func (s *videoSubscriber) enqueue(sample encodedVideoSample) bool {
	duration := sampleDuration(sample)
	if time.Duration(s.queuedDuration.Load())+duration > maxSubscriberQueuedVideo {
		s.drain()
		return false
	}
	s.queuedDuration.Add(int64(duration))
	select {
	case s.samples <- sample:
		return true
	default:
		s.queuedDuration.Add(-int64(duration))
		s.drain()
		return false
	}
}

func (s *videoSubscriber) close() {
	s.stopOnce.Do(func() { close(s.stop) })
	select {
	case <-s.done:
	case <-time.After(2 * time.Second):
	}
}

type scrcpyBroadcaster struct {
	adb        ADB
	serverPath string
	version    string
	idleGrace  time.Duration
	mu         sync.Mutex
	sources    map[string]*scrcpyVideoSource
}

func newScrcpyBroadcaster(adb ADB, serverPath, version string) *scrcpyBroadcaster {
	return &scrcpyBroadcaster{
		adb:        adb,
		serverPath: serverPath,
		version:    version,
		idleGrace:  scrcpyIdleGrace,
		sources:    make(map[string]*scrcpyVideoSource),
	}
}

func (b *scrcpyBroadcaster) source(serial string) *scrcpyVideoSource {
	b.mu.Lock()
	defer b.mu.Unlock()
	source := b.sources[serial]
	if source == nil {
		source = &scrcpyVideoSource{
			broadcaster: b,
			serial:      serial,
			subscribers: make(map[string]*videoSubscriber),
		}
		b.sources[serial] = source
	}
	return source
}

func (b *scrcpyBroadcaster) subscribe(
	serial string,
	sessionID string,
	track *webrtc.TrackLocalStaticSample,
	cancelSession context.CancelFunc,
) func() {
	return b.source(serial).subscribe(sessionID, track, cancelSession)
}

type scrcpyVideoSource struct {
	broadcaster   *scrcpyBroadcaster
	serial        string
	mu            sync.Mutex
	subscribers   map[string]*videoSubscriber
	cachedGOP     []encodedVideoSample
	running       bool
	stopProducer  func()
	cancelStartup context.CancelFunc
	stopRequested bool
	idleTimer     *time.Timer
	frames        uint64
	starts        uint64
	stops         uint64
}

func (s *scrcpyVideoSource) subscribe(
	sessionID string,
	track *webrtc.TrackLocalStaticSample,
	cancelSession context.CancelFunc,
) func() {
	subscriber := newVideoSubscriber(sessionID, track, cancelSession)
	s.mu.Lock()
	if s.idleTimer != nil {
		s.idleTimer.Stop()
		s.idleTimer = nil
	}
	s.stopRequested = false
	s.subscribers[sessionID] = subscriber
	if len(s.cachedGOP) > 0 && len(s.cachedGOP) <= cap(subscriber.samples) {
		bootstrapped := true
		for _, sample := range s.cachedGOP {
			sample.duration = time.Millisecond
			if !subscriber.enqueue(sample) {
				bootstrapped = false
				break
			}
		}
		subscriber.waitingForKeyFrame = !bootstrapped
	}
	if !s.running {
		s.running = true
		go s.run()
	}
	s.mu.Unlock()

	var once sync.Once
	return func() {
		once.Do(func() {
			s.mu.Lock()
			delete(s.subscribers, sessionID)
			if len(s.subscribers) == 0 {
				s.idleTimer = time.AfterFunc(s.broadcaster.idleGrace, s.stopIfIdle)
			}
			s.mu.Unlock()
			subscriber.close()
		})
	}
}

func (s *scrcpyVideoSource) stopIfIdle() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.subscribers) != 0 || !s.running {
		return
	}
	s.stopRequested = true
	if s.cancelStartup != nil {
		s.cancelStartup()
	}
	if s.stopProducer != nil {
		s.stopProducer()
	}
}

func (s *scrcpyVideoSource) run() {
	for {
		startupContext, cancelStartup := context.WithTimeout(context.Background(), 10*time.Second)
		s.mu.Lock()
		s.starts++
		s.cancelStartup = cancelStartup
		if s.stopRequested && len(s.subscribers) == 0 {
			cancelStartup()
		}
		s.mu.Unlock()
		stream, stop, cleanup, err := s.broadcaster.adb.ScrcpyH264(
			startupContext,
			s.serial,
			s.broadcaster.serverPath,
			s.broadcaster.version,
		)
		cancelStartup()
		s.mu.Lock()
		s.cancelStartup = nil
		s.mu.Unlock()
		if err != nil {
			if !s.shouldRestart() {
				return
			}
			log.Printf("device runtime scrcpy producer for %s failed to start: %v", s.serial, err)
			time.Sleep(time.Second)
			continue
		}

		s.mu.Lock()
		s.stopProducer = stop
		s.cachedGOP = nil
		if s.stopRequested && len(s.subscribers) == 0 {
			stop()
		} else {
			s.stopRequested = false
		}
		s.mu.Unlock()

		err = readScrcpySamples(stream, s.broadcast)
		cleanup()
		s.mu.Lock()
		s.stops++
		s.stopProducer = nil
		s.mu.Unlock()
		if err != nil && s.hasSubscribers() {
			log.Printf("device runtime scrcpy producer for %s stopped: %v", s.serial, err)
		}
		if !s.shouldRestart() {
			return
		}
		time.Sleep(250 * time.Millisecond)
	}
}

func (s *scrcpyVideoSource) shouldRestart() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.subscribers) == 0 {
		s.running = false
		s.stopRequested = false
		return false
	}
	s.stopRequested = false
	return true
}

func (s *scrcpyVideoSource) hasSubscribers() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.subscribers) > 0
}

func (s *scrcpyVideoSource) broadcast(sample encodedVideoSample) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.frames++
	if sample.keyFrame {
		s.cachedGOP = append(s.cachedGOP[:0], sample)
	} else if len(s.cachedGOP) > 0 {
		s.cachedGOP = append(s.cachedGOP, sample)
		if len(s.cachedGOP) > maxCachedGOPSamples {
			s.cachedGOP = nil
		}
	}
	for _, subscriber := range s.subscribers {
		if subscriber.waitingForKeyFrame && !sample.keyFrame {
			continue
		}
		if sample.keyFrame {
			subscriber.waitingForKeyFrame = false
		}
		if !subscriber.enqueue(sample) {
			subscriber.waitingForKeyFrame = true
			if sample.keyFrame {
				subscriber.waitingForKeyFrame = false
				if !subscriber.enqueue(sample) {
					subscriber.waitingForKeyFrame = true
				}
			}
		}
	}
}

type scrcpySourceStatus struct {
	Running     bool   `json:"running"`
	Subscribers int    `json:"subscribers"`
	Frames      uint64 `json:"frames"`
	Starts      uint64 `json:"starts"`
	Stops       uint64 `json:"stops"`
}

func (b *scrcpyBroadcaster) status() map[string]scrcpySourceStatus {
	b.mu.Lock()
	sources := make(map[string]*scrcpyVideoSource, len(b.sources))
	for serial, source := range b.sources {
		sources[serial] = source
	}
	b.mu.Unlock()
	status := make(map[string]scrcpySourceStatus, len(sources))
	for serial, source := range sources {
		source.mu.Lock()
		status[serial] = scrcpySourceStatus{
			Running:     source.running,
			Subscribers: len(source.subscribers),
			Frames:      source.frames,
			Starts:      source.starts,
			Stops:       source.stops,
		}
		source.mu.Unlock()
	}
	return status
}

func readScrcpySamples(stream interface {
	Read([]byte) (int, error)
}, broadcast func(encodedVideoSample)) error {
	reader := newScrcpyPacketReader(stream)
	var codecConfig []byte
	var previousPTS int64 = -1
	for {
		packet, err := reader.Next()
		if err != nil {
			return err
		}
		if packet.Config {
			codecConfig = append(codecConfig[:0], packet.Data...)
			continue
		}
		data := packet.Data
		if packet.KeyFrame && len(codecConfig) > 0 {
			data = make([]byte, 0, len(codecConfig)+len(packet.Data))
			data = append(data, codecConfig...)
			data = append(data, packet.Data...)
		}
		duration := scrcpySampleDuration(previousPTS, packet.PTS)
		previousPTS = packet.PTS
		broadcast(encodedVideoSample{data: data, duration: duration, keyFrame: packet.KeyFrame})
	}
}

func scrcpySampleDuration(previousPTS, currentPTS int64) time.Duration {
	if previousPTS < 0 || currentPTS <= previousPTS {
		return scrcpyNominalFrameDuration
	}
	duration := time.Duration(currentPTS-previousPTS) * time.Microsecond
	if duration < time.Millisecond || duration > scrcpyMaxRealtimeFrameGap {
		return scrcpyNominalFrameDuration
	}
	return duration
}
