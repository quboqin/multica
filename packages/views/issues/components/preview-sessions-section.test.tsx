// @vitest-environment jsdom

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, fireEvent, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { setApiInstance } from "@multica/core/api";
import type { ApiClient } from "@multica/core/api/client";
import type {
  PreviewSession,
  PreviewSessionListResponse,
} from "@multica/core/types";
import { renderWithI18n } from "../../test/i18n";

vi.mock("@multica/core/hooks", () => ({
  useWorkspaceId: () => "ws-1",
}));

import { PreviewSessionsSection } from "./preview-sessions-section";

function makeSession(
  overrides: Partial<PreviewSession> = {},
): PreviewSession {
  return {
    id: "preview-1",
    workspaceId: "ws-1",
    issueId: "issue-1",
    taskId: null,
    platform: "web",
    provider: "external_web",
    title: "Checkout preview",
    previewUrl: "https://preview.example.test/checkout",
    status: "running",
    creatorType: "member",
    creatorId: "user-1",
    errorMessage: null,
    expiresAt: null,
    lastActiveAt: null,
    leaseExpiresAt: null,
    startedAt: "2026-07-13T12:00:00Z",
    stoppedAt: null,
    createdAt: "2026-07-13T11:59:00Z",
    updatedAt: "2026-07-13T12:00:00Z",
    ...overrides,
  };
}

function renderSection(
  result: PreviewSessionListResponse | Error,
  onFeedback?: (session: PreviewSession) => void,
) {
  const listPreviewSessions = vi.fn().mockImplementation(async () => {
    if (result instanceof Error) throw result;
    return result;
  });
  const touchPreviewSession = vi.fn().mockImplementation(async (sessionId: string) => {
    if (result instanceof Error) throw result;
    const session = result.previewSessions.find((candidate) => candidate.id === sessionId);
    if (!session) throw new Error("session not found");
    return { ...session, status: "running" as const };
  });
  const switchPreviewSessionDevice = vi
    .fn()
    .mockImplementation(async (sessionId: string, serial: string) => {
      if (result instanceof Error) throw result;
      const session = result.previewSessions.find(
        (candidate) => candidate.id === sessionId,
      );
      if (!session) throw new Error("session not found");
      const previewUrl = new URL(session.previewUrl);
      previewUrl.searchParams.set("serial", serial);
      return { ...session, previewUrl: previewUrl.toString(), status: "running" as const };
    });
  setApiInstance({
    listPreviewSessions,
    touchPreviewSession,
    switchPreviewSessionDevice,
  } as unknown as ApiClient);
  const queryClient = new QueryClient({
    defaultOptions: {
      queries: { retry: false, gcTime: 0 },
      mutations: { retry: false },
    },
  });

  const view = renderWithI18n(
    <QueryClientProvider client={queryClient}>
      <PreviewSessionsSection
        issueId="issue-1"
        onFeedback={onFeedback}
      />
    </QueryClientProvider>,
  );
  return {
    ...view,
    listPreviewSessions,
    touchPreviewSession,
    switchPreviewSessionDevice,
    queryClient,
  };
}

beforeEach(() => {
  Object.defineProperty(document, "fullscreenElement", {
    configurable: true,
    value: null,
    writable: true,
  });
  Object.defineProperty(HTMLElement.prototype, "requestFullscreen", {
    configurable: true,
    writable: true,
    value: vi.fn().mockResolvedValue(undefined),
  });
  Object.defineProperty(document, "exitFullscreen", {
    configurable: true,
    writable: true,
    value: vi.fn().mockResolvedValue(undefined),
  });
});

afterEach(() => {
  cleanup();
  setApiInstance(undefined as unknown as ApiClient);
  vi.restoreAllMocks();
});

