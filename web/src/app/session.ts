import { useEffect, useSyncExternalStore } from "react";

import {
  createApiTransport,
  type ApiRequestError,
  type ApiTransport
} from "../lib/api";

export type ConsoleRole = "manager" | "operator";

export type ConsoleSession = Readonly<{
  subject: Readonly<{
    telegramId: string;
    role: ConsoleRole;
  }>;
  expiresAt: string;
  csrfToken: string;
}>;

export type ConsoleChat = Readonly<{
  id: string;
}>;

export type ConsoleSessionState =
  | Readonly<{ state: "loading" }>
  | Readonly<{ state: "checking-groups"; session: ConsoleSession }>
  | Readonly<{ state: "ready"; session: ConsoleSession; chats: readonly ConsoleChat[] }>
  | Readonly<{ state: "no-groups"; session: ConsoleSession; chats: readonly ConsoleChat[] }>
  | Readonly<{ state: "blocked"; error: ApiRequestError }>
  | Readonly<{
      state: "groups-unavailable";
      session: ConsoleSession;
      error: ApiRequestError;
    }>;

declare global {
  interface Window {
    Telegram?: {
      WebApp?: {
        initData?: unknown;
      };
    };
  }
}

function sessionFromPayload(payload: unknown): ConsoleSession | undefined {
  if (
    typeof payload !== "object" ||
    payload === null ||
    Array.isArray(payload) ||
    !("subject" in payload) ||
    typeof payload.subject !== "object" ||
    payload.subject === null ||
    Array.isArray(payload.subject) ||
    !("telegram_id" in payload.subject) ||
    !("role" in payload.subject) ||
    !("expires_at" in payload) ||
    !("csrf_token" in payload)
  ) {
    return undefined;
  }

  const telegramId = payload.subject.telegram_id;
  const role = payload.subject.role;
  const expiresAt = payload.expires_at;
  const csrfToken = payload.csrf_token;

  if (
    typeof telegramId !== "string" ||
    telegramId.length === 0 ||
    (role !== "manager" && role !== "operator") ||
    typeof expiresAt !== "string" ||
    expiresAt.length === 0 ||
    typeof csrfToken !== "string" ||
    csrfToken.length === 0
  ) {
    return undefined;
  }

  return {
    subject: { telegramId, role },
    expiresAt,
    csrfToken
  };
}

function chatsFromPayload(payload: unknown): readonly ConsoleChat[] | undefined {
  if (
    typeof payload !== "object" ||
    payload === null ||
    Array.isArray(payload) ||
    !("chats" in payload) ||
    !Array.isArray(payload.chats)
  ) {
    return undefined;
  }

  const chats: ConsoleChat[] = [];
  for (const chat of payload.chats) {
    if (
      typeof chat !== "object" ||
      chat === null ||
      Array.isArray(chat) ||
      !("id" in chat) ||
      typeof chat.id !== "string" ||
      chat.id.length === 0
    ) {
      return undefined;
    }
    chats.push({ id: chat.id });
  }

  return chats;
}

function telegramInitData(): string | undefined {
  const initData = window.Telegram?.WebApp?.initData;
  return typeof initData === "string" && initData.length > 0 ? initData : undefined;
}

class ConsoleSessionStore {
  private snapshot: ConsoleSessionState = { state: "loading" };
  private bootstrap: Promise<void> | undefined;
  private groupsLoad: Promise<void> | undefined;
  private readonly listeners = new Set<() => void>();
  readonly api: ApiTransport = createApiTransport(() => this.currentSession());

  readonly getSnapshot = (): ConsoleSessionState => this.snapshot;

  readonly subscribe = (listener: () => void): (() => void) => {
    this.listeners.add(listener);
    return () => this.listeners.delete(listener);
  };

  start(initData: string | undefined): Promise<void> {
    if (!this.bootstrap) {
      this.bootstrap = this.load(initData);
    }
    return this.bootstrap;
  }

  retryGroups(): Promise<void> {
    const session = this.currentSession();
    return session ? this.startGroupsLoad(session) : Promise.resolve();
  }

  private currentSession(): ConsoleSession | undefined {
    const { snapshot } = this;
    return "session" in snapshot ? snapshot.session : undefined;
  }

  private publish(snapshot: ConsoleSessionState): void {
    this.snapshot = snapshot;
    for (const listener of this.listeners) {
      listener();
    }
  }

  private async load(initData: string | undefined): Promise<void> {
    const existing = await this.api.request("/api/session", {
      parse: sessionFromPayload
    });
    const sessionResult = existing.ok
      ? existing
      : initData
        ? await this.api.request("/api/session", {
            method: "POST",
            body: { init_data: initData },
            parse: sessionFromPayload
          })
        : existing;

    if (!sessionResult.ok) {
      this.publish({ state: "blocked", error: sessionResult.error });
      return;
    }

    await this.startGroupsLoad(sessionResult.data);
  }

  private startGroupsLoad(session: ConsoleSession): Promise<void> {
    if (!this.groupsLoad) {
      this.groupsLoad = this.loadGroups(session).finally(() => {
        this.groupsLoad = undefined;
      });
    }
    return this.groupsLoad;
  }

  private async loadGroups(session: ConsoleSession): Promise<void> {
    this.publish({ state: "checking-groups", session });
    const chats = await this.api.request("/api/chats", { parse: chatsFromPayload });

    if (!chats.ok) {
      this.publish({ state: "groups-unavailable", session, error: chats.error });
      return;
    }

    this.publish(
      chats.data.length === 0
        ? { state: "no-groups", session, chats: chats.data }
        : { state: "ready", session, chats: chats.data }
    );
  }
}

export const consoleSessionStore = new ConsoleSessionStore();
export const consoleApi = consoleSessionStore.api;

export function retryConsoleGroups(): Promise<void> {
  return consoleSessionStore.retryGroups();
}

export function useConsoleSession(): ConsoleSessionState {
  const snapshot = useSyncExternalStore(
    consoleSessionStore.subscribe,
    consoleSessionStore.getSnapshot,
    consoleSessionStore.getSnapshot
  );

  useEffect(() => {
    void consoleSessionStore.start(telegramInitData());
  }, []);

  return snapshot;
}
