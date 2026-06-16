/**
 * Web fallback for mobile markdown code highlighting.
 *
 * The native implementation uses `react-native-shiki-engine`, which requires
 * a native TurboModule and crashes on Expo Web. Web keeps code blocks readable
 * by rendering the plain monospace fallback from `CodeBlock`.
 */

const LANG_ALIASES: Record<string, string> = {
  ts: "typescript",
  js: "javascript",
  py: "python",
  rs: "rust",
  sh: "bash",
  zsh: "bash",
  shell: "bash",
  yml: "yaml",
  md: "markdown",
};

const KNOWN_LANGS: ReadonlySet<string> = new Set([
  "bash",
  "go",
  "javascript",
  "json",
  "jsx",
  "markdown",
  "python",
  "rust",
  "sql",
  "tsx",
  "typescript",
  "yaml",
]);

export const SHIKI_THEME_LIGHT = "github-light";
export const SHIKI_THEME_DARK = "github-dark";

export interface HighlightedToken {
  content: string;
  color?: string;
}

export interface HighlightedLine {
  tokens: HighlightedToken[];
}

export function prewarmHighlighter(): void {
  // No-op on Web. See file header.
}

export function resolveLang(input: string | undefined): string | null {
  if (!input) return null;
  const lower = input.toLowerCase().trim();
  const resolved = LANG_ALIASES[lower] ?? lower;
  return KNOWN_LANGS.has(resolved) ? resolved : null;
}

export async function highlight(): Promise<HighlightedLine[] | null> {
  return null;
}
