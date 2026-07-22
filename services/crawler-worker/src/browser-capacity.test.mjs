import test from "node:test";
import assert from "node:assert/strict";

import { createBrowserCapacity } from "./browser-capacity.mjs";

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