describe("PreviewSessionsSection", () => {
  it("stays out of the inspector when the issue has no sessions", async () => {
    const { listPreviewSessions } = renderSection({
      previewSessions: [],
      total: 0,
    });

    await waitFor(() => expect(listPreviewSessions).toHaveBeenCalledWith("issue-1"));
    expect(screen.queryByTestId("preview-sessions-section")).not.toBeInTheDocument();
  });

  it("opens the latest running web artifact inside the issue", async () => {
    const user = userEvent.setup();
    renderSection({
      previewSessions: [
        makeSession({
          id: "stopped-newer",
          title: "Stopped newer",
          status: "stopped",
          createdAt: "2026-07-13T12:10:00Z",
        }),
        makeSession({
          id: "running-old",
          title: "Running old",
          previewUrl: "https://old.example.test",
          createdAt: "2026-07-13T11:00:00Z",
        }),
        makeSession({
          id: "running-new",
          title: "Running new",
          previewUrl: "https://new.example.test/app",
          createdAt: "2026-07-13T12:00:00Z",
        }),
      ],
      total: 3,
    });

    const frame = await screen.findByTitle("Running new");
    fireEvent.load(frame);
    expect(screen.queryByText("Loading preview...")).not.toBeInTheDocument();
    expect(frame).toHaveAttribute("src", "https://new.example.test/app");
    expect(frame).toHaveAttribute("allowfullscreen");
    expect(screen.getByTestId("preview-viewer")).toHaveClass(
      "sm:w-[min(94vw,1440px)]",
    );
    expect(screen.queryByText("Stopped newer")).not.toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: "Close preview" }));
    expect(screen.getByTestId("preview-viewer")).not.toBeVisible();
    expect(screen.getByTitle("Running new")).toBe(frame);
    await user.click(screen.getByRole("button", { name: "Open preview" }));
    expect(await screen.findByTitle("Running new")).toBe(frame);
    expect(screen.queryByText("Loading preview...")).not.toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: "Close preview" }));
    await user.click(screen.getByRole("button", { name: "Other previews (2)" }));
    expect(screen.getByText("Running old")).toBeInTheDocument();
    expect(screen.getByText("Stopped newer")).toBeInTheDocument();
  }, 10_000);

  it("uses the control-free embed contract for a local Android device", async () => {
    const { touchPreviewSession } = renderSection({
      previewSessions: [
        makeSession({
          platform: "android",
          provider: "local_device",
          title: "AdaKami Android",
          previewUrl: "http://127.0.0.1:18080/?serial=emulator-5554",
        }),
      ],
      total: 1,
    });

    const frame = await screen.findByTitle("AdaKami Android");
    expect(touchPreviewSession).toHaveBeenCalledWith("preview-1");
    expect(screen.getByTestId("preview-viewer")).toHaveClass(
      "sm:w-[min(90vw,760px)]",
    );
    const frameUrl = new URL(frame.getAttribute("src") ?? "");
    expect(frameUrl.searchParams.get("serial")).toBe("emulator-5554");
    expect(frameUrl.searchParams.get("embed")).toBe("1");
    expect(screen.getAllByText("emulator-5554").length).toBeGreaterThan(0);
    expect(screen.queryByRole("button", { name: /deploy|stop|log/i })).not.toBeInTheDocument();
  });

  it("switches to a USB device only after its app deployment succeeds", async () => {
    const user = userEvent.setup();
    const { switchPreviewSessionDevice } = renderSection({
      previewSessions: [
        makeSession({
          platform: "android",
          provider: "local_device",
          title: "AdaKami Android",
          previewUrl: "http://127.0.0.1:18081/?serial=emulator-5554",
        }),
      ],
      total: 1,
    });

    const frame = (await screen.findByTitle(
      "AdaKami Android",
    )) as HTMLIFrameElement;
    const runtimeOrigin = "http://127.0.0.1:18081";
    window.dispatchEvent(
      new MessageEvent("message", {
        origin: runtimeOrigin,
        source: frame.contentWindow,
        data: {
          type: "multica:device-inventory",
          devices: [
            {
              serial: "emulator-5554",
              state: "device",
              kind: "emulator",
              model: "Pixel 7",
            },
            {
              serial: "USB-123",
              state: "device",
              kind: "physical",
              model: "Test phone",
            },
          ],
        },
      }),
    );

    const usbButton = await screen.findByRole("button", {
      name: "Switch to USB device",
    });
    expect(screen.getByRole("button", { name: "Switch to emulator" })).toHaveAttribute(
      "aria-pressed",
      "true",
    );
    expect(switchPreviewSessionDevice).not.toHaveBeenCalled();

    const postMessage = vi
      .spyOn(frame.contentWindow as Window, "postMessage")
      .mockImplementation((message: unknown) => {
        const request = message as { type?: string; request_id?: string };
        if (
          request.type !== "multica:device-deploy" ||
          typeof request.request_id !== "string"
        ) {
          return;
        }
        queueMicrotask(() => {
          window.dispatchEvent(
            new MessageEvent("message", {
              origin: runtimeOrigin,
              source: frame.contentWindow,
              data: {
                type: "multica:device-deploy-progress",
                request_id: request.request_id,
                phase: "installing",
                status: "pending",
              },
            }),
          );
          window.setTimeout(() => {
            window.dispatchEvent(
              new MessageEvent("message", {
                origin: runtimeOrigin,
                source: frame.contentWindow,
                data: {
                  type: "multica:device-deploy-result",
                  request_id: request.request_id,
                  ok: true,
                },
              }),
            );
          }, 500);
        });
      });

    await user.click(usbButton);
    expect(await screen.findByText("Installing app...")).toBeInTheDocument();
    await waitFor(() =>
      expect(switchPreviewSessionDevice).toHaveBeenNthCalledWith(
        2,
        "preview-1",
        "USB-123",
        true,
      ),
    );
    expect(switchPreviewSessionDevice).toHaveBeenNthCalledWith(
      1,
      "preview-1",
      "USB-123",
      false,
    );
    expect(postMessage).toHaveBeenCalledWith(
      expect.objectContaining({ type: "multica:device-deploy", serial: "USB-123" }),
      runtimeOrigin,
    );
    await waitFor(() =>
      expect(new URL(frame.getAttribute("src") ?? "").searchParams.get("serial")).toBe(
        "USB-123",
      ),
    );
  });

  it("wakes a sleeping device preview before opening it", async () => {
    const user = userEvent.setup();
    const { touchPreviewSession } = renderSection({
      previewSessions: [
        makeSession({
          platform: "android",
          provider: "local_device",
          status: "sleeping",
          title: "Sleeping Android",
          previewUrl: "http://127.0.0.1:18081?serial=emulator-5554",
        }),
      ],
      total: 1,
    });

    expect(await screen.findByText("Sleeping")).toBeInTheDocument();
    expect(screen.queryByTitle("Sleeping Android")).not.toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "Open preview" }));
    expect(await screen.findByTitle("Sleeping Android")).toBeInTheDocument();
    expect(touchPreviewSession).toHaveBeenCalledWith("preview-1");
  });

  it("reloads the active artifact without leaving the issue", async () => {
    const user = userEvent.setup();
    renderSection({ previewSessions: [makeSession()], total: 1 });

    const firstFrame = await screen.findByTitle("Checkout preview");
    await user.click(screen.getByRole("button", { name: "Reload preview" }));
    await waitFor(() => {
      expect(screen.getByTitle("Checkout preview")).not.toBe(firstFrame);
    });
  });

  it("enters and exits native full screen for the bounded viewer", async () => {
    const user = userEvent.setup();
    let fullscreenElement: Element | null = null;
    Object.defineProperty(document, "fullscreenElement", {
      configurable: true,
      get: () => fullscreenElement,
    });
    const requestFullscreen = vi
      .spyOn(HTMLElement.prototype, "requestFullscreen")
      .mockImplementation(() => {
        fullscreenElement = screen.getByTestId("preview-viewer").firstElementChild;
        document.dispatchEvent(new Event("fullscreenchange"));
        return Promise.resolve();
      });
    const exitFullscreen = vi
      .spyOn(document, "exitFullscreen")
      .mockImplementation(() => {
        fullscreenElement = null;
        document.dispatchEvent(new Event("fullscreenchange"));
        return Promise.resolve();
      });
    renderSection({ previewSessions: [makeSession()], total: 1 });

    await screen.findByTitle("Checkout preview");
    await user.click(screen.getByRole("button", { name: "Full screen" }));
    expect(requestFullscreen).toHaveBeenCalledTimes(1);
    expect(await screen.findByRole("button", { name: "Exit full screen" })).toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: "Exit full screen" }));
    expect(exitFullscreen).toHaveBeenCalledTimes(1);
  });

  it("closes the viewer and hands feedback to the issue composer", async () => {
    const user = userEvent.setup();
    const onFeedback = vi.fn();
    const session = makeSession();
    renderSection({ previewSessions: [session], total: 1 }, onFeedback);

    await screen.findByTitle("Checkout preview");
    await user.click(screen.getByRole("button", { name: "Leave feedback" }));

    expect(onFeedback).toHaveBeenCalledWith(session);
    expect(screen.getByTestId("preview-viewer")).not.toBeVisible();
    expect(screen.getByTitle("Checkout preview")).toBeInTheDocument();
  });

  it("keeps failed and unsafe previews visible but non-interactive", async () => {
    renderSection({
      previewSessions: [
        makeSession({
          title: "Build failed",
          status: "failed",
          errorMessage: "Provider could not start the preview",
          previewUrl: "javascript:alert(document.cookie)",
        }),
      ],
      total: 1,
    });

    expect(await screen.findByText("Build failed")).toBeInTheDocument();
    expect(screen.getByText("Provider could not start the preview")).toBeInTheDocument();
    expect(screen.getByText("Invalid preview URL")).toBeInTheDocument();
    expect(screen.getByTestId("primary-preview")).toBeDisabled();
    expect(screen.queryByTestId("preview-viewer")).not.toBeInTheDocument();
  });

  it("treats a past lease as expired and does not open it", async () => {
    renderSection({
      previewSessions: [
        makeSession({ expiresAt: "2000-01-01T00:00:00Z" }),
      ],
      total: 1,
    });

    expect(await screen.findByText("Expired")).toBeInTheDocument();
    expect(screen.getByTestId("primary-preview")).toBeDisabled();
    expect(screen.queryByTestId("preview-viewer")).not.toBeInTheDocument();
  });

  it("shows a compact retry state when loading fails", async () => {
    const user = userEvent.setup();
    const { listPreviewSessions } = renderSection(new Error("network down"));

    expect(await screen.findByText("Couldn't load previews")).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "Retry" }));
    await waitFor(() => expect(listPreviewSessions).toHaveBeenCalledTimes(2));
  });
});
