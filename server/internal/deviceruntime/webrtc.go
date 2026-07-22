package deviceruntime

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/pion/webrtc/v4"
)

type localWebRTCTransport struct {
	api *webrtc.API
}

func newLocalWebRTCTransport() (*localWebRTCTransport, error) {
	settingEngine := webrtc.SettingEngine{}
	settingEngine.SetIncludeLoopbackCandidate(true)
	settingEngine.SetNetworkTypes([]webrtc.NetworkType{webrtc.NetworkTypeUDP4})
	return &localWebRTCTransport{
		api: webrtc.NewAPI(webrtc.WithSettingEngine(settingEngine)),
	}, nil
}

type webRTCSessionStore struct {
	mu       sync.Mutex
	sessions map[string]*webRTCSession
}

type webRTCSession struct {
	id     string
	serial string
	cancel context.CancelFunc
	done   chan struct{}
}

func newWebRTCSessionStore() *webRTCSessionStore {
	return &webRTCSessionStore{
		sessions: make(map[string]*webRTCSession),
	}
}

func (s *webRTCSessionStore) add(session *webRTCSession) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sessions[session.id] = session
}

func (s *webRTCSessionStore) close(id string) bool {
	s.mu.Lock()
	session, ok := s.sessions[id]
	s.mu.Unlock()
	if ok {
		session.cancel()
	}
	return ok
}

func (s *webRTCSessionStore) remove(session *webRTCSession) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.sessions, session.id)
}

func (s *webRTCSessionStore) count() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.sessions)
}

func (s *webRTCSessionStore) snapshot() map[string]string {
	s.mu.Lock()
	defer s.mu.Unlock()
	sessions := make(map[string]string, len(s.sessions))
	for id, session := range s.sessions {
		sessions[id] = session.serial
	}
	return sessions
}

