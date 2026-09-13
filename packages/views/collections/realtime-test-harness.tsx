import type { WSClient } from "@multica/core/api/ws-client";
import type { ApiClient } from "@multica/core/api/client";
import { createAuthStore, registerAuthStore } from "@multica/core/auth";
import {
  useRealtimeSync,
  type RealtimeSyncStores,
} from "@multica/core/realtime";

type EventHandler = (payload: unknown) => void;

export function createCollectionTestWs() {
  const handlers = new Map<string, Set<EventHandler>>();
  const ws = {
    on: (event: string, handler: EventHandler) => {
      const listeners = handlers.get(event) ?? new Set<EventHandler>();
      listeners.add(handler);
      handlers.set(event, listeners);
      return () => listeners.delete(handler);
    },
    onAny: () => () => {},
    onReconnect: () => () => {},
  } as unknown as WSClient;
  return {
    ws,
    emit(event: string, payload: unknown) {
      for (const handler of handlers.get(event) ?? []) handler(payload);
    },
  };
}

const authStore = createAuthStore({
  api: {} as ApiClient,
  storage: {
    getItem: () => null,
    setItem: () => {},
    removeItem: () => {},
  },
});
authStore.setState({
  user: {
    id: "u1",
    name: "Tester",
    email: "tester@example.com",
    avatar_url: null,
    onboarded_at: "2026-01-01T00:00:00Z",
    onboarding_questionnaire: {},
    starter_content_state: "imported",
    language: null,
    profile_description: "",
    timezone: null,
    created_at: "2026-01-01T00:00:00Z",
    updated_at: "2026-01-01T00:00:00Z",
  },
  isLoading: false,
  status: "authenticated",
});
registerAuthStore(authStore);

const stores = { authStore } as RealtimeSyncStores;

export function CollectionRealtimeHarness({ ws }: { ws: WSClient }) {
  useRealtimeSync(ws, stores);
  return null;
}
