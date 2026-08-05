import { describe, expect, it } from "vitest";
import { dynamicQRPolicy, withDynamicQRPayload } from "./market-pack-qr";

describe("market pack dynamic QR policy", () => {
  it("derives the technical policy from one approved HTTPS target", () => {
    expect(withDynamicQRPayload({ brand: "AdaKami" }, " https://WWW.ADAKAMI.ID/terms ")).toEqual({
      brand: "AdaKami",
      qr_payload: "https://WWW.ADAKAMI.ID/terms",
      qr_canonical_payload: "https://WWW.ADAKAMI.ID/terms",
      qr_allowed_domains: ["www.adakami.id"],
      qr_approval_status: "approved",
      qr_approval_note: "由资源包发布操作确认",
    });
  });

  it("keeps incomplete or non-HTTPS input pending", () => {
    expect(dynamicQRPolicy("http://example.com/terms")).toMatchObject({ valid: false, hostname: "example.com" });
    expect(withDynamicQRPayload({}, "not a url")).toMatchObject({
      qr_allowed_domains: [],
      qr_approval_status: "pending",
    });
  });
});
