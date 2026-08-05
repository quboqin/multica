export type DynamicQRPolicy = {
  payload: string;
  hostname: string;
  valid: boolean;
};

export function dynamicQRPolicy(payload: string): DynamicQRPolicy {
  const normalized = payload.trim();
  try {
    const parsed = new URL(normalized);
    const hostname = parsed.hostname.toLocaleLowerCase();
    return { payload: normalized, hostname, valid: parsed.protocol === "https:" && Boolean(hostname) };
  } catch {
    return { payload: normalized, hostname: "", valid: false };
  }
}

export function withDynamicQRPayload(config: Record<string, unknown>, payload: string): Record<string, unknown> {
  const policy = dynamicQRPolicy(payload);
  return {
    ...config,
    qr_payload: policy.payload,
    qr_canonical_payload: policy.payload,
    qr_allowed_domains: policy.valid ? [policy.hostname] : [],
    qr_approval_status: policy.valid ? "approved" : "pending",
    qr_approval_note: policy.valid ? "由资源包发布操作确认" : "",
  };
}
