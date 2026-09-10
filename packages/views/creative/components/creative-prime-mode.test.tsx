import "@testing-library/jest-dom/vitest";
import { cleanup, screen } from "@testing-library/react";
import { afterEach, expect, it } from "vitest";
import { CreativeOrderPrimeSummary } from "./creative-prime-mode";
import { renderWithI18n } from "../../test/i18n";

afterEach(cleanup);
it("shows the order snapshot, including the frozen template family", () => {
  renderWithI18n(<CreativeOrderPrimeSummary order={{ input_snapshot: { market_pack: { config: { prime_composition_mode: "model_integrated", prime_model_template_family: "light_background" } } } }} />);
  expect(screen.getByTestId("creative-order-prime-mode")).toHaveTextContent("Frozen order mode: Model integration");
  expect(screen.getByTestId("creative-order-prime-mode")).toHaveTextContent("Template family");
});
it("does not label orders without a market snapshot as platform overlays", () => {
  renderWithI18n(<CreativeOrderPrimeSummary order={{ input_snapshot: {} }} />);
  expect(screen.getByTestId("creative-order-prime-mode")).toHaveTextContent("Not recorded");
});
