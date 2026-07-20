package deviceruntime

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
)

type Server struct {
	ADB           ADB
	Artifact      string
	Package       string
	Component     string
	ScrcpyServer  string
	ScrcpyVersion string
	AccessToken   string
	deployments   *deploymentStore
	access        *runtimeAccessStore
	leases        *runtimeLeaseStore
	webRTC        *localWebRTCTransport
	sessions      *webRTCSessionStore
	broadcaster   *scrcpyBroadcaster
	webViews      *webViewSessionStore
}

func (s Server) Handler() http.Handler {
	s.ADB = s.ADB.withRuntimeState()
	s.webViews = newWebViewSessionStore()
	s.deployments = newDeploymentStore()
	s.access = newRuntimeAccessStore()
	s.leases = newRuntimeLeaseStore()
	if s.ScrcpyServer != "" {
		transport, err := newLocalWebRTCTransport()
		if err != nil {
			panic(fmt.Sprintf("initialize WebRTC transport: %v", err))
		}
		s.webRTC = transport
		s.sessions = newWebRTCSessionStore()
		s.broadcaster = newScrcpyBroadcaster(s.ADB, s.ScrcpyServer, s.ScrcpyVersion)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /", s.console)
	mux.HandleFunc("GET /api/devices", s.devices)
	mux.HandleFunc("POST /api/deploy", s.deploy)
	mux.HandleFunc("POST /api/deploy/jobs", s.createDeployment)
	mux.HandleFunc("GET /api/deploy/jobs/{id}", s.getDeployment)
	mux.HandleFunc("DELETE /api/deploy/jobs/{id}", s.cancelDeployment)
	mux.HandleFunc("GET /api/screenshot", s.screenshot)
	mux.HandleFunc("GET /api/stream", s.stream)
	mux.HandleFunc("POST /api/webrtc/offer", s.webrtcOffer)
	mux.HandleFunc("POST /api/webrtc/close", s.webrtcClose)
	mux.HandleFunc("GET /api/webrtc/status", s.webrtcStatus)
	mux.HandleFunc("POST /api/input", s.input)
	mux.HandleFunc("GET /api/webview/snapshot", s.webViewSnapshot)
	mux.HandleFunc("POST /api/webview/action", s.webViewAction)
	mux.HandleFunc("GET /api/logs", s.logs)
	mux.HandleFunc("POST /api/clear", s.clear)
	return s.protectAPI(mux)
}

func (s Server) deployContext(r *http.Request) (context.Context, context.CancelFunc) {
	return context.WithTimeout(r.Context(), 2*time.Minute)
}

func (s Server) requestContext(r *http.Request) (context.Context, context.CancelFunc) {
	return context.WithTimeout(r.Context(), 45*time.Second)
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeError(w http.ResponseWriter, err error) {
	writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
}

func (s Server) devices(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := s.requestContext(r)
	defer cancel()
	devices, err := s.ADB.Devices(ctx)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"devices": devices})
}

func (s Server) deploy(w http.ResponseWriter, r *http.Request) {
	var request deploymentRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil || request.Serial == "" {
		writeError(w, fmt.Errorf("serial is required"))
		return
	}
	ctx, cancel := s.deployContext(r)
	defer cancel()
	artifact := s.Artifact
	if strings.TrimSpace(request.Artifact) != "" {
		artifact = strings.TrimSpace(request.Artifact)
	}
	request.Artifact = artifact
	if err := validateAPKArtifact(artifact); err != nil {
		writeError(w, err)
		return
	}
	webURL, err := validateWebPreviewURL(request.WebURL)
	if err != nil {
		writeError(w, err)
		return
	}
	request.WebURL = webURL
	hash, err := artifactSHA256(artifact)
	if err != nil {
		writeError(w, err)
		return
	}
	sessionID := runtimeSessionID(r)
	if sessionID == "" {
		sessionID = strings.TrimSpace(request.SessionID)
	}
	previous, err := s.leases.acquire(sessionID, request.Serial)
	if err != nil {
		writeJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
		return
	}
	skipped, err := s.runDeployment(ctx, request, artifact, hash, func(string) {})
	if err != nil {
		s.leases.restore(sessionID, previous, request.Serial)
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "running", "phase": "ready", "artifact_sha256": hash, "install_skipped": skipped})
}

