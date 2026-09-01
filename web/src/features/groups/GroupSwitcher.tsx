import { useTranslation } from "react-i18next";
import { useSearchParams } from "react-router-dom";

import { useConsoleSession } from "../../app/session";
import {
  allGroupsSelection,
  groupFixtures,
  isGroupFixtureFallback,
  resolveGroupSelection
} from "./fixtures";

export function GroupSwitcher() {
  const { t } = useTranslation();
  const [searchParams, setSearchParams] = useSearchParams();
  const session = useConsoleSession();
  const fixtureFallback = isGroupFixtureFallback(session);
  const options =
    session.state === "ready"
      ? session.chats.map((chat) => ({
          id: chat.id,
          label: t("groups.groupOption", { id: chat.id })
        }))
      : fixtureFallback
        ? groupFixtures.map((group) => ({
            id: group.id,
            label: t(group.nameKey)
          }))
        : [];
  const selectedGroupId = resolveGroupSelection(searchParams.get("group"), options);
  const isLoading = session.state === "loading" || session.state === "checking-groups";

  function changeSelectedGroup(nextGroupId: string): void {
    setSearchParams((currentSearchParams) => {
      const nextSearchParams = new URLSearchParams(currentSearchParams);

      if (nextGroupId === allGroupsSelection) {
        nextSearchParams.delete("group");
      } else {
        nextSearchParams.set("group", nextGroupId);
      }

      return nextSearchParams;
    });
  }

  return (
    <label data-group-switcher>
      <span>{t("shell.groupSwitcher")}</span>
      <select
        aria-busy={isLoading ? "true" : undefined}
        aria-label={t("shell.groupSwitcher")}
        disabled={options.length === 0}
        value={selectedGroupId}
        onChange={(event) => changeSelectedGroup(event.currentTarget.value)}
      >
        <option value={allGroupsSelection}>{t("shell.allGroups")}</option>
        {options.map((option) => (
          <option key={option.id} value={option.id}>
            {option.label}
          </option>
        ))}
      </select>
    </label>
  );
}
