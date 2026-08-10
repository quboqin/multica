export function normalizedProductionPromptText(value: string): string {
  return value.toLowerCase().replace(/\s+/g, " ").trim();
}

export function productionPromptLines(lines: string[]): string[] {
  const seen = new Set<string>();
  return lines
    .flatMap((line) => line.split(/\n+/))
    .map((line) => line.trim())
    .filter((line) => {
      if (!line) return false;
      const normalized = normalizedProductionPromptText(line);
      if (normalized.length >= 48 && seen.has(normalized)) return false;
      if (normalized.length >= 48) seen.add(normalized);
      return true;
    });
}

export function userFacingProductionPrompt(value: string): string {
  return productionPromptLines([value])
    .filter((line) => !normalizedProductionPromptText(line).startsWith("approved repayment rows:"))
    .join("\n");
}
