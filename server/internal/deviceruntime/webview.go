package deviceruntime

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

const (
	webViewDiscoveryTimeout = 5 * time.Second
	maxWebViewActionTimeout = 30 * time.Second
	webViewIdleGrace        = 20 * time.Minute
)

type webViewSelector struct {
	Role        string `json:"role,omitempty"`
	Name        string `json:"name,omitempty"`
	Text        string `json:"text,omitempty"`
	Placeholder string `json:"placeholder,omitempty"`
	TestID      string `json:"test_id,omitempty"`
	CSS         string `json:"css,omitempty"`
	Contains    bool   `json:"contains,omitempty"`
}

type webViewActionRequest struct {
	Serial    string          `json:"serial"`
	Action    string          `json:"action"`
	Selector  webViewSelector `json:"selector"`
	Value     string          `json:"value,omitempty"`
	Text      string          `json:"text,omitempty"`
	URL       string          `json:"url,omitempty"`
	TimeoutMS int             `json:"timeout_ms,omitempty"`
}

type webViewTarget struct {
	ID                   string `json:"id"`
	Title                string `json:"title"`
	Type                 string `json:"type"`
	URL                  string `json:"url"`
	WebSocketDebuggerURL string `json:"webSocketDebuggerUrl"`
}

type cdpResponse struct {
	ID     int `json:"id"`
	Result struct {
		Result struct {
			Type        string          `json:"type"`
			Value       json.RawMessage `json:"value"`
			Description string          `json:"description"`
		} `json:"result"`
		ExceptionDetails *struct {
			Text      string `json:"text"`
			Exception struct {
				Description string `json:"description"`
			} `json:"exception"`
		} `json:"exceptionDetails,omitempty"`
	} `json:"result"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

func (s Server) webViewSnapshot(w http.ResponseWriter, r *http.Request) {
	serial := strings.TrimSpace(r.URL.Query().Get("serial"))
	if serial == "" {
		writeError(w, fmt.Errorf("serial is required"))
		return
	}
	ctx, cancel := s.requestContext(r)
	if !s.authorizeDeviceLease(w, r, serial) {
		return
	}

	defer cancel()
	var snapshot any
	if err := s.withWebView(ctx, serial, func(client *cdpClient) error {
		return client.evaluate(ctx, webViewExpression(webViewOperation{Action: "snapshot"}), &snapshot)
	}); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, snapshot)
}

func (s Server) webViewAction(w http.ResponseWriter, r *http.Request) {
	var request webViewActionRequest
	decoder := json.NewDecoder(io.LimitReader(r.Body, 1<<20))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil {
		writeError(w, fmt.Errorf("decode WebView action: %w", err))
		return
	}
	if err := validateWebViewAction(request); err != nil {
		writeError(w, err)
		return
	}
	if !s.authorizeDeviceLease(w, r, request.Serial) {
		return
	}

	timeout := time.Duration(request.TimeoutMS) * time.Millisecond
	if timeout == 0 {
		timeout = 5 * time.Second
	}
	ctx, cancel := context.WithTimeout(r.Context(), timeout+webViewDiscoveryTimeout)
	defer cancel()
	operation := webViewOperation{
		Action:    request.Action,
		Selector:  request.Selector,
		Value:     request.Value,
		Text:      request.Text,
		URL:       request.URL,
		TimeoutMS: int(timeout / time.Millisecond),
	}
	var result any
	if err := s.withWebView(ctx, request.Serial, func(client *cdpClient) error {
		return client.evaluate(ctx, webViewExpression(operation), &result)
	}); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func validateWebViewAction(request webViewActionRequest) error {
	if strings.TrimSpace(request.Serial) == "" {
		return fmt.Errorf("serial is required")
	}
	switch request.Action {
	case "fill", "tap", "assert", "wait_for":
	default:
		return fmt.Errorf("unsupported WebView action %q", request.Action)
	}
	selector := request.Selector
	if selector.Role == "" && selector.Name == "" && selector.Text == "" && selector.Placeholder == "" && selector.TestID == "" && selector.CSS == "" {
		return fmt.Errorf("selector must contain role, name, text, placeholder, test_id, or css")
	}
	if request.Action == "fill" && len(request.Value) > 16*1024 {
		return fmt.Errorf("fill value exceeds 16 KiB")
	}
	if request.TimeoutMS < 0 || time.Duration(request.TimeoutMS)*time.Millisecond > maxWebViewActionTimeout {
		return fmt.Errorf("timeout_ms must be between 0 and %d", maxWebViewActionTimeout.Milliseconds())
	}
	return nil
}

type webViewSessionStore struct {
	mu        sync.Mutex
	sessions  map[string]*webViewSession
	idleGrace time.Duration
}

type webViewSession struct {
	mu        sync.Mutex
	client    *cdpClient
	forward   adbForward
	idleTimer *time.Timer
}

func newWebViewSessionStore() *webViewSessionStore {
	return newWebViewSessionStoreWithIdleGrace(webViewIdleGrace)

}

func newWebViewSessionStoreWithIdleGrace(idleGrace time.Duration) *webViewSessionStore {
	return &webViewSessionStore{sessions: make(map[string]*webViewSession), idleGrace: idleGrace}
}

func (s *webViewSessionStore) session(serial string) *webViewSession {
	s.mu.Lock()
	defer s.mu.Unlock()
	session := s.sessions[serial]
	if session == nil {
		session = &webViewSession{}
		s.sessions[serial] = session
	}
	return session
}

func (s Server) withWebView(ctx context.Context, serial string, run func(*cdpClient) error) error {
	if s.webViews == nil {
		return fmt.Errorf("WebView automation is unavailable")
	}
	session := s.webViews.session(serial)
	session.mu.Lock()
	if session.idleTimer != nil {
		session.idleTimer.Stop()
		session.idleTimer = nil
	}
	defer func() {
		session.armIdleClose(s.webViews, serial)
		session.mu.Unlock()
	}()
	for attempt := 0; attempt < 2; attempt++ {
		if session.client == nil {
			forward, target, err := s.discoverWebView(ctx, serial)
			if err != nil {
				return err
			}
			connection, _, err := websocket.DefaultDialer.DialContext(ctx, target.WebSocketDebuggerURL, nil)
			if err != nil {
				forward.close()
				return fmt.Errorf("connect WebView debugger: %w", err)
			}
			session.forward = forward
			session.client = &cdpClient{connection: connection}
		}
		if err := run(session.client); err == nil {
			return nil
		} else if attempt == 1 {
			session.close()
			return err
		}
		session.close()
	}
	return fmt.Errorf("WebView automation failed")
}

func (s *webViewSession) armIdleClose(store *webViewSessionStore, serial string) {
	if store.idleGrace <= 0 {
		return
	}
	s.idleTimer = time.AfterFunc(store.idleGrace, func() {
		s.mu.Lock()
		s.close()
		s.idleTimer = nil
		s.mu.Unlock()
	})
}

func (s *webViewSession) close() {
	if s.client != nil {
		_ = s.client.connection.Close()
		s.client = nil
	}
	if s.forward.forward != "" {
		s.forward.close()
		s.forward = adbForward{}
	}
}

type adbForward struct {
	adb     ADB
	serial  string
	forward string
}

func (f adbForward) close() {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_, _ = f.adb.run(ctx, "-s", f.serial, "forward", "--remove", f.forward)
}

func (s Server) discoverWebView(ctx context.Context, serial string) (adbForward, webViewTarget, error) {
	pidOutput, err := s.ADB.run(ctx, "-s", serial, "shell", "pidof", s.Package)
	if err != nil || strings.TrimSpace(string(pidOutput)) == "" {
		return adbForward{}, webViewTarget{}, fmt.Errorf("application %s is not running", s.Package)
	}
	pid := strings.Fields(string(pidOutput))[0]
	socket := "webview_devtools_remote_" + pid
	sockets, err := s.ADB.run(ctx, "-s", serial, "shell", "cat", "/proc/net/unix")
	if err != nil || !strings.Contains(string(sockets), socket) {
		return adbForward{}, webViewTarget{}, fmt.Errorf("WebView debugging is unavailable for %s", s.Package)
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return adbForward{}, webViewTarget{}, fmt.Errorf("allocate WebView debugger port: %w", err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	_ = listener.Close()
	forwardName := "tcp:" + strconv.Itoa(port)
	if _, err = s.ADB.run(ctx, "-s", serial, "forward", forwardName, "localabstract:"+socket); err != nil {
		return adbForward{}, webViewTarget{}, fmt.Errorf("forward WebView debugger: %w", err)
	}
	forward := adbForward{adb: s.ADB, serial: serial, forward: forwardName}
	deadline := time.Now().Add(webViewDiscoveryTimeout)
	for time.Now().Before(deadline) {
		targets, targetErr := fetchWebViewTargets(ctx, port)
		if targetErr == nil {
			if target, ok := selectWebViewTarget(targets); ok {
				return forward, target, nil
			}
		}
		select {
		case <-ctx.Done():
			forward.close()
			return adbForward{}, webViewTarget{}, ctx.Err()
		case <-time.After(50 * time.Millisecond):
		}
	}
	forward.close()
	return adbForward{}, webViewTarget{}, fmt.Errorf("WebView page target was not found")
}

func fetchWebViewTargets(ctx context.Context, port int) ([]webViewTarget, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://127.0.0.1:"+strconv.Itoa(port)+"/json", nil)
	if err != nil {
		return nil, err
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	var targets []webViewTarget
	if err := json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&targets); err != nil {
		return nil, err
	}
	return targets, nil
}

func selectWebViewTarget(targets []webViewTarget) (webViewTarget, bool) {
	for _, target := range targets {
		if target.Type == "page" && target.WebSocketDebuggerURL != "" && target.URL != "" {
			return target, true
		}
	}
	return webViewTarget{}, false
}

type cdpClient struct {
	connection *websocket.Conn
}

func (c *cdpClient) evaluate(ctx context.Context, expression string, out any) error {
	deadline, ok := ctx.Deadline()
	if ok {
		_ = c.connection.SetWriteDeadline(deadline)
		_ = c.connection.SetReadDeadline(deadline)
	}
	request := map[string]any{
		"id":     1,
		"method": "Runtime.evaluate",
		"params": map[string]any{
			"expression":    expression,
			"awaitPromise":  true,
			"returnByValue": true,
		},
	}
	if err := c.connection.WriteJSON(request); err != nil {
		return fmt.Errorf("send WebView command: %w", err)
	}
	for {
		var response cdpResponse
		if err := c.connection.ReadJSON(&response); err != nil {
			return fmt.Errorf("read WebView command: %w", err)
		}
		if response.ID != 1 {
			continue
		}
		if response.Error != nil {
			return fmt.Errorf("WebView command failed: %s", response.Error.Message)
		}
		if response.Result.ExceptionDetails != nil {
			description := response.Result.ExceptionDetails.Exception.Description
			if description == "" {
				description = response.Result.ExceptionDetails.Text
			}
			return fmt.Errorf("WebView evaluation failed: %s", description)
		}
		if out == nil {
			return nil
		}
		if len(response.Result.Result.Value) == 0 {
			return fmt.Errorf("WebView evaluation returned no value: %s", response.Result.Result.Description)
		}
		if err := json.Unmarshal(response.Result.Result.Value, out); err != nil {
			return fmt.Errorf("decode WebView result: %w", err)
		}
		return nil
	}
}

type webViewOperation struct {
	Action    string          `json:"action"`
	Selector  webViewSelector `json:"selector,omitempty"`
	Value     string          `json:"value,omitempty"`
	Text      string          `json:"text,omitempty"`
	URL       string          `json:"url,omitempty"`
	TimeoutMS int             `json:"timeout_ms,omitempty"`
}

func webViewExpression(operation webViewOperation) string {
	payload, _ := json.Marshal(operation)
	return fmt.Sprintf(`(async () => {
  const request = %s;
  const normalize = value => String(value || '').replace(/\s+/g, ' ').trim().toLowerCase();
  const visible = element => {
    const style = getComputedStyle(element);
    const rect = element.getBoundingClientRect();
    return style.display !== 'none' && style.visibility !== 'hidden' && Number(style.opacity) !== 0 && rect.width > 0 && rect.height > 0;
  };
  const roleOf = element => {
    const explicit = element.getAttribute('role');
    if (explicit) return explicit;
    const tag = element.tagName.toLowerCase();
    const type = String(element.getAttribute('type') || '').toLowerCase();
    if (tag === 'textarea' || (tag === 'input' && !['button','submit','reset','checkbox','radio','range'].includes(type))) return 'textbox';
    if (tag === 'button' || (tag === 'input' && ['button','submit','reset'].includes(type))) return 'button';
    if (tag === 'a') return 'link';
    if (tag === 'select') return 'combobox';
    if (type === 'checkbox') return 'checkbox';
    if (type === 'radio') return 'radio';
    if (type === 'range') return 'slider';
    if (/^h[1-6]$/.test(tag)) return 'heading';
    return '';
  };
  const labelOf = element => {
    if (element.labels && element.labels.length) return Array.from(element.labels).map(label => label.innerText).join(' ');
    const id = element.id;
    if (id) {
      const label = document.querySelector('label[for="' + CSS.escape(id) + '"]');
      if (label) return label.innerText;
    }
    return '';
  };
  const nameOf = element => element.getAttribute('aria-label') || labelOf(element) || element.getAttribute('placeholder') || element.innerText || element.getAttribute('value') || '';
  const describe = element => {
    const rect = element.getBoundingClientRect();
    const type = String(element.getAttribute('type') || '');
    const value = type.toLowerCase() === 'password' ? '[redacted]' : ('value' in element ? String(element.value || '') : '');
    return {
      tag: element.tagName.toLowerCase(), role: roleOf(element), name: nameOf(element).trim(), text: String(element.innerText || '').trim(),
      label: labelOf(element).trim(), placeholder: element.getAttribute('placeholder') || '', test_id: element.getAttribute('data-testid') || '',
      id: element.id || '', type, value, disabled: Boolean(element.disabled), visible: visible(element),
      rect: { x: Math.round(rect.x), y: Math.round(rect.y), width: Math.round(rect.width), height: Math.round(rect.height) }
    };
  };
  const candidates = () => Array.from(document.querySelectorAll('input,textarea,button,a,select,[role],[contenteditable="true"],[data-testid],h1,h2,h3,h4,h5,h6')).filter(visible);
	const formControl = element => element.matches('input,textarea,select,[contenteditable="true"]') ? element : element.querySelector('input,textarea,select,[contenteditable="true"]');
	const actionTarget = element => element.matches('button,a,input,textarea,select,[role="button"],[contenteditable="true"]') ? element : element.querySelector('button,a,input,textarea,select,[role="button"],[contenteditable="true"]') || element;
  const matches = (element, selector) => {
    const description = describe(element);
    const compare = (actual, expected) => selector.contains ? normalize(actual).includes(normalize(expected)) : normalize(actual) === normalize(expected);
    return (!selector.role || compare(description.role, selector.role)) && (!selector.name || compare(description.name, selector.name)) &&
      (!selector.text || compare(description.text, selector.text)) && (!selector.placeholder || compare(description.placeholder, selector.placeholder)) &&
      (!selector.test_id || description.test_id === selector.test_id);
  };
  const resolve = selector => {
    const elements = selector.css ? Array.from(document.querySelectorAll(selector.css)).filter(visible) : candidates().filter(element => matches(element, selector));
    if (elements.length !== 1) throw new Error('selector matched ' + elements.length + ' elements: ' + JSON.stringify(selector));
    return elements[0];
  };
  const condition = () => {
    const element = resolve(request.selector);
		const control = formControl(element) || element;
    if (request.value !== undefined && request.value !== '' && String(control.value || '') !== request.value) throw new Error('value mismatch');
    if (request.text && !normalize(element.innerText || '').includes(normalize(request.text))) throw new Error('text mismatch');
    if (request.url && !String(location.href).includes(request.url)) throw new Error('url mismatch');
    return element;
  };
  if (request.action === 'snapshot') return { url: location.href, title: document.title, elements: candidates().slice(0, 500).map(describe) };
  let element;
  if (request.action === 'wait_for' || request.action === 'assert') {
    const deadline = performance.now() + (request.timeout_ms || 5000);
    let lastError;
    while (performance.now() <= deadline) {
      try { element = condition(); break; } catch (error) { lastError = error; await new Promise(resolve => setTimeout(resolve, 50)); }
    }
    if (!element) throw lastError || new Error('condition timed out');
  } else {
    element = resolve(request.selector);
  }
  if (request.action === 'fill') {
		const control = formControl(element);
		if (!control) throw new Error('resolved element does not support fill');
		control.focus();
		const prototype = control instanceof HTMLTextAreaElement ? HTMLTextAreaElement.prototype : HTMLInputElement.prototype;
    const setter = Object.getOwnPropertyDescriptor(prototype, 'value')?.set;
    if (!setter) throw new Error('resolved element does not support fill');
		setter.call(control, request.value);
		control.dispatchEvent(new InputEvent('input', { bubbles: true, inputType: 'insertText', data: request.value }));
		control.dispatchEvent(new Event('change', { bubbles: true }));
  } else if (request.action === 'tap') {
		const target = actionTarget(element);
		target.focus();
		target.click();
  }
  return { ok: true, action: request.action, url: location.href, element: describe(element) };
})()`, payload)
}
