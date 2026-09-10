import { useTranslation } from "react-i18next";

import { Badge } from "@react-spectrum/s2/Badge";
import type { SettingSource } from "../verification/api";

const sourceMessageKeys: Readonly<Record<SettingSource, string>> = {
  "factory default": "home.source.factoryDefault",
  "user file": "home.source.userFile",
  "chat override": "home.source.chatOverride"
};

export function HomeSourceBadge({ source }: Readonly<{ source: SettingSource }>) {
  const { t } = useTranslation();
  return (
    <Badge variant="neutral" fillStyle="subtle" data-home-source>
      {t("home.source.value", { source: t(sourceMessageKeys[source]) })}
    </Badge>
  );
}
