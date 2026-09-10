import type { ReactNode } from "react";
import { Button, Text } from "@react-spectrum/s2/Button";
import { LinkButton } from "@react-spectrum/s2/LinkButton";
import { Content, Heading, IllustratedMessage, InlineAlert, ProgressCircle } from "@react-spectrum/s2";
import { style } from "@react-spectrum/s2/style" with { type: "macro" };
import { useTranslation } from "react-i18next";

import { Icon, type IconName } from "../../icons";
import { useConsoleSize } from "../../components/ConsoleProvider";
import type { HomeDataState } from "./useHomeData";
import { HomeDashboard } from "./HomeDashboard";

type HomeStateContentProps = Readonly<{
  state: HomeDataState;
  chatID: string | undefined;
  reload: () => void;
}>;

function StateCard({
  id,
  titleKey,
  descriptionKey,
  iconName,
  role,
  live,
  children
}: Readonly<{
  id: string;
  titleKey: string;
  descriptionKey: string;
  iconName?: IconName;
  role?: "alert";
  live?: "polite";
  children?: ReactNode;
}>) {
  const { t } = useTranslation();
  const content = (
    <>
      <Heading id={`home-${id}-title`}>{t(titleKey)}</Heading>
      <Content>{t(descriptionKey)}</Content>
      {children}
    </>
  );
  if (role === "alert") {
    return <InlineAlert variant="negative" data-home-state-card={id} aria-labelledby={`home-${id}-title`}>{content}</InlineAlert>;
  }
  return (
    <IllustratedMessage
      styles={style({ width: "full" })}
      data-home-state-card={id}
      aria-live={live}
      aria-labelledby={`home-${id}-title`}
    >
      {iconName ? <Icon name={iconName} /> : id === "loading" ? <ProgressCircle isIndeterminate aria-label={t(titleKey)} /> : null}
      {content}
    </IllustratedMessage>
  );
}

function GroupRequiredState() {
  const { t } = useTranslation();
  const size = useConsoleSize("XL");
  return (
    <StateCard
      id="group-required"
      titleKey="home.groupRequired.title"
      descriptionKey="home.groupRequired.description"
    >
      <LinkButton
        href="/groups"
        variant="accent"
        size={size}
        data-console-control
        data-control-size={size}
      >
        <Icon name="usersRound" />
        <Text>{t("home.groupRequired.select")}</Text>
      </LinkButton>
    </StateCard>
  );
}

function UnavailableState({ reload }: Readonly<{ reload: () => void }>) {
  const { t } = useTranslation();
  const size = useConsoleSize("XL");
  return (
    <StateCard
      id="unavailable"
      titleKey="home.unavailable.title"
      descriptionKey="home.unavailable.description"
      role="alert"
    >
      <Button
        variant="accent"
        size={size}
        data-console-control
        data-control-size={size}
        onPress={reload}
      >
        <Icon name="refreshCw" />
        <Text>{t("home.unavailable.retry")}</Text>
      </Button>
    </StateCard>
  );
}

export function HomeStateContent({ state, chatID, reload }: HomeStateContentProps) {
  if (state.kind === "loaded" && chatID) {
    return <HomeDashboard data={state.data} chatID={chatID} />;
  }
  if (state.kind === "loading") {
    return (
      <StateCard
        id="loading"
        titleKey="home.loading.title"
        descriptionKey="home.loading.description"
        live="polite"
      />
    );
  }
  if (state.kind === "group-required" || (state.kind === "loaded" && !chatID)) {
    return <GroupRequiredState />;
  }
  if (state.kind === "no-groups") {
    return (
      <StateCard
        id="no-groups"
        titleKey="home.noGroups.title"
        iconName="usersRound"
        descriptionKey="home.noGroups.description"
      />
    );
  }
  return <UnavailableState reload={reload} />;
}
