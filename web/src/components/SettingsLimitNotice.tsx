import { useTranslation } from "react-i18next";
import { Link } from "react-router-dom";

import type { ApiRequestError } from "../lib/api";
import { useConsoleSession } from "../app/session";
import { groupName } from "../lib/chatNames";

type SettingsLimitNoticeProps = Readonly<{
  error: ApiRequestError;
  messageKey: string;
}>;

const editors: Readonly<Record<string, string>> = {
  timeout_seconds: "/verification",
  ban_seconds: "/verification",
  mute_seconds: "/verification",
  lookup_ttl_seconds: "/messages",
  verify_retry_seconds: "/verification",
  verify_max_fails: "/verification",
  warn_limit: "/moderation",
  questions: "/questions",
  fallback_questions: "/questions",
  channel_whitelist: "/bypass",
  trusted_member_group_ids: "/bypass",
  known_chat_ids: "/groups"
};

export function settingsEditorPath(field: string, chatID: string): string {
  const search = new URLSearchParams({ group: chatID });
  if (field === "known_chat_ids") search.set("edit", "known-chats");
  return `${editors[field] ?? "/groups"}?${search}`;
}

export function SettingsLimitNotice({ error, messageKey }: SettingsLimitNoticeProps) {
  const { t } = useTranslation();
  const session = useConsoleSession();
  const chats = "chats" in session ? session.chats : [];
  const exceeded = error.kind === "api" && error.code === "settings_limit_exceeded";
  const violations = exceeded ? error.limitViolations : [];
  return (
    <div>
      <span>{t(exceeded ? "settings.errors.settingsLimitExceeded" : messageKey)}</span>
      {violations.length > 0 ? (
        <ul data-settings-limit-violations>
          {violations.map((violation) => (
            <li key={`${violation.chatId}:${violation.field}`}>
              {t("owner.violations.row", {
                chatId: groupName(violation.chatId, chats.find(chat => chat.id === violation.chatId)?.title),
                field: t(`owner.fields.${violation.field}`, { defaultValue: violation.field }),
                value: violation.value,
                limit: violation.limit
              })}
              {" "}
              <Link to={settingsEditorPath(violation.field, violation.chatId)}>
                {t("owner.violations.edit")}
              </Link>
            </li>
          ))}
        </ul>
      ) : null}
    </div>
  );
}
