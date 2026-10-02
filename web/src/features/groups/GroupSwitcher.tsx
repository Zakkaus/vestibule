import { Picker, PickerItem } from "@react-spectrum/s2/Picker";
import { Content } from "@react-spectrum/s2";
import { style } from "@react-spectrum/s2/style" with { type: "macro" };
import { useSearchParams } from "react-router-dom";
import { useTranslation } from "react-i18next";

import { useConsoleSession } from "../../app/session";
import { groupName } from "../../lib/chatNames";
import {
  allGroupsSelection,
  groupFixtures,
  isGroupFixtureFallback,
  resolveGroupSelection
} from "./fixtures";
const groupSwitcherLayout = style({
  width: "full",
  minWidth: 0
});

export function GroupSwitcher({ compact = false }: Readonly<{ compact?: boolean }>) {
  const { t } = useTranslation();
  const [searchParams, setSearchParams] = useSearchParams();
  const session = useConsoleSession();
  const fixtureFallback = isGroupFixtureFallback(session);
  const options =
    session.state === "ready"
      ? session.chats.map((chat) => ({
          id: chat.id,
          label: groupName(chat.id, chat.title)
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
    <Content data-group-switcher styles={style({ minWidth: 0, width: "full" })}>
      <Picker
        aria-label={t("shell.groupSwitcher")}
        label={compact ? undefined : t("shell.groupSwitcher")}
        isDisabled={options.length === 0}
        loadingState={isLoading ? "loading" : "idle"}
        selectedKey={selectedGroupId}
        onSelectionChange={(key) => { if (key !== null) changeSelectedGroup(String(key)); }}
        items={selectionOptions}
        size="M"
        styles={groupSwitcherLayout}
        data-console-control
        data-control-size="M"
      >
        {(option) => <PickerItem id={option.value}>{option.label}</PickerItem>}
      </Picker>
    </Content>
  );
}
