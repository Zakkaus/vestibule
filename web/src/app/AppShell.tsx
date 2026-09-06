import { useState, type ReactNode } from "react";
import {
  Anchor, AppShell as MantineShell, Box, Button, Divider, Drawer,
  Flex, Group, NavLink, Stack, Text
} from "@mantine/core";
import { useTranslation } from "react-i18next";
import {
  Link,
  Outlet,
  useLocation,
  useMatches
} from "react-router-dom";

import { UtilityControls } from "../components/UtilityControls";
import { Icon, type IconName } from "../icons";
import { GroupSwitcher } from "../features/groups";
import {
  canViewInstanceStatus,
  useConsoleSession,
  type ConsoleSessionState
} from "./session";

type ShellVariant = "entry" | "console";

type RouteHandle = {
  shell?: ShellVariant;
  presentation?: "library";
};

type NavigationCapability = "instance-status";

type NavigationGroupID =
  | "daily"
  | "verification"
  | "group"
  | "content"
  | "observe"
  | "console";

type NavigationGroup = Readonly<{
  id: NavigationGroupID;
  labelKey: string;
}>;

type NavigationItem = Readonly<{
  path: string;
  labelKey: string;
  icon: IconName;
  group: NavigationGroupID;
  capability?: NavigationCapability;
}>;

type NavigationSection = Readonly<{
  group: NavigationGroup;
  items: readonly NavigationItem[];
}>;


const capabilityChecks: Readonly<
  Record<NavigationCapability, (state: ConsoleSessionState) => boolean>
> = {
  "instance-status": canViewInstanceStatus
};

const navigationGroups: readonly NavigationGroup[] = [
  { id: "daily", labelKey: "navigation.sections.daily" },
  { id: "verification", labelKey: "navigation.sections.verification" },
  { id: "group", labelKey: "navigation.sections.group" },
  { id: "content", labelKey: "navigation.sections.content" },
  { id: "observe", labelKey: "navigation.sections.observe" },
  { id: "console", labelKey: "navigation.sections.console" }
];

const navigationItems: readonly NavigationItem[] = [
  {
    path: "/home",
    labelKey: "home.navigation",
    icon: "layoutDashboard",
    group: "daily"
  },
  {
    path: "/queue",
    labelKey: "navigation.queue",
    icon: "inbox",
    group: "daily"
  },
  {
    path: "/audit",
    labelKey: "audit.navigation",
    icon: "clipboardList",
    group: "daily"
  },
  {
    path: "/verification",
    labelKey: "verification.navigation",
    icon: "shieldCheck",
    group: "verification"
  },
  {
    path: "/questions",
    labelKey: "questions.navigation",
    icon: "circleHelp",
    group: "verification"
  },
  {
    path: "/bypass",
    labelKey: "bypass.navigation",
    icon: "shieldOff",
    group: "verification"
  },
  {
    path: "/groups",
    labelKey: "navigation.groups",
    icon: "usersRound",
    group: "group"
  },
  {
    path: "/moderation",
    labelKey: "moderation.navigation",
    icon: "shieldAlert",
    group: "group"
  },
  {
    path: "/messages",
    labelKey: "messages.navigation",
    icon: "messagesSquare",
    group: "group"
  },
  {
    path: "/feeds",
    labelKey: "feeds.navigation",
    icon: "rss",
    group: "content"
  },
  {
    path: "/stats",
    labelKey: "stats.navigation",
    icon: "chartNoAxesCombined",
    group: "observe"
  },
  {
    path: "/diagnostics",
    labelKey: "diagnostics.navigation",
    icon: "activity",
    group: "observe"
  },
  {
    path: "/version",
    labelKey: "version.navigation",
    icon: "refreshCw",
    group: "console",
    capability: "instance-status"
  },
  {
    path: "/capabilities",
    labelKey: "capabilities.navigation",
    icon: "slidersHorizontal",
    group: "console"
  },
  {
    path: "/preferences",
    labelKey: "navigation.preferences",
    icon: "settings",
    group: "console"
  }
];

function navigationSections(items: readonly NavigationItem[]): readonly NavigationSection[] {
  const sections: NavigationSection[] = [];

  for (const group of navigationGroups) {
    const groupItems = items.filter((item) => item.group === group.id);
    if (groupItems.length > 0) {
      sections.push({ group, items: groupItems });
    }
  }

  return sections;
}


function ConsoleNavigation({
  sections, selectedGroupSearch, onNavigate
}: Readonly<{
  sections: readonly NavigationSection[];
  selectedGroupSearch: string;
  onNavigate?: () => void;
}>) {
  const { t } = useTranslation();
  const location = useLocation();
  return (
    <Stack component="nav" gap="md" aria-label={t("navigation.label")}>
      {sections.map(({ group, items }) => (
        <Stack key={group.id} gap="xs" data-navigation-group={group.id}>
          <Divider label={t(group.labelKey)} labelPosition="left" />
          {items.map((item) => (
            <NavLink key={item.path} component={Link}
              to={{ pathname: item.path, search: selectedGroupSearch }}
              active={location.pathname === item.path}
              aria-current={location.pathname === item.path ? "page" : undefined}
              variant="light" leftSection={<Icon name={item.icon} />}
              label={t(item.labelKey)} onClick={onNavigate} />
          ))}
        </Stack>
      ))}
    </Stack>
  );
}

