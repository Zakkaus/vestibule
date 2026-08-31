import { useTranslation } from "react-i18next";
import { useSearchParams } from "react-router-dom";

import {
  allGroupsSelection,
  groupFixtures,
  resolveGroupSelection
} from "./fixtures";

export function GroupSwitcher() {
  const { t } = useTranslation();
  const [searchParams, setSearchParams] = useSearchParams();
  const selectedGroupId = resolveGroupSelection(searchParams.get("group"));

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
        aria-label={t("shell.groupSwitcher")}
        value={selectedGroupId}
        onChange={(event) => changeSelectedGroup(event.currentTarget.value)}
      >
        <option value={allGroupsSelection}>{t("shell.allGroups")}</option>
        {groupFixtures.map((group) => (
          <option key={group.id} value={group.id}>
            {t(group.nameKey)}
          </option>
        ))}
      </select>
    </label>
  );
}
