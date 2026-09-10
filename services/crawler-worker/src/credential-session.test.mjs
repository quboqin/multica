import test from "node:test";
import assert from "node:assert/strict";

import { closeRemoteBrowser, deferSessionExpiryWhileCompleting, verifyConnectorPageAuth } from "./index.mjs";

test("defers a remote browser close while credential persistence is in progress", async () => {
  let closed = false;
  const session = {
    profileID: "profile-1",
    connectorID: "appgrowing",
    status: "pending",
    expiresAt: Date.now() + 60_000,
    browser: { close: async () => { closed = true; } },
    releaseBrowserSlot: () => {},
    context: {},
    page: { url: () => "https://appgrowing-global.youcloud.com/leaflet" },
    autoCompleting: true,
  };

  const result = await closeRemoteBrowser(session);

  assert.equal(closed, false);
  assert.equal(session.closeAfterCompletion, true);
  assert.equal(result.browser_open, true);
});

test("defers session expiry after credential persistence has started", () => {
  const session = { autoCompleting: true };

  assert.equal(deferSessionExpiryWhileCompleting(session), true);
  assert.equal(session.expireAfterCompletion, true);
});

test("bounds the page-context credential probe", async () => {
  let timeoutMS = 0;
  const result = await verifyConnectorPageAuth({
    evaluate: async (_callback, args) => {
      timeoutMS = args.timeoutMS;
      return { status: null, body: null, error: "credential page auth probe timed out" };
    },
  }, {
    authCheck: {
      url: "https://example.test/graphql",
      payload: { query: "query userinfo {}" },
      userIDPath: ["data", "userinfo", "user_id"],
    },
  });

  assert.ok(timeoutMS > 0);
  assert.equal(result.authenticated, false);
  assert.match(result.probe_error, /timed out/);
});
