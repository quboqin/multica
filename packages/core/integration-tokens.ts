const NON_ENV_KEY_CHARS = /[^A-Z0-9]+/g;

export const DEFAULT_INTEGRATION_TOKEN_KEYS = [
  "git_token",
  "feishu_mcp_token",
  "paones_token",
  "jingwei_token",
];

export function integrationTokenEnvKey(key: string): string {
  return key.trim().toUpperCase().replace(NON_ENV_KEY_CHARS, "_").replace(/^_+|_+$/g, "");
}

export function integrationTokenPlaceholder(key: string): string {
  const envKey = integrationTokenEnvKey(key);
  if (!envKey) return "";
  const name = envKey.startsWith("MULTICA_INTEGRATION_")
    ? envKey
    : "MULTICA_INTEGRATION_" + envKey;
  return "${" + name + "}";
}
