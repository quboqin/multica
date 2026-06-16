/**
 * Web fallback for mobile persistent storage.
 *
 * This is not a security boundary; it exists so the Expo Web dev surface can
 * run without native SecureStore. Native builds use persistent-storage.ts.
 */

function getStorage(): Storage | null {
  try {
    return typeof window === "undefined" ? null : window.localStorage;
  } catch {
    return null;
  }
}

export async function getPersistentItem(key: string): Promise<string | null> {
  try {
    return getStorage()?.getItem(key) ?? null;
  } catch {
    return null;
  }
}

export async function setPersistentItem(
  key: string,
  value: string,
): Promise<void> {
  try {
    getStorage()?.setItem(key, value);
  } catch {
    // Persistence is best-effort on Web dev.
  }
}

export async function deletePersistentItem(key: string): Promise<void> {
  try {
    getStorage()?.removeItem(key);
  } catch {
    // Persistence is best-effort on Web dev.
  }
}
