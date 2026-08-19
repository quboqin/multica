export function createBrowserCapacity(maxOpenBrowsers) {
  let active = 0;

  return {
    tryAcquire() {
      if (active >= maxOpenBrowsers) {
        return null;
      }
      active += 1;
      let released = false;
      return () => {
        if (!released) {
          released = true;
          active -= 1;
        }
      };
    },
    active() {
      return active;
    },
    limit() {
      return maxOpenBrowsers;
    },
  };
}

export function createBrowserLeaseRegistry(capacity, options = {}) {
  const timeoutMS = Number(options.timeoutMS);
  const now = options.now || (() => Date.now());
  const setTimeoutFn = options.setTimeoutFn || setTimeout;
  const clearTimeoutFn = options.clearTimeoutFn || clearTimeout;
  const leases = new Set();
  let nextID = 1;

  async function retire(lease, reason = "released") {
    if (!lease) {
      return;
    }
    if (lease.retirePromise) {
      return lease.retirePromise;
    }
    lease.retirePromise = (async () => {
      leases.delete(lease);
      if (lease.timeoutHandle) {
        clearTimeoutFn(lease.timeoutHandle);
        lease.timeoutHandle = null;
      }
      const browser = lease.browser;
      lease.browser = null;
      try {
        await browser?.close?.();
      } catch {
        // Browser cleanup is best effort; the slot must still be released.
      } finally {
        lease.releaseBrowserSlot?.();
        lease.releaseBrowserSlot = null;
        lease.closedAt = now();
        lease.closeReason = reason;
      }
    })();
    return lease.retirePromise;
  }

  async function reapExpired() {
    const current = now();
    const expired = Array.from(leases).filter((lease) => lease.expiresAt <= current);
    await Promise.allSettled(expired.map((lease) => retire(lease, "timeout")));
  }

  async function acquire(metadata = {}) {
    await reapExpired();
    const releaseBrowserSlot = capacity.tryAcquire();
    if (!releaseBrowserSlot) {
      return null;
    }
    const startedAt = now();
    const lease = {
      id: nextID,
      ...metadata,
      startedAt,
      expiresAt: startedAt + timeoutMS,
      browser: null,
      releaseBrowserSlot,
      timeoutHandle: null,
      retirePromise: null,
      closedAt: null,
      closeReason: "",
    };
    nextID += 1;
    leases.add(lease);
    return lease;
  }

  function attachBrowser(lease, browser) {
    lease.browser = browser;
    if (Number.isFinite(timeoutMS) && timeoutMS > 0) {
      lease.timeoutHandle = setTimeoutFn(() => {
        void retire(lease, "timeout");
      }, Math.max(0, lease.expiresAt - now()));
      lease.timeoutHandle?.unref?.();
    }
    return lease;
  }

  return {
    acquire,
    attachBrowser,
    retire,
    reapExpired,
    active() {
      return leases.size;
    },
  };
}
