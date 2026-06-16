/**
 * Async key/value persistence used by mobile-owned stores.
 *
 * Native uses Expo SecureStore. Web has a platform-specific implementation
 * backed by localStorage because SecureStore's native module is unavailable
 * in the browser.
 */
import * as SecureStore from "expo-secure-store";

export async function getPersistentItem(key: string): Promise<string | null> {
  return SecureStore.getItemAsync(key);
}

export async function setPersistentItem(
  key: string,
  value: string,
): Promise<void> {
  await SecureStore.setItemAsync(key, value);
}

export async function deletePersistentItem(key: string): Promise<void> {
  await SecureStore.deleteItemAsync(key);
}
