const MCP_SUPPORTED_PROVIDERS = new Set([
  "claude",
  "codex",
  "cursor",
  "hermes",
  "kimi",
  "kiro",
  "opencode",
  "openclaw",
]);

export function providerSupportsMcpConfig(provider: string | undefined | null): boolean {
  return provider != null && MCP_SUPPORTED_PROVIDERS.has(provider);
}
