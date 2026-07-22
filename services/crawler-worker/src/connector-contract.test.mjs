import test from "node:test";
import assert from "node:assert/strict";

import {
  connectorForID,
  connectorTargetURL,
  normalizeDeclarativeConnector,
} from "./index.mjs";

test("normalizes a declarative connector without AppGrowing defaults", () => {
  const connector = normalizeDeclarativeConnector({
    id: "example-ads",
    display_name: "Example Ads",
    login_url: "https://auth.example.com/login",
    probe_url: "https://ads.example.com/library",
    state_domains: ["example.com"],
    allowed_target_domains: ["ads.example.com"],
    capabilities: ["profile_verify", "page_extract"],
    auth_check: {
      url: "https://ads.example.com/api/me",
      user_id_path: ["data", "id"],
    },
  });

  assert.equal(connector.id, "example-ads");
  assert.equal(connector.displayName, "Example Ads");
  assert.deepEqual(connector.capabilities, ["profile_verify", "page_extract"]);
  assert.equal(connector.authCheck.url, "https://ads.example.com/api/me");
  assert.deepEqual(connector.authCheck.userIDPath, ["data", "id"]);
});

test("rejects unknown connector IDs instead of falling back", () => {
  assert.throws(
    () => connectorForID("not-registered"),
    /unknown credential connector: not-registered/,
  );
});

test("restricts declarative crawl targets to allowed domains", () => {
  const connector = normalizeDeclarativeConnector({
    id: "example-ads",
    login_url: "https://ads.example.com/login",
    state_domains: ["ads.example.com"],
    allowed_target_domains: ["ads.example.com"],
    capabilities: ["page_extract"],
  });

  assert.equal(
    connectorTargetURL(connector, "https://media.ads.example.com/library?page=2"),
    "https://media.ads.example.com/library?page=2",
  );
  assert.throws(
    () => connectorTargetURL(connector, "https://example.net/library"),
    /crawl target host example.net is not allowed/,
  );
});
