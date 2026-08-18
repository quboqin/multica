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

  it("renders the current release body when the runtime changelog snapshot is stale", async () => {
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
          "Prime 不再要求贴片包含二维码；平台按上传的完整透明模板原样叠加，若存在二维码则记录可解码证据，并继续校验文件、尺寸、布局和合成完整性。",
        ),
      ).toBeInTheDocument();
    });
    expect(screen.queryByText("releases.v0_3_36.summary")).not.toBeInTheDocument();
  });
});
