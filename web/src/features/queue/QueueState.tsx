import { Button } from "@react-spectrum/s2/Button";
import { ButtonGroup } from "@react-spectrum/s2/ButtonGroup";
import { Content, Heading, InlineAlert, Text } from "@react-spectrum/s2";
import { IllustratedMessage } from "@react-spectrum/s2/IllustratedMessage";
import { LinkButton } from "@react-spectrum/s2/LinkButton";
import { ProgressCircle } from "@react-spectrum/s2/ProgressCircle";
import NoElements from "@react-spectrum/s2/illustrations/linear/NoElements";
import NoFilter from "@react-spectrum/s2/illustrations/linear/NoFilter";
import UserGroup from "@react-spectrum/s2/illustrations/linear/UserGroup";
import { style } from "@react-spectrum/s2/style" with { type: "macro" };
import { useTranslation } from "react-i18next";

import { useConsoleSize } from "../../components/ConsoleProvider";
import type { QueueFilter } from "./fixtures";

const stateStyles = style({
  width: "full"
});

export function QueueEmptyState() {
  const { t } = useTranslation();

  return (
    <IllustratedMessage data-record-empty data-queue-empty aria-labelledby="queue-empty-title" styles={stateStyles}>
      <NoElements />
      <Heading id="queue-empty-title" data-state-heading>{t("queue.empty.title")}</Heading>
      <Content><Text>{t("queue.empty.description")}</Text></Content>
    </IllustratedMessage>
  );
}

export function QueueLoadingState() {
  const { t } = useTranslation();

  return (
    <IllustratedMessage data-record-empty data-queue-empty aria-live="polite" aria-labelledby="queue-loading-title" styles={stateStyles}>
      <ProgressCircle isIndeterminate aria-label={t("queue.loading.title")} />
      <Heading id="queue-loading-title" data-state-heading>{t("queue.loading.title")}</Heading>
      <Content><Text>{t("queue.loading.description")}</Text></Content>
    </IllustratedMessage>
  );
}

export function QueueGroupRequiredState() {
  const { t } = useTranslation();
  const size = useConsoleSize("L");

  return (
    <IllustratedMessage data-record-empty data-queue-empty aria-labelledby="queue-group-required-title" styles={stateStyles}>
      <UserGroup />
      <Heading id="queue-group-required-title" data-state-heading>{t("queue.groupRequired.title")}</Heading>
      <Content><Text>{t("queue.groupRequired.description")}</Text></Content>
      <ButtonGroup>
        <LinkButton
          href="/groups"
          variant="accent"
          size={size}
          data-console-control
          data-control-size={size}
        >
          <Text>{t("queue.groupRequired.select")}</Text>
        </LinkButton>
      </ButtonGroup>
    </IllustratedMessage>
  );
}

export function QueueNoGroupsState() {
  const { t } = useTranslation();

  return (
    <IllustratedMessage data-record-empty data-queue-empty aria-labelledby="queue-no-groups-title" styles={stateStyles}>
      <UserGroup />
      <Heading id="queue-no-groups-title" data-state-heading>{t("queue.noGroups.title")}</Heading>
      <Content><Text>{t("queue.noGroups.description")}</Text></Content>
    </IllustratedMessage>
  );
}

export function QueueUnavailableState({
  messageKey,
  onRetry
}: Readonly<{ messageKey: string; onRetry: () => void }>) {
  const { t } = useTranslation();
  const size = useConsoleSize("L");

  return (
    <InlineAlert data-record-empty data-queue-empty data-queue-unavailable variant="negative" aria-labelledby="queue-unavailable-title" styles={stateStyles}>
      <Heading id="queue-unavailable-title" data-state-heading>{t("queue.unavailable.title")}</Heading>
      <Content><Text>{t(messageKey)}</Text></Content>
      <ButtonGroup>
        <Button
          variant="secondary"
          fillStyle="outline"
          size={size}
          data-console-control
          data-control-size={size}
          onPress={onRetry}
        >
          <Text>{t("queue.unavailable.retry")}</Text>
        </Button>
      </ButtonGroup>
    </InlineAlert>
  );
}

type QueueFilteredEmptyStateProps = Readonly<{
  filter?: QueueFilter;
  query?: string;
  onClear: () => void;
}>;

export function QueueFilteredEmptyState({ filter, query, onClear }: QueueFilteredEmptyStateProps) {
  const { t } = useTranslation();
  const size = useConsoleSize("L");
  const group = filter ? (filter.groupLabelKey ? t(filter.groupLabelKey) : filter.groupKey) : "";

  return (
    <IllustratedMessage data-record-empty data-queue-empty aria-labelledby="queue-filtered-empty-title" styles={stateStyles}>
      <NoFilter />
      <Heading id="queue-filtered-empty-title" data-state-heading>{t("queue.filteredEmpty.title")}</Heading>
      <Content>
        <Text>
          {filter
            ? t("queue.filteredEmpty.currentCondition", {
                group,
                result: t(filter.result.labelKey)
              })
            : t("queue.filteredEmpty.queryCondition", { query })}
        </Text>
      </Content>
      <ButtonGroup>
        <Button
          variant="secondary"
          fillStyle="outline"
          size={size}
          data-console-control
          data-control-size={size}
          onPress={onClear}
        >
          <Text>{t("queue.filteredEmpty.clear")}</Text>
        </Button>
      </ButtonGroup>
    </IllustratedMessage>
  );
}
