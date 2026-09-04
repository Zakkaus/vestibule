import { useTranslation } from "react-i18next";
import { useSearchParams } from "react-router-dom";

import { useConsoleSession } from "../../app/session";
import { AppSelect } from "../../components/AppSelect";
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
          label: chat.title && chat.title.trim() ? chat.title : chat.id
        }))
      : fixtureFallback
        ? groupFixtures.map((group) => ({
            id: group.id,
            label: t(group.nameKey)
          }))
        : [];
  const selectedGroupId = resolveGroupSelection(searchParams.get("group"), options);
  const isLoading = session.state === "loading" || session.state === "checking-groups";
  const selectionOptions = [
    {
      label: t("shell.allGroups"),
      value: allGroupsSelection
    },
    ...options.map((option) => ({
      label: option.label,
      value: option.id
    }))
  ];


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
      <AppSelect
        aria-busy={isLoading || undefined}
        aria-label={t("shell.groupSwitcher")}
        disabled={options.length === 0}
        value={selectedGroupId}
        options={selectionOptions}
        onValueChange={changeSelectedGroup}
      />
    </label>
  );
}
