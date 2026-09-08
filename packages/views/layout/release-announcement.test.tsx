// @vitest-environment jsdom

import { cleanup, render, screen, waitFor, within } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { I18nProvider } from "@multica/core/i18n/react";
import { ReleaseAnnouncement } from "./release-announcement";
import { PLATFORM_RELEASE_ID, PLATFORM_VERSION, platformReleaseContent } from "../changelog/releases";

vi.mock("../navigation", () => ({
  useNavigation: () => ({ push: vi.fn() }),
}));

vi.mock("@multica/core/paths", () => ({
  useWorkspacePaths: () => ({ changelog: () => "/test/changelog" }),
}));

describe("ReleaseAnnouncement", () => {
  afterEach(cleanup);
  beforeEach(() => {
    window.localStorage.clear();
  });

  it("shows the first release announcement when no version has been seen", async () => {
    const release = platformReleaseContent("zh-Hans", PLATFORM_RELEASE_ID);

    render(
      <I18nProvider
        locale="zh-Hans"
        resources={{
          "zh-Hans": {
            layout: {
              release_announcement: {
                title: "Multica {{version}} 已更新",
                view_changelog: "查看更新日志",
                later: "稍后",
              },
            },
            changelog: { releases: {} },
          },
        }}
      >
        <ReleaseAnnouncement />
      </I18nProvider>,
    );

    await waitFor(() => {
      expect(screen.getByText(release.changes[0]!)).toBeInTheDocument();
    });
    expect(within(screen.getByRole("alertdialog")).getAllByRole("listitem")).toHaveLength(release.changes.length);
    for (const change of release.changes) expect(screen.getByText(change)).toBeInTheDocument();
    expect(window.localStorage.getItem("multica:last-seen-platform-version")).toBe(PLATFORM_VERSION);
  });
});
