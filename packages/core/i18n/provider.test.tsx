// @vitest-environment jsdom

import { render, screen } from "@testing-library/react";
import { useTranslation } from "react-i18next";
import { describe, expect, it } from "vitest";
import { I18nProvider } from "./provider";

function TranslationProbe() {
  const { t } = useTranslation("changelog");
  return <output>{t("releases.current.title")}</output>;
}

describe("I18nProvider", () => {
  it("uses a new resource snapshot after the provider is rerendered", () => {
    const initialResources = {
      en: { changelog: { releases: { current: { title: "Initial" } } } },
    };
    const updatedResources = {
      en: { changelog: { releases: { current: { title: "Updated" } } } },
    };

    const view = render(
      <I18nProvider locale="en" resources={initialResources}>
        <TranslationProbe />
      </I18nProvider>,
    );

    expect(screen.getByText("Initial").textContent).toBe("Initial");

    view.rerender(
      <I18nProvider locale="en" resources={updatedResources}>
        <TranslationProbe />
      </I18nProvider>,
    );

    expect(screen.getByText("Updated").textContent).toBe("Updated");
    expect(screen.queryByText("Initial")).toBeNull();
  });
});
