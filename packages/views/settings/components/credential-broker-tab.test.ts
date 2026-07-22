import { describe, expect, it } from "vitest";
import { shouldAutoCloseCredentialSession } from "./credential-broker-tab";

const session = {
  id: "session-1",
  profile_id: "profile-1",
  connector_id: "appgrowing",
  browser_url: "https://fat-cybertron.adakamicorp.id/sessions/redacted",
  status: "pending",
  expires_at: "2026-07-22T08:53:37.819Z",
  created_at: "2026-07-22T08:38:37.819Z",
};

describe("shouldAutoCloseCredentialSession", () => {
  it("keeps a rebind session open when the existing profile was already active", () => {
    expect(shouldAutoCloseCredentialSession(session, {
      id: "profile-1",
      status: "active",
      updated_at: "2026-07-22T08:36:42.305Z",
    })).toBe(false);
  });

  it("auto closes after the active profile is updated by this session", () => {
    expect(shouldAutoCloseCredentialSession(session, {
      id: "profile-1",
      status: "active",
      updated_at: "2026-07-22T08:39:12.000Z",
    })).toBe(true);
  });

  it("does not close for another profile", () => {
    expect(shouldAutoCloseCredentialSession(session, {
      id: "profile-2",
      status: "active",
      updated_at: "2026-07-22T08:39:12.000Z",
    })).toBe(false);
  });
});