function ConsoleFrame({
  sections, selectedGroupSearch, title, children
}: Readonly<{
  sections: readonly NavigationSection[];
  selectedGroupSearch: string;
  title: string;
  children: ReactNode;
}>) {
  const { t } = useTranslation();
  const [navigationOpen, setNavigationOpen] = useState(false);
  return (
    <MantineShell
      data-app-shell data-shell-variant="console" data-library-shell
      header={{ height: { base: 212, xs: 156, sm: 112, md: 72 } }}
      navbar={{ width: 248, breakpoint: "md", collapsed: { mobile: true } }}
      padding="md" transitionDuration={0}
      bg="var(--mantine-color-body)" c="var(--mantine-color-text)"
      style={{ fontFamily: "var(--mantine-font-family)" }}
    >
      <MantineShell.Header data-library-header px="md">
        <Flex h="100%" gap="sm" justify="center" direction={{ base: "column", md: "row" }} align={{ md: "center" }}>
          <Group wrap="nowrap" justify="space-between">
            <Button hiddenFrom="md" variant="default" onClick={() => setNavigationOpen(true)}
              aria-expanded={navigationOpen} leftSection={<Icon name="layoutDashboard" />}>
              {t("shell.mobileNavigation")}
            </Button>
            <Text fw={600} visibleFrom="md">{title}</Text>
          </Group>
          <Flex data-library-header-controls gap="sm" miw={0} flex={1}
            direction={{ base: "column", sm: "row" }} justify="flex-end">
            <Box miw={0} w={{ base: "100%", sm: 240 }}><GroupSwitcher /></Box>
            <Box miw={0} w={{ base: "100%", sm: 360 }}><UtilityControls variant="chrome" /></Box>
          </Flex>
        </Flex>
      </MantineShell.Header>
      <MantineShell.Navbar data-library-sidebar p="md" visibleFrom="md">
        <MantineShell.Section mb="lg">
          <Anchor component={Link} to={{ pathname: "/home", search: selectedGroupSearch }} c="inherit" underline="never">
            <Group gap="xs" wrap="nowrap"><Icon name="shieldCheck" /><Text fw={700}>{t("app.name")}</Text></Group>
          </Anchor>
        </MantineShell.Section>
        <MantineShell.Section grow style={{ overflowY: "auto" }}>
          <ConsoleNavigation sections={sections} selectedGroupSearch={selectedGroupSearch} />
        </MantineShell.Section>
      </MantineShell.Navbar>
      <Drawer opened={navigationOpen} onClose={() => setNavigationOpen(false)}
        title={t("navigation.label")} size="xs" transitionProps={{ duration: 0 }}
        closeButtonProps={{ icon: <Icon name="x" />, "aria-label": t("shell.closeNavigation") }}>
        <ConsoleNavigation sections={sections} selectedGroupSearch={selectedGroupSearch}
          onNavigate={() => setNavigationOpen(false)} />
      </Drawer>
      <MantineShell.Main><Box miw={0}>{children}</Box></MantineShell.Main>
    </MantineShell>
  );
}


export function AppShell() {
  const { t } = useTranslation();
  const location = useLocation();
  const session = useConsoleSession();
  const selectedGroupId = new URLSearchParams(location.search).get("group");
  const selectedGroupSearch = selectedGroupId
    ? `?${new URLSearchParams({ group: selectedGroupId }).toString()}`
    : "";
  const visibleNavigationItems = navigationItems.filter((item) =>
    item.capability === undefined || capabilityChecks[item.capability](session)
  );
  const visibleNavigationSections = navigationSections(visibleNavigationItems);

  const currentNavigationItem = visibleNavigationItems.find((item) => item.path === location.pathname);
  const matches = useMatches();
  const routeHandle = matches.at(-1)?.handle as RouteHandle | undefined;
  const shellVariant = routeHandle?.shell ?? "entry";

  document.title = t("app.title");

  if (shellVariant === "entry") {
    return (
      <div data-app-shell data-shell-variant={shellVariant}>
        <header data-entry-utilities>
          <UtilityControls variant="chrome" />
        </header>
        <main data-entry-main>
          <Outlet />
        </main>
      </div>
    );
  }

  return (
    <ConsoleFrame sections={visibleNavigationSections} selectedGroupSearch={selectedGroupSearch}
      title={currentNavigationItem ? t(currentNavigationItem.labelKey) : t("app.name")}>
      {routeHandle?.presentation === "library" ? <Box data-library-page><Outlet /></Box> : (
        <Box data-legacy-content bg="var(--background)" c="var(--foreground)"
          style={{ fontFamily: "var(--font-sans)", overflowX: "auto" }}>
          <Outlet />
        </Box>
      )}
    </ConsoleFrame>
  );
}
