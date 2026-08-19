// @vitest-environment jsdom

import { render, screen, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { I18nProvider } from "@multica/core/i18n/react";
import { ReleaseAnnouncement } from "./release-announcement";

vi.mock("../navigation", () => ({
  useNavigation: () => ({ push: vi.fn() }),
}));

vi.mock("@multica/core/paths", () => ({
  useWorkspacePaths: () => ({ changelog: () => "/test/changelog" }),
}));

describe("ReleaseAnnouncement", () => {
  beforeEach(() => {
    window.localStorage.clear();
  });

  it("shows the first release announcement when no version has been seen", async () => {
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
      expect(
        screen.getByText(
          "创意工厂可以在每个工作区独立开启或关闭。",
        ),
      ).toBeInTheDocument();
    });
    expect(window.localStorage.getItem("multica:last-seen-platform-version")).toBe("0.3.23");
  });
});
