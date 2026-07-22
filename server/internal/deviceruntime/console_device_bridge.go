package deviceruntime

const consoleDeviceBridge = `<script>
(() => {
  if (!document.body.classList.contains('embedded') || window.parent === window) return;
  const deployments = new Map();
  const wait = duration => new Promise(resolve => setTimeout(resolve, duration));
  const publish = (source, origin, payload) => source.postMessage(payload, origin);

  async function publishDeviceInventory() {
    try {
      const response = await api('/api/devices');
      const data = await response.json();
      window.parent.postMessage({
        type: 'multica:device-inventory',
        devices: Array.isArray(data.devices) ? data.devices : [],
      }, '*');
    } catch (_) {
      window.parent.postMessage({type: 'multica:device-inventory', devices: []}, '*');
    }
  }

  async function deploy(event) {
    const requestId = String(event.data.request_id || '');
    const targetSerial = String(event.data.serial || '').trim();
    if (!requestId || !targetSerial || deployments.has(requestId)) return;
    let jobId = '';
    deployments.set(requestId, () => {
      if (jobId) void api('/api/deploy/jobs/' + encodeURIComponent(jobId), {method: 'DELETE'});
    });
    try {
      const devicesResponse = await api('/api/devices');
      const devicesData = await devicesResponse.json();
      const target = Array.isArray(devicesData.devices)
        ? devicesData.devices.find(device => device.serial === targetSerial && device.state === 'device')
        : null;
      if (!target) throw new Error(target?.reason || 'Target device is not online');
      const createdResponse = await api('/api/deploy/jobs', {
        method: 'POST',
        headers: {'Content-Type': 'application/json'},
        body: JSON.stringify({serial: targetSerial}),
      });
      const created = await createdResponse.json();
      jobId = String(created.id || '');
      if (!jobId) throw new Error('Device Runtime did not create a deployment job');
      while (deployments.has(requestId)) {
        const response = await api('/api/deploy/jobs/' + encodeURIComponent(jobId));
        const job = await response.json();
        publish(event.source, event.origin, {
          type: 'multica:device-deploy-progress',
          request_id: requestId,
          phase: job.phase,
          status: job.status,
          install_skipped: job.install_skipped === true,
        });
        if (job.status === 'running') {
          publish(event.source, event.origin, {
            type: 'multica:device-deploy-result', request_id: requestId, ok: true,
            install_skipped: job.install_skipped === true,
          });
          return;
        }
        if (job.status === 'failed' || job.status === 'canceled') {
          throw new Error(job.error || (job.status === 'canceled' ? 'Deployment canceled' : 'Deployment failed'));
        }
        await wait(400);
      }
    } catch (error) {
      publish(event.source, event.origin, {
        type: 'multica:device-deploy-result', request_id: requestId, ok: false,
        error: error.message || String(error),
      });
    } finally {
      deployments.delete(requestId);
    }
  }

  window.addEventListener('message', event => {
    if (event.source !== window.parent || !event.data) return;
    if (event.data.type === 'multica:device-deploy') void deploy(event);
    if (event.data.type === 'multica:device-deploy-cancel') {
      const requestId = String(event.data.request_id || '');
      const cancel = deployments.get(requestId);
      if (cancel) cancel();
      deployments.delete(requestId);
    }
  });
  void publishDeviceInventory();
  window.setInterval(publishDeviceInventory, 1000);
})();
</script>`
