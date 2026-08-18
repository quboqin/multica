import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { I18nProvider } from "@multica/core/i18n/react";
import { renderWithI18n } from "../../test/i18n";
import { PLATFORM_RELEASE_IDS } from "../releases";
import { ChangelogPage } from "./changelog-page";

describe("ChangelogPage", () => {
  it("renders the full release history for the active locale", () => {
    renderWithI18n(<ChangelogPage />, { locale: "zh-Hans" });

    expect(screen.getByText("无匹配文案生成待确认推荐")).toBeInTheDocument();
    expect(screen.getByText("预适配允许部分映射")).toBeInTheDocument();
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
        "预适配现在以已审核的我方还款方案为准，按原图行数与我方可用方案数取小值落表，多出的原图数值直接作为移除项。",
      ),
    ).toBeInTheDocument();
    expect(screen.queryByText("releases.v0_3_36.summary")).not.toBeInTheDocument();
  });
});
