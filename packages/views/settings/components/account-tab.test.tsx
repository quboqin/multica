import type { ReactNode } from "react";
import { describe, it, expect, beforeEach, vi } from "vitest";
import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { I18nProvider } from "@multica/core/i18n/react";
import enCommon from "../../locales/en/common.json";
import enSettings from "../../locales/en/settings.json";
import type { User } from "@multica/core/types";

const mockUpdateMe = vi.hoisted(() => vi.fn());
const mockSetUser = vi.hoisted(() => vi.fn());
const mockToastSuccess = vi.hoisted(() => vi.fn());
const mockToastError = vi.hoisted(() => vi.fn());
const userRef = vi.hoisted<{ current: User }>(() => ({
  current: {
    id: "user-1",
    name: "Alice",
    email: "alice@example.com",
    avatar_url: null,
    onboarded_at: null,
    onboarding_questionnaire: {},
    starter_content_state: null,
    language: null,
    profile_description: "",
    timezone: null,
    integration_tokens: {
      git_token: "old-git",
      notion_token: "old-notion",
    },
    created_at: "2026-01-01T00:00:00Z",
    updated_at: "2026-01-01T00:00:00Z",
  } satisfies User,
}));

vi.mock("@multica/core/api", () => ({
  api: { updateMe: mockUpdateMe },
}));

vi.mock("@multica/core/auth", () => {
  type AuthState = {
    user: typeof userRef.current;
    setUser: typeof mockSetUser;
  };
  const state = (): AuthState => ({
    user: userRef.current,
    setUser: mockSetUser,
  });
  const useAuthStore = Object.assign(
    (sel?: (s: AuthState) => unknown) => (sel ? sel(state()) : state()),
    { getState: state },
  );
  return { useAuthStore };
});

vi.mock("@multica/core/hooks/use-file-upload", () => ({
  useFileUpload: () => ({ upload: vi.fn(), uploading: false }),
}));

vi.mock("@multica/core/workspace/avatar-url", () => ({
  resolvePublicFileUrl: (url: string) => url,
}));

vi.mock("sonner", () => ({
  toast: { success: mockToastSuccess, error: mockToastError },
}));

import { AccountTab } from "./account-tab";

const TEST_RESOURCES = {
  en: { common: enCommon, settings: enSettings },
};

function I18nWrapper({ children }: { children: ReactNode }) {
  return (
    <I18nProvider locale="en" resources={TEST_RESOURCES}>
      {children}
    </I18nProvider>
  );
}

describe("AccountTab integration credentials", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    userRef.current = {
      ...userRef.current,
      name: "Alice",
      profile_description: "",
      integration_tokens: {
        git_token: "old-git",
        notion_token: "old-notion",
      },
    };
    mockUpdateMe.mockImplementation(async (payload) => ({
      ...userRef.current,
      ...payload,
      integration_tokens: payload.integration_tokens,
    }));
  });

  it("saves added credentials and sends blank values for deleted credentials", async () => {
    const user = userEvent.setup();
    render(<AccountTab />, { wrapper: I18nWrapper });

    await user.click(screen.getByRole("button", { name: /delete notion_token/i }));
    await user.click(screen.getByRole("button", { name: /add credential/i }));

    const rows = screen.getAllByTestId("integration-token-row");
    const newRow = rows.at(-1);
    if (!newRow) throw new Error("new credential row not found");

    const keyInput = within(newRow).getByPlaceholderText("Choose or type a key");
    await user.clear(keyInput);
    await user.type(keyInput, "linear_token");

    const tokenInput = within(newRow).getByPlaceholderText("Enter token");
    await user.type(tokenInput, "lin-123");

    await user.click(screen.getByRole("button", { name: /update profile/i }));

    await waitFor(() => {
      expect(mockUpdateMe).toHaveBeenCalledTimes(1);
    });
    expect(mockUpdateMe).toHaveBeenCalledWith(
      expect.objectContaining({
        integration_tokens: {
          git_token: "old-git",
          linear_token: "lin-123",
          notion_token: "",
        },
      }),
    );
    expect(mockSetUser).toHaveBeenCalledTimes(1);
    expect(mockToastSuccess).toHaveBeenCalledTimes(1);
  });

  it("does not recreate empty built-in credentials after they are removed", () => {
    userRef.current = {
      ...userRef.current,
      integration_tokens: {},
    };

    render(<AccountTab />, { wrapper: I18nWrapper });

    expect(screen.queryByDisplayValue("git_token")).not.toBeInTheDocument();
    expect(screen.queryByDisplayValue("feishu_mcp_token")).not.toBeInTheDocument();
    expect(screen.queryByDisplayValue("paones_token")).not.toBeInTheDocument();
    expect(screen.queryByDisplayValue("jingwei_token")).not.toBeInTheDocument();
    expect(screen.getByText("No integration credentials saved.")).toBeInTheDocument();
  });

  it("blocks save when duplicate credential keys are present", async () => {
    const user = userEvent.setup();
    render(<AccountTab />, { wrapper: I18nWrapper });

    await user.click(screen.getByRole("button", { name: /add credential/i }));
    const keyInputs = screen.getAllByPlaceholderText("Choose or type a key");
    await user.clear(keyInputs.at(-1)!);
    await user.type(keyInputs.at(-1)!, "git_token");

    expect(await screen.findByText(/can only appear once/i)).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /update profile/i })).toBeDisabled();
    expect(mockUpdateMe).not.toHaveBeenCalled();
  });
});
