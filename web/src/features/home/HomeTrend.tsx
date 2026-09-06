import { Button, Card, Group, Stack, Table, Text, Title } from "@mantine/core";
import { useTranslation } from "react-i18next";
import { Link } from "react-router-dom";

import { Icon } from "../../icons";
import type { StatsDay } from "../stats/api";
import type { HomeData } from "./useHomeData";

export function HomeTrend({
  data,
  groupSearch
}: Readonly<{ data: HomeData; groupSearch: string }>) {
  const { i18n, t } = useTranslation();
  const locale = i18n.resolvedLanguage ?? i18n.language;
  const number = new Intl.NumberFormat(locale);
  const percent = new Intl.NumberFormat(locale, {
    style: "percent",
    maximumFractionDigits: 1
  });
  const date = new Intl.DateTimeFormat(locale, {
    timeZone: "UTC",
    year: "numeric",
    month: "short",
    day: "numeric"
  });
  const trend: readonly StatsDay[] = data.stats.trend;

  return (
    <Card
      component="section"
      withBorder
      data-home-section="trend"
      aria-labelledby="home-trend-title"
      p="lg"
    >
      <Stack gap="lg">
        <Group justify="space-between" align="flex-start" wrap="wrap" gap="md">
          <Stack gap="xs">
            <Title order={2} size="h3" id="home-trend-title">
              {t("home.trend.title")}
            </Title>
            <Text c="dimmed">{t("home.trend.tableDescription")}</Text>
          </Stack>
          <Button
            component={Link}
            to={{ pathname: "/stats", search: groupSearch }}
            variant="light"
            size="sm"
            data-home-trend-link
            leftSection={<Icon name="chartNoAxesCombined" />}
          >
            {t("home.trend.openStats")}
          </Button>
        </Group>
        {trend.length === 0 ? (
          <Text c="dimmed" data-home-trend-empty>
            {t("stats.trend.noDays")}
          </Text>
        ) : (
          <Table.ScrollContainer
            minWidth={700}
            type="native"
            role="region"
            tabIndex={0}
            aria-label={t("home.trend.tableLabel")}
          >
            <Table
              data-home-trend-table
              withTableBorder
              withColumnBorders
              withRowBorders
              tabularNums
            >
              <Table.Caption>{t("home.trend.tableLabel")}</Table.Caption>
              <Table.Thead>
                <Table.Tr>
                  <Table.Th scope="col">{t("stats.daily.date")}</Table.Th>
                  <Table.Th scope="col">{t("stats.daily.challenges")}</Table.Th>
                  <Table.Th scope="col">{t("stats.daily.approved")}</Table.Th>
                  <Table.Th scope="col">{t("stats.daily.declined")}</Table.Th>
                  <Table.Th scope="col">{t("stats.daily.banned")}</Table.Th>
                  <Table.Th scope="col">{t("stats.daily.expired")}</Table.Th>
                  <Table.Th scope="col">{t("stats.daily.passRate")}</Table.Th>
                </Table.Tr>
              </Table.Thead>
              <Table.Tbody>
                {trend.map((day) => (
                  <Table.Tr key={day.date}>
                    <Table.Th scope="row">
                      <time dateTime={day.date}>
                        {date.format(new Date(`${day.date}T00:00:00Z`))}
                      </time>
                    </Table.Th>
                    <Table.Td>{number.format(day.challenges)}</Table.Td>
                    <Table.Td>{number.format(day.approved)}</Table.Td>
                    <Table.Td>{number.format(day.declined)}</Table.Td>
                    <Table.Td>{number.format(day.banned)}</Table.Td>
                    <Table.Td>{number.format(day.expired)}</Table.Td>
                    <Table.Td>{percent.format(day.pass_rate)}</Table.Td>
                  </Table.Tr>
                ))}
              </Table.Tbody>
            </Table>
          </Table.ScrollContainer>
        )}
      </Stack>
    </Card>
  );
}
