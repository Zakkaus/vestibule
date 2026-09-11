import { useEffect, useSyncExternalStore } from "react";

import {
  createApiTransport,
  objectFromPayload,
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
  isOwner: boolean;
}>;

export type ConsoleChatUser = Readonly<{
  id: string;
  firstName: string;
  lastName?: string;
  username?: string;
}>;

export type ConsoleChatPermissionKey = (typeof consoleChatPermissionKeys)[number];

export type ConsoleChatPermissions = Readonly<
  Record<ConsoleChatPermissionKey, boolean | null>
>;

export type ConsoleChatAdministratorStatus = "creator" | "administrator";

export type ConsoleChatAdministrator = Readonly<{
  user: ConsoleChatUser;
  status: ConsoleChatAdministratorStatus;
  permissions: ConsoleChatPermissions;
}>;

export type ConsoleChat = Readonly<{
  id: string;
  title?: string;
  owner: ConsoleChatUser | null;
  administrators: readonly ConsoleChatAdministrator[];
  administratorsStatus: "available" | "unavailable";
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
    !("csrf_token" in payload) ||
    !("is_owner" in payload)
  ) {
    return undefined;
  }

  const telegramId = payload.subject.telegram_id;
  const role = payload.subject.role;
  const expiresAt = payload.expires_at;
  const csrfToken = payload.csrf_token;
  const isOwner = payload.is_owner;

  if (
    typeof telegramId !== "string" ||
    telegramId.length === 0 ||
    (role !== "manager" && role !== "operator") ||
    typeof expiresAt !== "string" ||
    expiresAt.length === 0 ||
    typeof csrfToken !== "string" ||
    csrfToken.length === 0 ||
    typeof isOwner !== "boolean"
  ) {
    return undefined;
  }

  return {
    subject: { telegramId, role },
    expiresAt,
    csrfToken,
    isOwner
  };
}

export const consoleChatPermissionKeys = [
  "can_manage_chat",
  "can_delete_messages",
  "can_manage_video_chats",
  "can_restrict_members",
  "can_promote_members",
  "can_change_info",
  "can_invite_users",
  "can_post_stories",
  "can_edit_stories",
  "can_delete_stories",
  "can_post_messages",
  "can_edit_messages",
  "can_pin_messages",
  "can_manage_topics"
] as const;

function chatUserFromPayload(payload: unknown): ConsoleChatUser | undefined {
  const value = objectFromPayload(payload);
  if (
    !value ||
    typeof value.id !== "string" ||
    value.id.length === 0 ||
    typeof value.first_name !== "string" ||
    value.first_name.length === 0
  ) {
    return undefined;
  }
  const lastName = value.last_name;
  const username = value.username;
  if (
    (lastName !== undefined && typeof lastName !== "string") ||
    (username !== undefined && typeof username !== "string")
  ) {
    return undefined;
  }
  return {
    id: value.id,
    firstName: value.first_name,
    ...(lastName === undefined ? {} : { lastName }),
    ...(username === undefined ? {} : { username })
  };
}

function chatPermissionsFromPayload(payload: unknown): ConsoleChatPermissions | undefined {
  const value = objectFromPayload(payload);
  if (!value) {
    return undefined;
  }
  const permissions = {} as Record<ConsoleChatPermissionKey, boolean | null>;
  for (const key of consoleChatPermissionKeys) {
    const permission = value[key];
    if (permission !== null && typeof permission !== "boolean") {
      return undefined;
    }
    permissions[key] = permission;
  }
  return permissions;
}

function chatAdministratorFromPayload(payload: unknown): ConsoleChatAdministrator | undefined {
  const value = objectFromPayload(payload);
  if (!value || (value.status !== "creator" && value.status !== "administrator")) {
    return undefined;
  }
  const user = chatUserFromPayload(value.user);
  const permissions = chatPermissionsFromPayload(value.permissions);
  return user === undefined || permissions === undefined
    ? undefined
    : { user, status: value.status, permissions };
}

function chatsFromPayload(payload: unknown): readonly ConsoleChat[] | undefined {
  const result = objectFromPayload(payload);
  if (!result || !Array.isArray(result.chats)) {
    return undefined;
  }

  const chats: ConsoleChat[] = [];
  for (const rawValue of result.chats) {
    const value = objectFromPayload(rawValue);
    if (!value || typeof value.id !== "string" || value.id.length === 0) {
      return undefined;
    }
    const title = value.title;
    if (title !== undefined && typeof title !== "string") {
      return undefined;
    }

    let owner: ConsoleChatUser | null;
    if (value.owner === null) {
      owner = null;
    } else {
      const parsedOwner = chatUserFromPayload(value.owner);
      if (parsedOwner === undefined) {
        return undefined;
      }
      owner = parsedOwner;
    }

    if (!Array.isArray(value.administrators)) {
      return undefined;
    }
    const administrators: ConsoleChatAdministrator[] = [];
    for (const administrator of value.administrators) {
      const parsed = chatAdministratorFromPayload(administrator);
      if (parsed === undefined) {
        return undefined;
      }
      administrators.push(parsed);
    }

    if (value.administrators_status !== "available" && value.administrators_status !== "unavailable") {
      return undefined;
    }
    const administratorsStatus = value.administrators_status;

    const chat: ConsoleChat = {
      id: value.id,
      title,
      owner,
      administrators,
      administratorsStatus
    };
    chats.push(chat);
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

  // start memoises, because every screen calls it and only the first should
  // fetch. A transport failure has to be able to ask again, so this is the one
  // place that clears the memo.
  retrySession(initData: string | undefined): Promise<void> {
    this.bootstrap = undefined;
    this.publish({ state: "loading" });
    return this.start(initData);
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

export function canViewInstanceStatus(state: ConsoleSessionState): boolean {
  return "session" in state && state.session.subject.role === "operator";
}
export function canViewOwner(state: ConsoleSessionState): boolean {
  return "session" in state && state.session.isOwner;
}

export function retryConsoleGroups(): Promise<void> {
  return consoleSessionStore.retryGroups();
}

export function retryConsoleSession(): Promise<void> {
  return consoleSessionStore.retrySession(telegramInitData());
}

export function retryConsoleAccess(state: ConsoleSessionState): boolean {
  if (state.state === "blocked") {
    void retryConsoleSession();
    return true;
  }
  if (state.state === "groups-unavailable") {
    void retryConsoleGroups();
    return true;
  }
  return false;
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