func (s Server) createDeployment(w http.ResponseWriter, r *http.Request) {
	var request deploymentRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil || strings.TrimSpace(request.Serial) == "" {
		writeError(w, fmt.Errorf("serial is required"))
		return
	}
	request.Serial = strings.TrimSpace(request.Serial)
	artifact := s.Artifact
	if strings.TrimSpace(request.Artifact) != "" {
		artifact = strings.TrimSpace(request.Artifact)
	}
	if err := validateAPKArtifact(artifact); err != nil {
		writeError(w, err)
		return
	}
	webURL, err := validateWebPreviewURL(request.WebURL)
	if err != nil {
		writeError(w, err)
		return
	}
	request.Artifact, request.WebURL = artifact, webURL
	hash, err := artifactSHA256(artifact)
	if err != nil {
		writeError(w, err)
		return
	}
	sessionID := runtimeSessionID(r)
	if sessionID == "" {
		sessionID = strings.TrimSpace(request.SessionID)
	}
	previous, err := s.leases.acquire(sessionID, request.Serial)
	if err != nil {
		writeJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
		return
	}
	snapshot, err := s.deployments.launch(sessionID, request.Serial, hash, func(ctx context.Context, update func(string), artifactHash string) (bool, error) {
		skipped, runErr := s.runDeployment(ctx, request, artifact, artifactHash, update)
		if runErr != nil {
			s.leases.restore(sessionID, previous, request.Serial)
		}
		return skipped, runErr
	})
	if err != nil {
		s.leases.restore(sessionID, previous, request.Serial)
		writeJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusAccepted, snapshot)
}

func (s Server) getDeployment(w http.ResponseWriter, r *http.Request) {
	snapshot, ok := s.deployments.snapshotFor(r.PathValue("id"), runtimeSessionID(r))
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "deployment not found"})
		return
	}
	writeJSON(w, http.StatusOK, snapshot)
}

func (s Server) cancelDeployment(w http.ResponseWriter, r *http.Request) {
	snapshot, ok := s.deployments.cancelFor(r.PathValue("id"), runtimeSessionID(r))
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "deployment not found"})
		return
	}
	writeJSON(w, http.StatusOK, snapshot)
}

func validateWebPreviewURL(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", nil
	}
	parsed, err := url.ParseRequestURI(raw)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" || parsed.User != nil {
		return "", fmt.Errorf("web_url must be an HTTP(S) URL without credentials")
	}
	return parsed.String(), nil
}

func validateAPKArtifact(artifact string) error {
	if !filepath.IsAbs(artifact) {
		return fmt.Errorf("artifact must be an absolute path")
	}
	if !strings.EqualFold(filepath.Ext(artifact), ".apk") {
		return fmt.Errorf("artifact must be an APK file")
	}
	info, err := os.Stat(artifact)
	if err != nil {
		return fmt.Errorf("read artifact: %w", err)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("artifact must be a regular file")
	}
	return nil
}

func (s Server) screenshot(w http.ResponseWriter, r *http.Request) {
	serial := r.URL.Query().Get("serial")
	if serial == "" {
		writeError(w, fmt.Errorf("serial is required"))
		return
	}
	if !s.authorizeDeviceLease(w, r, serial) {
		return
	}

	ctx, cancel := s.requestContext(r)
	defer cancel()
	png, err := s.ADB.Screenshot(ctx, serial)
	if err != nil {
		writeError(w, err)
		return
	}
	w.Header().Set("Content-Type", "image/png")
	_, _ = w.Write(png)
}

func (s Server) stream(w http.ResponseWriter, r *http.Request) {
	serial := r.URL.Query().Get("serial")
	if serial == "" {
		writeError(w, fmt.Errorf("serial is required"))
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, fmt.Errorf("streaming is unavailable"))
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "multipart/x-mixed-replace; boundary=frame")
	ticker := time.NewTicker(180 * time.Millisecond)
	defer ticker.Stop()
	for {
		ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
		png, err := s.ADB.Screenshot(ctx, serial)
		cancel()
		if err != nil {
			return
		}
		_, _ = fmt.Fprintf(w, "--frame\r\nContent-Type: image/png\r\nContent-Length: %d\r\n\r\n", len(png))
		_, _ = w.Write(png)
		_, _ = w.Write([]byte("\r\n"))
		flusher.Flush()
		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
		}
	}
}

