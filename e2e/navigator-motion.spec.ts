import { expect, test } from "@playwright/test";
import { createTestApi, loginAsDefault } from "./helpers";
import type { TestApiClient } from "./fixtures";

test.describe("Document and collection panel motion", () => {
  let api: TestApiClient;
  test.beforeEach(async () => { api = await createTestApi(); });
  test.afterEach(async () => { await api.cleanup(); });

  test("animates both panels without reflow and supports interrupted motion and reduced motion", async ({ page }, info) => {
    test.setTimeout(120000);
    await page.setViewportSize({ width: 1440, height: 900 });
    const doc = await api.createIssue("Panel motion document", { kind: "doc" });
    const collection = await api.createCollection("Panel motion table");
    const slug = await loginAsDefault(page);

    for (const [kind, title, path] of [
      ["documents", "Documents", `documents/${doc.id}`],
      ["collections", "Tables", `collections/${collection.id}`],
    ]) {
      await page.goto(`/${slug}/${path}`);
      const panel = page.getByRole("complementary", { name: title, exact: true });
      const toggle = panel.getByRole("button", { name: "Collapse panel", exact: true });
      await expect(toggle).toBeVisible();
      const heading = await panel.getByRole("heading", { name: title, exact: true }).boundingBox();
      const button = await toggle.boundingBox();
      expect(Math.abs(heading!.y + heading!.height / 2 - button!.y - button!.height / 2)).toBeLessThan(2);
      expect(button!.x + button!.width).toBeLessThan(heading!.x);
      await page.screenshot({ animations: "disabled", path: info.outputPath(`${kind}-expanded.png`) });
      const content = panel.locator('[id$="-content"]');
      const contentWidth = (await content.boundingBox())!.width;
      const motion = await panel.evaluate(async (element) => {
        const before = element.getBoundingClientRect().width;
        element.querySelector<HTMLButtonElement>("button[aria-expanded]")!.click();
        await new Promise<void>((resolve) => requestAnimationFrame(() => requestAnimationFrame(() => resolve())));
        const animation = element.getAnimations().find((item) => (item as CSSTransition).transitionProperty === "width");
        if (!animation) throw new Error("Panel did not create a width transition");
        animation.pause();
        animation.currentTime = Number(animation.effect!.getTiming().duration) / 2;
        const content = element.querySelector('[id$="-content"]')!;
        return {
          before,
          middle: element.getBoundingClientRect().width,
          contentTransitions: content.getAnimations().map((item) => (item as CSSTransition).transitionProperty),
        };
      });
      expect(motion.before).toBe(240);
      expect(motion.contentTransitions).toEqual(expect.arrayContaining(["opacity", "translate"]));
      expect(motion.middle).toBeGreaterThan(48);
      expect(motion.middle).toBeLessThan(240);
      expect((await content.boundingBox())!.width).toBeCloseTo(contentWidth, 2);
      await expect(content).toHaveAttribute("inert", "");
      await page.screenshot({ path: info.outputPath(`${kind}-collapsing.png`) });
      await panel.evaluate(async (element) => {
        const animations = element.getAnimations();
        animations.forEach((animation) => animation.play());
        await Promise.all(animations.map((animation) => animation.finished));
      });
      await expect.poll(async () => (await panel.boundingBox())!.width).toBe(48);
      await page.screenshot({ animations: "disabled", path: info.outputPath(`${kind}-collapsed.png`) });
      const expand = panel.getByRole("button", { name: "Expand panel", exact: true });
      await expand.focus();
      await page.keyboard.press("Tab");
      expect(await content.evaluate((element) => element.contains(document.activeElement))).toBe(false);

      // Reverse a running transition, then expand again: the same DOM survives.
      await expand.click();
      await panel.getByRole("button", { name: "Collapse panel", exact: true }).click();
      await panel.getByRole("button", { name: "Expand panel", exact: true }).click();
      await expect.poll(async () => (await panel.boundingBox())!.width).toBe(240);
      await expect(content).not.toHaveAttribute("inert");

      await page.emulateMedia({ reducedMotion: "reduce" });
      await panel.getByRole("button", { name: "Collapse panel", exact: true }).click();
      expect(await panel.evaluate((element) => element.getAnimations().length)).toBe(0);
      expect((await panel.boundingBox())!.width).toBe(48);
      await page.setViewportSize({ width: 390, height: 844 });
      await page.goto(`/${slug}/${kind}`);
      await expect(page.getByRole("textbox", { name: "Search", exact: true })).toBeVisible();
      await expect(page.getByRole("button", { name: "Expand panel", exact: true })).toBeHidden();
      await page.setViewportSize({ width: 1440, height: 900 });
      await page.emulateMedia({ reducedMotion: "no-preference" });
    }
  });
});
