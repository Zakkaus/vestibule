import { Button } from "@react-spectrum/s2/Button";
import { LinkButton } from "@react-spectrum/s2/LinkButton";
import { Text } from "@react-spectrum/s2/Text";
import { useTranslation } from "react-i18next";

import { useConsoleSize } from "../../components/ConsoleProvider";
import { Icon } from "../../icons";
import type { QueueFilter } from "./fixtures";

export function QueueEmptyState() {
  const { t } = useTranslation();

  return (
    <section data-console-card data-record-empty data-queue-empty aria-labelledby="queue-empty-title">
      <h2 id="queue-empty-title" data-state-heading>
        <Icon name="inbox" />
        <Text>{t("queue.empty.title")}</Text>
      </h2>
      <p>{t("queue.empty.description")}</p>
    </section>
  );
}

export function QueueLoadingState() {
  const { t } = useTranslation();

  return (
    <section data-console-card data-record-empty data-queue-empty aria-live="polite" aria-labelledby="queue-loading-title">
      <h2 id="queue-loading-title" data-state-heading>
        <Icon name="loaderCircle" />
        <Text>{t("queue.loading.title")}</Text>
      </h2>
      <p>{t("queue.loading.description")}</p>
    </section>
  );
}

export function QueueGroupRequiredState() {
  const { t } = useTranslation();
  const size = useConsoleSize("L");

  return (
    <section data-console-card data-record-empty data-queue-empty aria-labelledby="queue-group-required-title">
      <h2 id="queue-group-required-title" data-state-heading>
        <Icon name="usersRound" />
        <Text>{t("queue.groupRequired.title")}</Text>
      </h2>
      <p>{t("queue.groupRequired.description")}</p>
      <LinkButton
        href="/groups"
        variant="accent"
        size={size}
        data-console-control
        data-control-size={size}
      >
        <Icon name="usersRound" />
        <Text>{t("queue.groupRequired.select")}</Text>
      </LinkButton>
    </section>
  );
}

export function QueueNoGroupsState() {
  const { t } = useTranslation();

  return (
    <section data-console-card data-record-empty data-queue-empty aria-labelledby="queue-no-groups-title">
      <h2 id="queue-no-groups-title" data-state-heading>
        <Icon name="usersRound" />
        <Text>{t("queue.noGroups.title")}</Text>
      </h2>
      <p>{t("queue.noGroups.description")}</p>
    </section>
  );
}

export function QueueUnavailableState({
  messageKey,
  onRetry
}: Readonly<{ messageKey: string; onRetry: () => void }>) {
  const { t } = useTranslation();
  const size = useConsoleSize("L");

  return (
    <section data-console-card data-record-empty data-queue-empty data-queue-unavailable role="alert" aria-labelledby="queue-unavailable-title">
      <h2 id="queue-unavailable-title" data-state-heading>
        <Icon name="circleAlert" />
        <Text>{t("queue.unavailable.title")}</Text>
      </h2>
      <p>{t(messageKey)}</p>
      <Button
        variant="secondary"
        fillStyle="outline"
        size={size}
        data-console-control
        data-control-size={size}
        onPress={onRetry}
      >
        <Icon name="refreshCw" />
        <Text>{t("queue.unavailable.retry")}</Text>
      </Button>
    </section>
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
    <section data-console-card data-record-empty data-queue-empty aria-labelledby="queue-filtered-empty-title">
      <h2 id="queue-filtered-empty-title" data-state-heading>
        <Icon name="inbox" />
        <Text>{t("queue.filteredEmpty.title")}</Text>
      </h2>
      <p>
        {filter
          ? t("queue.filteredEmpty.currentCondition", {
              group,
              result: t(filter.result.labelKey)
            })
          : t("queue.filteredEmpty.queryCondition", { query })}
      </p>
      <Button
        variant="secondary"
        fillStyle="outline"
        size={size}
        data-console-control
        data-control-size={size}
        onPress={onClear}
      >
        <Icon name="listX" />
        <Text>{t("queue.filteredEmpty.clear")}</Text>
      </Button>
    </section>
  );
}
