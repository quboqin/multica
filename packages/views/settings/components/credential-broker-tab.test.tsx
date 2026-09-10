import type { ReactNode } from "react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { render, screen } from "@testing-library/react";
import { I18nProvider } from "@multica/core/i18n/react";
import enCommon from "../../locales/en/common.json";
import enSettings from "../../locales/en/settings.json";

const profilesRef = vi.hoisted(() => ({
  current: {
    profiles: [{
      id: "profile-1",
      connector_id: "appgrowing",
      label: "Enterprise AppGrowing",
      status: "active",
      scope: "deployment",
      can_manage: false,
      managers: [],
      created_at: "2026-08-05T00:00:00.000Z",
      updated_at: "2026-08-05T00:00:00.000Z",
    }],
  },
}));

vi.mock("@tanstack/react-query", () => ({
  useQuery: (options: { queryKey: unknown[] }) => {
    const key = JSON.stringify(options.queryKey);
    if (key.includes("connectors")) {
      return { data: { connectors: [{ id: "appgrowing", display_name: "AppGrowing", login_url: "https://example.test", capabilities: [], scope: "deployment" }] }, isLoading: false, isFetching: false, refetch: vi.fn() };
    }
    if (key.includes("profiles")) return { data: profilesRef.current, isLoading: false, isFetching: false, refetch: vi.fn() };
    return { data: [{ user_id: "member-1", role: "member", name: "Material user", email: "member@example.test" }] };
  },
  useMutation: () => ({ mutate: vi.fn(), isPending: false, variables: undefined }),
  useQueryClient: () => ({ invalidateQueries: vi.fn() }),
}));

vi.mock("@multica/core/hooks", () => ({ useWorkspaceId: () => "workspace-1" }));
vi.mock("@multica/core/auth", () => ({
  useAuthStore: (selector: (state: { user: { id: string } }) => unknown) => selector({ user: { id: "member-1" } }),
}));
vi.mock("@multica/core/api", () => ({
  api: {
    startCredentialLoginSession: vi.fn(),
    deleteCredentialProfile: vi.fn(),
    addCredentialProfileManager: vi.fn(),
    deleteCredentialProfileManager: vi.fn(),
    runCredentialCrawl: vi.fn(),
  },
}));
vi.mock("@multica/core/workspace/queries", () => ({ memberListOptions: () => ({ queryKey: ["members"] }) }));
vi.mock("@multica/core/credential", () => ({
  credentialConnectorsOptions: () => ({ queryKey: ["credential", "connectors"] }),
  credentialProfilesOptions: () => ({ queryKey: ["credential", "profiles"] }),
  credentialKeys: { profiles: () => ["credential", "profiles"] },
}));
vi.mock("sonner", () => ({ toast: { success: vi.fn(), error: vi.fn() } }));

import { CredentialBrokerTab } from "./credential-broker-tab";

const TEST_RESOURCES = { en: { common: enCommon, settings: enSettings } };

function I18nWrapper({ children }: { children: ReactNode }) {
  return <I18nProvider locale="en" resources={TEST_RESOURCES}>{children}</I18nProvider>;
}

describe("CredentialBrokerTab", () => {
  beforeEach(() => {
    profilesRef.current = {
      profiles: [{
        id: "profile-1",
        connector_id: "appgrowing",
        label: "Enterprise AppGrowing",
        status: "active",
        scope: "deployment",
        can_manage: false,
        managers: [],
        created_at: "2026-08-05T00:00:00.000Z",
        updated_at: "2026-08-05T00:00:00.000Z",
      }],
    };
  });

  it("shows a read-only user how the shared credential can be used without exposing managers", () => {
    render(<CredentialBrokerTab />, { wrapper: I18nWrapper });

    expect(screen.getByText(/You can use this shared session for material crawls/i)).toBeTruthy();
    expect(screen.queryByText(/^Managed by:/i)).toBeNull();
    expect(screen.queryByRole("button", { name: /^Rebind$/ })).toBeNull();
    expect(screen.queryByRole("button", { name: /^Revoke$/ })).toBeNull();
  });
});