func (s Server) webrtcOffer(w http.ResponseWriter, r *http.Request) {
	if s.ScrcpyServer == "" {
		writeError(w, fmt.Errorf("WebRTC video requires --scrcpy-server"))
		return
	}
	var request struct {
		Serial string                    `json:"serial"`
		Offer  webrtc.SessionDescription `json:"offer"`
	}
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil || request.Serial == "" || request.Offer.SDP == "" {
		writeError(w, fmt.Errorf("serial and offer are required"))
		return
	}

	if !s.acquireDeviceLease(w, r, request.Serial) {
		return
	}

	if s.webRTC == nil {
		writeError(w, fmt.Errorf("WebRTC transport is unavailable"))
		return
	}
	peer, err := s.webRTC.api.NewPeerConnection(webrtc.Configuration{})
	if err != nil {
		writeError(w, err)
		return
	}
	handedOff := false
	defer func() {
		if !handedOff {
			_ = peer.Close()
		}
	}()
	track, err := webrtc.NewTrackLocalStaticSample(webrtc.RTPCodecCapability{
		MimeType:    webrtc.MimeTypeH264,
		SDPFmtpLine: "level-asymmetry-allowed=1;packetization-mode=1;profile-level-id=42e01f",
	}, "android-video", "multica-device")
	if err != nil {
		writeError(w, err)
		return
	}
	sender, err := peer.AddTrack(track)
	if err != nil {
		writeError(w, err)
		return
	}
	go func() {
		buffer := make([]byte, 1500)
		for {
			if _, _, readErr := sender.Read(buffer); readErr != nil {
				return
			}
		}
	}()
	if err = peer.SetRemoteDescription(request.Offer); err != nil {
		writeError(w, err)
		return
	}
	answer, err := peer.CreateAnswer(nil)
	if err != nil {
		writeError(w, err)
		return
	}
	gatherComplete := webrtc.GatheringCompletePromise(peer)
	if err = peer.SetLocalDescription(answer); err != nil {
		writeError(w, err)
		return
	}
	select {
	case <-gatherComplete:
	case <-r.Context().Done():
		writeError(w, r.Context().Err())
		return
	case <-time.After(5 * time.Second):
		writeError(w, fmt.Errorf("WebRTC ICE gathering timed out"))
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	sessionID := uuid.NewString()
	session := &webRTCSession{id: sessionID, serial: request.Serial, cancel: cancel, done: make(chan struct{})}
	if s.sessions != nil {
		s.sessions.add(session)
	}
	connected := make(chan struct{})
	var connectedOnce sync.Once
	peer.OnConnectionStateChange(func(state webrtc.PeerConnectionState) {
		switch state {
		case webrtc.PeerConnectionStateConnected:
			connectedOnce.Do(func() { close(connected) })
		case webrtc.PeerConnectionStateFailed, webrtc.PeerConnectionStateClosed, webrtc.PeerConnectionStateDisconnected:
			cancel()
		}
	})
	go func() {
		defer func() {
			cancel()
			_ = peer.Close()
			close(session.done)
			if s.sessions != nil {
				s.sessions.remove(session)
			}
		}()
		select {
		case <-connected:
		case <-ctx.Done():
			return
		case <-time.After(30 * time.Second):
			log.Printf("device runtime WebRTC peer for %s did not connect", request.Serial)
			return
		}
		if s.broadcaster == nil {
			log.Printf("device runtime WebRTC broadcaster for %s is unavailable", request.Serial)
			return
		}
		unsubscribe := s.broadcaster.subscribe(request.Serial, sessionID, track, cancel)
		defer unsubscribe()
		<-ctx.Done()
	}()
	handedOff = true
	writeJSON(w, http.StatusOK, map[string]any{
		"answer":     *peer.LocalDescription(),
		"session_id": sessionID,
	})
}

func (s Server) webrtcClose(w http.ResponseWriter, r *http.Request) {
	var request struct {
		SessionID string `json:"session_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil || request.SessionID == "" {
		writeError(w, fmt.Errorf("session_id is required"))
		return
	}
	closed := s.sessions != nil && s.sessions.close(request.SessionID)
	writeJSON(w, http.StatusOK, map[string]bool{"closed": closed})
}

func (s Server) webrtcStatus(w http.ResponseWriter, _ *http.Request) {
	sources := map[string]scrcpySourceStatus{}
	if s.broadcaster != nil {
		sources = s.broadcaster.status()
	}
	sessions := map[string]string{}
	if s.sessions != nil {
		sessions = s.sessions.snapshot()
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"session_count": len(sessions),
		"sessions":      sessions,
		"sources":       sources,
	})
}

func (a ADB) ScrcpyH264(ctx context.Context, serial, serverPath, version string) (io.ReadCloser, func(), func(), error) {
	if _, err := a.run(ctx, "-s", serial, "push", serverPath, "/data/local/tmp/multica-scrcpy-server.jar"); err != nil {
		return nil, nil, nil, err
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, nil, nil, err
	}
	port := listener.Addr().(*net.TCPAddr).Port
	_ = listener.Close()
	var scidBytes [4]byte
	if _, err := rand.Read(scidBytes[:]); err != nil {
		return nil, nil, nil, fmt.Errorf("generate scrcpy session id: %w", err)
	}
	scid := binary.BigEndian.Uint32(scidBytes[:]) & 0x7fffffff
	socketName := fmt.Sprintf("scrcpy_%08x", scid)
	forward := "tcp:" + strconv.Itoa(port)
	removeADBForward(a, serial, forward)
	if _, err := a.run(ctx, "-s", serial, "forward", forward, "localabstract:"+socketName); err != nil {
		return nil, nil, nil, err
	}
	command := scrcpyServerCommand(version, scid)
	// The startup context only bounds discovery and connection setup. Once the
	// socket is connected, the producer owns the Android process lifecycle.
	process := exec.Command(a.binary(), "-s", serial, "shell", command)
	var processOutput lockedBuffer
	process.Stdout = &processOutput
	process.Stderr = &processOutput
	if err := process.Start(); err != nil {
		removeADBForward(a, serial, forward)
		return nil, nil, nil, err
	}
	processDone := make(chan error, 1)
	go func() { processDone <- process.Wait() }()

	address := "127.0.0.1:" + strconv.Itoa(port)
	deadline := time.Now().Add(10 * time.Second)
	var connection net.Conn
	for time.Now().Before(deadline) {
		select {
		case processErr := <-processDone:
			removeADBForward(a, serial, forward)
			return nil, nil, nil, fmt.Errorf("scrcpy server exited before video was ready: %w: %s", processErr, processOutput.String())
		default:
		}

		if err := ctx.Err(); err != nil {
			break
		}
		sockets, socketErr := a.run(ctx, "-s", serial, "shell", "cat", "/proc/net/unix")
		if socketErr != nil || !bytes.Contains(sockets, []byte(socketName)) {
			select {
			case <-ctx.Done():
			case <-time.After(100 * time.Millisecond):
			}
			continue
		}
		connection, err = net.DialTimeout("tcp", address, 500*time.Millisecond)
		if err == nil {
			break
		}
		connection = nil
		if ctx.Err() != nil {
			break
		}
		select {
		case <-ctx.Done():
		case <-time.After(100 * time.Millisecond):
		}
	}
	if connection == nil {
		_ = process.Process.Kill()
		select {
		case <-processDone:
		case <-time.After(2 * time.Second):
		}
		removeADBForward(a, serial, forward)
		if ctxErr := ctx.Err(); ctxErr != nil {
			err = ctxErr
		}
		if err == nil {
			err = fmt.Errorf("timed out waiting for scrcpy video socket")
		}
		return nil, nil, nil, fmt.Errorf("connect scrcpy video socket: %w: %s", err, processOutput.String())
	}
	controlConnection, controlErr := net.DialTimeout("tcp", address, time.Second)
	if controlErr != nil {
		_ = connection.Close()
		_ = process.Process.Kill()
		select {
		case <-processDone:
		case <-time.After(2 * time.Second):
		}
		removeADBForward(a, serial, forward)
		return nil, nil, nil, fmt.Errorf("connect scrcpy control socket: %w: %s", controlErr, processOutput.String())
	}
	control := newScrcpyControlClient(controlConnection)
	if a.runtime != nil {
		a.runtime.controls.set(serial, control)
	}
	go func() {
		_, _ = io.Copy(io.Discard, controlConnection)
		if a.runtime != nil {
			a.runtime.controls.remove(serial, control)
		}
	}()
	var stopOnce sync.Once
	stop := func() {
		stopOnce.Do(func() {
			if a.runtime != nil {
				a.runtime.controls.remove(serial, control)
			}
			control.close()
			stopContext, stopCancel := context.WithTimeout(context.Background(), 3*time.Second)
			_, _ = a.run(stopContext, "-s", serial, "shell", "pkill", "-f", fmt.Sprintf("scid=%08x", scid))
			stopCancel()
			removeADBForward(a, serial, forward)
			_ = connection.Close()
		})
	}
	cleanup := func() {
		if a.runtime != nil {
			a.runtime.controls.remove(serial, control)
		}
		control.close()
		_ = connection.Close()
		select {
		case <-processDone:
		default:
			_ = process.Process.Kill()
			select {
			case <-processDone:
			case <-time.After(2 * time.Second):
			}
		}
		removeADBForward(a, serial, forward)
	}
	return connection, stop, cleanup, nil
}

func scrcpyServerCommand(version string, scid uint32) string {
	return strings.Join([]string{
		"CLASSPATH=/data/local/tmp/multica-scrcpy-server.jar", "app_process / com.genymobile.scrcpy.Server", version,
		fmt.Sprintf("scid=%08x", scid), "tunnel_forward=true", "audio=false", "control=true", "clipboard_autosync=false", "cleanup=false",
		"send_device_meta=false", "send_frame_meta=true", "send_dummy_byte=false", "send_codec_meta=false", "max_size=1024",
		"max_fps=60", "video_bit_rate=4000000",
		"video_codec_options=i-frame-interval=1,repeat-previous-frame-after=16666,priority=0,operating-rate=60",
	}, " ")
}

func removeADBForward(adb ADB, serial, forward string) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_, _ = adb.run(ctx, "-s", serial, "forward", "--remove", forward)
}

type lockedBuffer struct {
	mu   sync.Mutex
	data bytes.Buffer
}

func (b *lockedBuffer) Write(data []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.data.Write(data)
}

func (b *lockedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return strings.TrimSpace(b.data.String())
}

const (
	scrcpyConfigFlag = uint64(1) << 63
	scrcpyKeyFlag    = uint64(1) << 62
	scrcpyPTSMask    = scrcpyKeyFlag - 1
	maxScrcpyPacket  = 32 << 20
)

type scrcpyPacket struct {
	Data     []byte
	PTS      int64
	Config   bool
	KeyFrame bool
}

type scrcpyPacketReader struct {
	reader io.Reader
}

func newScrcpyPacketReader(reader io.Reader) *scrcpyPacketReader {
	return &scrcpyPacketReader{reader: reader}
}

func (r *scrcpyPacketReader) Next() (scrcpyPacket, error) {
	var header [12]byte
	if _, err := io.ReadFull(r.reader, header[:]); err != nil {
		return scrcpyPacket{}, err
	}
	ptsAndFlags := binary.BigEndian.Uint64(header[:8])
	size := binary.BigEndian.Uint32(header[8:])
	if size == 0 || size > maxScrcpyPacket {
		return scrcpyPacket{}, fmt.Errorf("invalid scrcpy video packet size %d", size)
	}
	data := make([]byte, size)
	if _, err := io.ReadFull(r.reader, data); err != nil {
		return scrcpyPacket{}, err
	}
	return scrcpyPacket{
		Data:     data,
		PTS:      int64(ptsAndFlags & scrcpyPTSMask),
		Config:   ptsAndFlags&scrcpyConfigFlag != 0,
		KeyFrame: ptsAndFlags&scrcpyKeyFlag != 0,
	}, nil
}
