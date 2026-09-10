import { cleanup, render, screen, within } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import { I18nProvider } from "@multica/core/i18n/react";
import { renderWithI18n } from "../../test/i18n";
import { PLATFORM_RELEASE_IDS, platformReleaseContent } from "../releases";
import { ChangelogPage } from "./changelog-page";

afterEach(cleanup);

describe("ChangelogPage", () => {
  it.each(["zh-Hans", "en", "ja", "ko"] as const)("renders only the authored highlights in %s", (locale) => {
    renderWithI18n(<ChangelogPage />, { locale });

    const articles = screen.getAllByRole("article");
    expect(articles).toHaveLength(PLATFORM_RELEASE_IDS.length);
    for (const [index, id] of PLATFORM_RELEASE_IDS.entries()) {
      const release = platformReleaseContent(locale, id);
      const article = within(articles[index]!);
      expect(article.getByRole("heading", { name: release.title })).toBeInTheDocument();
      expect(article.getAllByRole("listitem")).toHaveLength(release.changes.length);
      for (const change of release.changes) expect(article.getByText(change)).toBeInTheDocument();
    }
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
        platformReleaseContent("zh-Hans", "v0_3_23").changes[0]!,
      ),
    ).toBeInTheDocument();
    expect(screen.queryByText("releases.v0_3_36.summary")).not.toBeInTheDocument();
  });
});