func (s Server) input(w http.ResponseWriter, r *http.Request) {
	var request struct {
		Serial string `json:"serial"`
		Type   string `json:"type"`
		X      int    `json:"x"`
		Y      int    `json:"y"`
		EndX   int    `json:"end_x"`
		EndY   int    `json:"end_y"`
		Key    string `json:"key"`
		Text   string `json:"text"`
		Width  int    `json:"source_width"`
		Height int    `json:"source_height"`
	}
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil || request.Serial == "" {
		writeError(w, fmt.Errorf("serial is required"))
		return
	}
	if !s.authorizeDeviceLease(w, r, request.Serial) {
		return
	}

	ctx, cancel := s.requestContext(r)
	defer cancel()
	var err error
	switch request.Type {
	case "tap":
		err = s.ADB.TapAt(ctx, request.Serial, request.X, request.Y, request.Width, request.Height)
	case "swipe":
		err = s.ADB.SwipeAt(ctx, request.Serial, request.X, request.Y, request.EndX, request.EndY, request.Width, request.Height)
	case "key":
		err = s.ADB.Key(ctx, request.Serial, request.Key)
	case "rotate":
		err = s.ADB.Rotate(ctx, request.Serial)
	case "text":
		err = s.ADB.Text(ctx, request.Serial, request.Text)
	default:
		err = fmt.Errorf("unsupported input type")
	}
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func scaleCoordinate(value, sourceSize, targetSize int) int {
	if sourceSize <= 0 || targetSize <= 0 {
		return value
	}
	value = max(0, min(value, sourceSize))
	return (value*targetSize + sourceSize/2) / sourceSize
}

func (s Server) logs(w http.ResponseWriter, r *http.Request) {
	serial := r.URL.Query().Get("serial")
	if serial == "" {
		writeError(w, fmt.Errorf("serial is required"))
		return
	}
	if !s.authorizeDeviceLease(w, r, serial) {
		return
	}

	ctx, cancel := s.requestContext(r)
	defer cancel()
	logs, err := s.ADB.Logs(ctx, serial, s.Package)
	if err != nil {
		writeError(w, err)
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = w.Write(logs)
}

func (s Server) clear(w http.ResponseWriter, r *http.Request) {
	var request struct {
		Serial string `json:"serial"`
	}
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil || request.Serial == "" {
		writeError(w, fmt.Errorf("serial is required"))
		return
	}
	if !s.authorizeDeviceLease(w, r, request.Serial) {
		return
	}

	ctx, cancel := s.requestContext(r)
	defer cancel()
	if err := s.ADB.Clear(ctx, request.Serial, s.Package); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "cleared"})
}

func (s Server) console(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	sessionID := strings.TrimSpace(r.URL.Query().Get("session_id"))
	if sessionID == "" {
		sessionID = "console-" + uuid.NewString()
	}
	token := s.access.mint(sessionID)
	bootstrap := `<script>window.__MULTICA_DEVICE_TOKEN=` + strconv.Quote(token) + `;(function(){const base=window.fetch.bind(window);window.fetch=(input,init={})=>{const headers=new Headers(init.headers||{});headers.set('X-Multica-Device-Token',window.__MULTICA_DEVICE_TOKEN);return base(input,{...init,headers})}})();</script>`
	if s.ScrcpyServer != "" {
		html := strings.Replace(consoleWebRTC, "</body>", consoleWebRTCEnhancements+consoleDeviceBridge+"</body>", 1)
		html = strings.Replace(html, "</head>", bootstrap+"</head>", 1)
		if r.URL.Query().Get("embed") == "1" {
			html = strings.Replace(html, "<body>", `<body class="embedded">`, 1)
		}
		_, _ = w.Write([]byte(html))
		return
	}
	_, _ = w.Write([]byte(strings.Replace(consoleHTML, "</head>", bootstrap+"</head>", 1)))
	_, _ = w.Write([]byte(consoleEnhancements))
}

