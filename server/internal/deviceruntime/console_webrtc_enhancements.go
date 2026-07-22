package deviceruntime

const consoleWebRTCEnhancements = `<script>
(() => {
  const runtimeFetch = window.fetch.bind(window);
  let inputQueue = Promise.resolve();
  let pointerQueue = Promise.resolve();
  let webRTCSessionID = '';
  let pendingText = '';
  let pendingTextTimer;

  function selectedSerial() {
    return document.querySelector('#devices').value;
  }

  function queueFetch(input, init) {
    const request = inputQueue.then(() => runtimeFetch(input, init));
    inputQueue = request.then(() => undefined, () => undefined);
    return request;
  }

  function queuePointerFetch(input, init) {
    const request = pointerQueue.then(() => runtimeFetch(input, init));
    pointerQueue = request.then(() => undefined, () => undefined);
    return request;
  }

  function enqueueInput(body) {
    queueFetch('/api/input', {
      method: 'POST',
      headers: {'Content-Type': 'application/json'},
      body: JSON.stringify({serial: selectedSerial(), ...body}),
    }).catch(error => say(error.message, true));
  }

  function flushPendingText() {
    if (pendingTextTimer) clearTimeout(pendingTextTimer);
    pendingTextTimer = undefined;
    if (!pendingText) return;
    const text = pendingText;
    pendingText = '';
    enqueueInput({type: 'text', text});
  }

  function enqueueText(text) {
    pendingText += text;
    if (pendingTextTimer) clearTimeout(pendingTextTimer);
    pendingTextTimer = setTimeout(flushPendingText, 24);
  }

  function closeWebRTCSession() {
    if (!webRTCSessionID) return Promise.resolve();
    const sessionID = webRTCSessionID;
    webRTCSessionID = '';
    return runtimeFetch('/api/webrtc/close', {
      method: 'POST',
      headers: {'Content-Type': 'application/json'},
      body: JSON.stringify({session_id: sessionID}),
      keepalive: true,
    }).then(() => undefined, () => undefined);
  }

  window.fetch = (input, init) => {
    const url = typeof input === 'string' ? input : input.url;
    const path = new URL(url, window.location.href).pathname;
    const isInput = path === '/api/input';
    let isPointerInput = false;
    if (isInput && init && init.body) {
      try {
        const body = JSON.parse(init.body);
        if (body.type === 'tap' || body.type === 'swipe') {
          isPointerInput = true;
          const video = document.querySelector('#screen');
          body.source_width = video.videoWidth;
          body.source_height = video.videoHeight;
          init = {...init, body: JSON.stringify(body)};
        }
      } catch (_) {
        // Let the API return its normal validation response for malformed input.
      }
    }
    if (path === '/api/webrtc/offer') {
      return closeWebRTCSession().then(() => runtimeFetch(input, init)).then(response => {
        response.clone().json().then(data => {
          webRTCSessionID = data.session_id || '';
        }).catch(() => {});
        return response;
      });
    }
    if (!isInput) return runtimeFetch(input, init);
    return isPointerInput ? queuePointerFetch(input, init) : queueFetch(input, init);
  };

  window.addEventListener('pagehide', () => {
    void closeWebRTCSession();
  });

  // A successful SDP exchange does not guarantee that a browser decoded a
  // first H.264 frame. Recover from a stale or failed peer instead of leaving
  // the embedded Issue viewer as a permanent black rectangle.
  let playbackStartedAt = performance.now();
  let recoveryInFlight = false;
  let recoveryAttempts = 0;
  const baseConnect = window.connect;

  async function recoverVideo(reason) {
    if (recoveryInFlight || recoveryAttempts >= 3 || !selectedSerial()) return;
    recoveryInFlight = true;
    recoveryAttempts += 1;
    playbackStartedAt = performance.now();
    say(reason);
    try {
      const activePeer = eval('peer');
      if (activePeer) activePeer.close();
      await closeWebRTCSession();
      await eval('connect()');
    } catch (error) {
      say(error.message || String(error), true);
    } finally {
      recoveryInFlight = false;
    }
  }

  if (typeof baseConnect === 'function') {
    window.connect = async (...args) => {
      playbackStartedAt = performance.now();
      return baseConnect(...args);
    };
  }

  window.setInterval(() => {
    const video = document.querySelector('#screen');
    if (!(video instanceof HTMLVideoElement)) return;
    let activePeer;
    try {
      activePeer = eval('peer');
    } catch (_) {
      return;
    }
    if (!activePeer) return;
    if (activePeer.connectionState === 'failed' || activePeer.connectionState === 'disconnected') {
      void recoverVideo('reconnecting WebRTC...');
      return;
    }
    const decodedFrames = video.getVideoPlaybackQuality?.().totalVideoFrames ?? 0;
    if (decodedFrames > 0) {
      recoveryAttempts = 0;
      return;
    }
    if (performance.now() - playbackStartedAt >= 6_000) {
      void recoverVideo('waiting for the first video frame...');
    }
  }, 1_000);

  if (document.body.classList.contains('embedded')) {
    window.addEventListener('keydown', event => {
      if (event.ctrlKey || event.metaKey || event.altKey) return;
      const keyCodes = {Backspace: '67', Enter: '66', Escape: '4', ArrowUp: '19', ArrowDown: '20', ArrowLeft: '21', ArrowRight: '22'};
      if (keyCodes[event.key]) {
        event.preventDefault();
        flushPendingText();
        enqueueInput({type: 'key', key: keyCodes[event.key]});
      } else if (event.key.length === 1) {
        event.preventDefault();
        enqueueText(event.key);
      }
    });
    window.addEventListener('paste', event => {
      const text = event.clipboardData && event.clipboardData.getData('text');
      if (!text) return;
      event.preventDefault();
      flushPendingText();
      enqueueInput({type: 'text', text});
    });
  }
})();
</script>`
