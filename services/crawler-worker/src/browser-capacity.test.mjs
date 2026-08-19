import test from "node:test";
import assert from "node:assert/strict";

import { createBrowserCapacity, createBrowserLeaseRegistry } from "./browser-capacity.mjs";

test("allows only the configured number of active browsers", () => {
  const capacity = createBrowserCapacity(1);
  const release = capacity.tryAcquire();

  assert.equal(typeof release, "function");
  assert.equal(capacity.limit(), 1);
  assert.equal(capacity.active(), 1);
  assert.equal(capacity.tryAcquire(), null);

  release();
  assert.equal(capacity.active(), 0);
  assert.equal(typeof capacity.tryAcquire(), "function");
});

test("releasing a browser slot is idempotent", () => {
  const capacity = createBrowserCapacity(1);
  const release = capacity.tryAcquire();

  release();
  release();

  assert.equal(capacity.active(), 0);
});

test("browser lease registry reaps expired browsers before acquiring", async () => {
  let currentTime = 0;
  let closed = 0;
  const capacity = createBrowserCapacity(1);
  const registry = createBrowserLeaseRegistry(capacity, {
    timeoutMS: 100,
    now: () => currentTime,
    setTimeoutFn: () => ({ unref() {} }),
    clearTimeoutFn: () => {},
  });

  const first = await registry.acquire({ purpose: "crawl" });
  registry.attachBrowser(first, {
    async close() {
      closed += 1;
    },
  });

  assert.equal(capacity.active(), 1);
  assert.equal(await registry.acquire({ purpose: "second" }), null);

  currentTime = 101;
  const second = await registry.acquire({ purpose: "second" });

  assert.equal(closed, 1);
  assert.equal(typeof second, "object");
  assert.equal(capacity.active(), 1);

  await registry.retire(second);
  assert.equal(capacity.active(), 0);
});

test("browser lease timeout closes and releases the slot", async () => {
  const timers = [];
  const capacity = createBrowserCapacity(1);
  const registry = createBrowserLeaseRegistry(capacity, {
    timeoutMS: 100,
    setTimeoutFn: (fn, ms) => {
      timers.push({ fn, ms, cleared: false });
      return timers[timers.length - 1];
    },
    clearTimeoutFn: (handle) => {
      handle.cleared = true;
    },
  });
  let closed = 0;

  const lease = await registry.acquire();
  registry.attachBrowser(lease, {
    async close() {
      closed += 1;
    },
  });

  assert.equal(timers.length, 1);
  assert.equal(timers[0].ms, 100);

  timers[0].fn();
  await lease.retirePromise;

  assert.equal(closed, 1);
  assert.equal(capacity.active(), 0);
  assert.equal(registry.active(), 0);
});