const consoleHTML = `<!doctype html><html><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>Multica Device Runtime</title><style>
:root{--bg:#111412;--rail:#191d1a;--surface:#222722;--line:#394139;--ink:#f3f5ef;--muted:#9ca69b;--accent:#6bdb82;--danger:#ff8d86}*{box-sizing:border-box}body{margin:0;background:var(--bg);color:var(--ink);font-family:ui-monospace,SFMono-Regular,Menlo,monospace}.app{height:100vh;min-height:680px;display:grid;grid-template-columns:66px minmax(420px,1fr) 330px}.rail{background:var(--rail);border-right:1px solid var(--line);padding:14px 10px;display:flex;flex-direction:column;gap:10px}.mark{height:34px;display:grid;place-items:center;color:var(--accent);font:700 18px Georgia,serif;border-bottom:1px solid var(--line);padding-bottom:12px;margin-bottom:8px}.tool{height:42px;border:1px solid transparent;border-radius:6px;background:transparent;color:var(--muted);font-size:19px;cursor:pointer}.tool:hover{background:#2a302a;color:var(--ink)}.tool.danger:hover{color:var(--danger)}.workspace{min-width:0;display:flex;flex-direction:column}.bar{height:70px;padding:0 24px;border-bottom:1px solid var(--line);display:flex;align-items:center;gap:18px}.title{font:600 15px Georgia,serif}.sub{font-size:11px;color:var(--muted);margin-top:4px}.online{margin-left:auto;color:var(--accent);font-size:11px}.canvas{flex:1;min-height:0;display:grid;place-items:center;padding:26px;background:#151916}.device{height:min(78vh,860px);max-width:calc(100% - 20px);aspect-ratio:9/19.5;background:#070807;border:8px solid #353d35;border-radius:26px;padding:5px;box-shadow:0 22px 50px #0008}.screen{width:100%;height:100%;object-fit:contain;display:block;cursor:crosshair;border-radius:16px;user-select:none;-webkit-user-drag:none}.inspector{background:var(--rail);border-left:1px solid var(--line);padding:20px;display:flex;flex-direction:column;gap:20px;overflow:auto}.section{border-bottom:1px solid var(--line);padding:20px;display:flex;flex-direction:column;gap:20px;overflow:auto}.section{border-bottom:1px solid var(--line);padding-bottom:18px}.label{font-size:10px;color:var(--muted);letter-spacing:1px;text-transform:uppercase;margin-bottom:10px}.device-select,input{width:100%;background:var(--surface);border:1px solid var(--line);border-radius:5px;color:var(--ink);padding:10px;font:12px ui-monospace,SFMono-Regular,Menlo,monospace}.button-row{display:grid;grid-template-columns:1fr 1fr;gap:8px;margin-top:9px}button.action{border:1px solid var(--line);background:var(--surface);border-radius:5px;padding:9px;color:var(--ink);font:12px ui-monospace,SFMono-Regular,Menlo,monospace;cursor:pointer}button.primary{background:var(--accent);border-color:var(--accent);color:#102016;font-weight:700}button.action:hover{filter:brightness(1.12)}.log{margin:0;height:230px;overflow:auto;color:#b8c5b9;font:10px/1.5 ui-monospace,SFMono-Regular,Menlo,monospace;white-space:pre-wrap}.toast{min-height:18px;color:var(--muted);font-size:11px}.hint{font-size:11px;color:var(--muted);line-height:1.5}@media(max-width:760px){.app{grid-template-columns:52px 1fr}.inspector{display:none}.bar{height:56px;padding:0 14px}.canvas{padding:12px}.device{height:calc(100vh - 82px)}}
</style></head><body><main class="app"><aside class="rail"><div class="mark">M</div><button class="tool" id="back" title="Back">&larr;</button><button class="tool" id="home" title="Home">H</button><button class="tool" id="recent" title="Recent apps">[]</button><button class="tool" id="rotate" title="Rotate">R</button><button class="tool" id="capture" title="Capture screenshot">O</button><button class="tool danger" id="clear" title="Stop and clear app data">x</button></aside><section class="workspace"><header class="bar"><div><div class="title">Android device session</div><div class="sub" id="device-name">Connecting to ADB...</div></div><div class="online" id="state">&bull; waiting</div></header><div class="canvas"><div class="device"><img class="screen" id="screen" alt="Live Android device screen"></div></div></section><aside class="inspector"><section class="section"><div class="label">Device</div><select class="device-select" id="devices"></select><div class="button-row"><button class="action primary" id="deploy">Deploy build</button><button class="action" id="logs">Refresh logs</button></div></section><section class="section"><div class="label">Keyboard</div><input id="text" placeholder="Send text to focused field"><div class="button-row"><button class="action" id="send">Send text</button><button class="action" id="hide">Hide keyboard</button></div></section><section><div class="label">Session log</div><div class="toast" id="notice">Ready</div><pre class="log" id="log">Select a device to inspect logcat.</pre><p class="hint">Tap or drag directly on the device. The control layer maps your pointer to Android screen coordinates.</p></section></aside></main><script>
const state=document.querySelector('#state'),select=document.querySelector('#devices'),screen=document.querySelector('#screen'),device=document.querySelector('.device'),log=document.querySelector('#log'),notice=document.querySelector('#notice'),nameEl=document.querySelector('#device-name');let serial='',start=null;const api=async(path,options)=>{const r=await fetch(path,options);if(!r.ok)throw new Error((await r.json()).error||r.statusText);return r};function say(v,bad){notice.textContent=v;state.textContent=bad?'action failed':'online: '+v;state.style.color=bad?'var(--danger)':'var(--accent)'}function fitDevice(){if(screen.naturalWidth&&screen.naturalHeight)device.style.aspectRatio=screen.naturalWidth+'/'+screen.naturalHeight}async function devices(){try{const data=await (await api('/api/devices')).json();select.innerHTML='';for(const d of data.devices){const o=document.createElement('option');o.value=d.serial;o.textContent=(d.model||d.serial)+' / '+d.kind+' / '+d.state;select.append(o)}serial=select.value||'';nameEl.textContent=select.options[select.selectedIndex]?.textContent||'No ADB devices';if(serial){screen.src='/api/stream?serial='+encodeURIComponent(serial);say('connected')}else say('no device',true)}catch(e){say(e.message,true)}}function point(e){const r=screen.getBoundingClientRect();return{x:Math.round((e.clientX-r.left)*screen.naturalWidth/r.width),y:Math.round((e.clientY-r.top)*screen.naturalHeight/r.height)}}async function post(path,body){await api(path,{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify(body)})}async function key(k){try{await post('/api/input',{serial,type:'key',key:k});say('sent '+k)}catch(e){say(e.message,true)}}screen.onload=fitDevice;select.onchange=()=>{serial=select.value;nameEl.textContent=select.options[select.selectedIndex]?.textContent||'';screen.src='/api/stream?serial='+encodeURIComponent(serial)};screen.onpointerdown=e=>{start=point(e);screen.setPointerCapture(e.pointerId)};screen.onpointerup=async e=>{if(!start)return;const end=point(e),d=Math.hypot(end.x-start.x,end.y-start.y);try{await post('/api/input',d<12?{serial,type:'tap',x:start.x,y:start.y}:{serial,type:'swipe',x:start.x,y:start.y,end_x:end.x,end_y:end.y});say(d<12?'tap sent':'swipe sent')}catch(err){say(err.message,true)}start=null};document.querySelector('#deploy').onclick=async()=>{try{say('installing build...');await post('/api/deploy',{serial});say('build running')}catch(e){say(e.message,true)}};document.querySelector('#send').onclick=async()=>{try{const text=document.querySelector('#text').value;await post('/api/input',{serial,type:'text',text});document.querySelector('#text').value='';say('text sent')}catch(e){say(e.message,true)}};document.querySelector('#logs').onclick=async()=>{try{log.textContent=await (await api('/api/logs?serial='+encodeURIComponent(serial))).text();say('logcat refreshed')}catch(e){say(e.message,true)}};document.querySelector('#back').onclick=()=>key('4');document.querySelector('#home').onclick=()=>key('3');document.querySelector('#recent').onclick=()=>key('187');document.querySelector('#hide').onclick=()=>key('111');document.querySelector('#rotate').onclick=async()=>{try{await post('/api/input',{serial,type:'rotate'});say('rotated')}catch(e){say(e.message,true)}};document.querySelector('#clear').onclick=async()=>{try{await post('/api/clear',{serial});say('app data cleared')}catch(e){say(e.message,true)}};document.querySelector('#capture').onclick=()=>{const a=document.createElement('a');a.href='/api/screenshot?serial='+encodeURIComponent(serial);a.download='device.png';a.click();say('screenshot downloaded')};devices();
</script></body></html>`
