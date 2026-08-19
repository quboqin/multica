import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { I18nProvider } from "@multica/core/i18n/react";
import { renderWithI18n } from "../../test/i18n";
import { PLATFORM_RELEASE_IDS } from "../releases";
import { ChangelogPage } from "./changelog-page";

describe("ChangelogPage", () => {
  it("renders the single release for the active locale", () => {
    renderWithI18n(<ChangelogPage />, { locale: "zh-Hans" });

    expect(screen.getByText("创意工厂、模型配置与平台更新")).toBeInTheDocument();
    expect(
      screen.getByText(
        "Codex 智能体支持选择 GPT-5.6 Sol、GPT-5.6 Terra 和 GPT-5.6 Luna，并按模型展示可用的思考程度；每个智能体都可以单独配置模型和思考程度。",
      ),
    ).toBeInTheDocument();
    expect(screen.getAllByRole("article")).toHaveLength(PLATFORM_RELEASE_IDS.length);
    expect(screen.queryByText("releases.v0_3_35.title")).not.toBeInTheDocument();
  });

  it("keeps release content when the runtime i18n snapshot predates it", () => {
    render(
      <I18nProvider
        locale="zh-Hans"
        resources={{
          "zh-Hans": {
            changelog: {
              title: "更新日志",
              subtitle: "这里维护 Multica 平台自己的版本更新内容。",
              current_version: "当前平台版本：{{version}}",
              releases: {},
            },
          },
        }}
      >
        <ChangelogPage />
      </I18nProvider>,
    );

    expect(
      screen.getByText(
        "创意工厂支持按工作区独立启用，串联素材采集、分析、预适配、素材与文案确认、生图和交付；API、导航和直达页面统一执行能力校验。",
      ),
    ).toBeInTheDocument();
    expect(screen.queryByText("releases.v0_3_36.summary")).not.toBeInTheDocument();
  });
});
