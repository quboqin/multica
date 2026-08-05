import { describe, expect, it } from "vitest";
import { creativeTimeZoneLabel, formatCreativeDateTime } from "./creative-time";

describe("creative time formatting", () => {
  it("renders UTC timestamps in Asia/Shanghai regardless of the browser timezone", () => {
    expect(formatCreativeDateTime("2026-08-05T05:07:58Z")).toBe("2026/08/05 13:07:58");
    expect(creativeTimeZoneLabel()).toBe("北京时间");
  });

  it("preserves malformed server values for diagnosis", () => {
    expect(formatCreativeDateTime("not-a-date")).toBe("not-a-date");
    expect(formatCreativeDateTime("")).toBe("-");
  });
});
