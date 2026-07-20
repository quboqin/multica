package deviceruntime

// consoleEnhancements keeps the self-contained runtime usable as a local binary
// while allowing the workbench behavior to evolve separately from the transport.
const consoleEnhancements = `<style>
body{font-family:-apple-system,BlinkMacSystemFont,"Segoe UI",sans-serif}
.title,.mark{font-family:-apple-system,BlinkMacSystemFont,"Segoe UI",sans-serif;letter-spacing:0}
.title{font-size:16px;font-weight:650}.sub,.online,.label,.device-select,input,button.action,.toast,.hint{font-family:-apple-system,BlinkMacSystemFont,"Segoe UI",sans-serif}
.app{grid-template-columns:68px minmax(520px,1fr) clamp(312px,24vw,372px)}
.rail{padding:16px 12px}.tool{position:relative;border-radius:5px}.tool:focus-visible,button:focus-visible,input:focus-visible,select:focus-visible{outline:2px solid var(--accent);outline-offset:2px}
.canvas{padding:clamp(18px,3vw,42px)}.device{height:min(80vh,880px);max-width:100%;border-radius:28px;transition:aspect-ratio .18s ease,width .18s ease,height .18s ease}
.screen{border-radius:18px}.inspector{padding:22px}.section{padding-bottom:18px}.log{height:min(30vh,290px);font-family:ui-monospace,SFMono-Regular,Menlo,monospace}
@media(max-width:1040px){.app{grid-template-columns:60px minmax(360px,1fr) 290px}.inspector{padding:16px}.canvas{padding:18px}.device{height:min(76vh,760px)}}
@media(max-width:760px){.app{grid-template-columns:52px 1fr}.device{height:min(82vh,780px);max-width:100%}}
</style><script>
(() => {
  const screenElement = document.querySelector('#screen');
  const deviceElement = document.querySelector('.device');
  const canvasElement = document.querySelector('.canvas');
  const rotateButton = document.querySelector('#rotate');
  const captureButton = document.querySelector('#capture');
  const labels = {
    back: 'Back', home: 'Home', recent: 'Recent apps', rotate: 'Rotate device',
    capture: 'Save screenshot', clear: 'Stop and clear application data'
  };

  for (const [id, label] of Object.entries(labels)) {
    document.querySelector('#' + id).setAttribute('aria-label', label);
  }

  function fitDeviceToStream() {
    if (!screenElement.naturalWidth || !screenElement.naturalHeight) return;
    const ratio = screenElement.naturalWidth / screenElement.naturalHeight;
    const width = Math.max(1, canvasElement.clientWidth - 48);
    const height = Math.max(1, canvasElement.clientHeight - 48);
    const fittedWidth = Math.min(width, height * ratio);
    deviceElement.style.aspectRatio = screenElement.naturalWidth + ' / ' + screenElement.naturalHeight;
    deviceElement.style.width = Math.floor(fittedWidth) + 'px';
    deviceElement.style.height = Math.floor(fittedWidth / ratio) + 'px';
  }

  screenElement.addEventListener('load', fitDeviceToStream);
  new ResizeObserver(fitDeviceToStream).observe(canvasElement);
  fitDeviceToStream();

  rotateButton.onclick = async () => {
    try {
      await post('/api/input', {serial, type: 'rotate'});
      say('rotated; reconnecting stream');
      // Chromium does not safely swap an active MJPEG response in-place. A page
      // reload releases the old response before the new orientation is rendered.
      window.setTimeout(() => window.location.reload(), 180);
    } catch (error) {
      say(error.message, true);
    }
  };

  captureButton.onclick = async () => {
    try {
      const response = await fetch('/api/screenshot?serial=' + encodeURIComponent(serial));
      if (!response.ok) throw new Error((await response.json()).error || response.statusText);
      const link = document.createElement('a');
      const objectURL = URL.createObjectURL(await response.blob());
      link.href = objectURL;
      link.download = 'device.png';
      link.click();
      window.setTimeout(() => URL.revokeObjectURL(objectURL), 0);
      say('screenshot downloaded');
    } catch (error) {
      say(error.message, true);
    }
  };
})();
</script>`
