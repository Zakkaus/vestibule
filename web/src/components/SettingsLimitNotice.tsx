import { useTranslation } from "react-i18next";

import type { ApiRequestError } from "../lib/api";

type SettingsLimitNoticeProps = Readonly<{
  error: ApiRequestError;
  messageKey: string;
}>;

export function SettingsLimitNotice({ error, messageKey }: SettingsLimitNoticeProps) {
  const { t } = useTranslation();
  const violations = error.kind === "api" && error.code === "settings_limit_exceeded"
    ? error.limitViolations
    : [];
  return (
    <div>
      <span>{t(messageKey)}</span>
      {violations.length > 0 ? (
        <ul data-settings-limit-violations>
          {violations.map((violation) => (
            <li key={`${violation.chatId}:${violation.field}`}>
              {t("owner.violations.row", {
                chatId: violation.chatId,
                field: t(`owner.fields.${violation.field}`, { defaultValue: violation.field }),
                value: violation.value,
                limit: violation.limit
              })}
            </li>
          ))}
        </ul>
      ) : null}
    </div>
  );
}
