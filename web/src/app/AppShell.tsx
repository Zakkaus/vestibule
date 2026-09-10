import { Button } from "@react-spectrum/s2/Button";
import { Content, Header, Link, Popover, SideNav, SideNavItem, SideNavItemContent, SideNavItemLink, Text } from "@react-spectrum/s2";
import { size, style } from "@react-spectrum/s2/style" with { type: "macro" };
import { DialogTrigger } from "@react-spectrum/s2/Dialog";
import { useEffect, useState } from "react";
import type { Key } from "@react-spectrum/s2";
import { useTranslation } from "react-i18next";
import { Outlet, useLocation, useMatches } from "react-router-dom";

import { UtilityControls } from "../components/UtilityControls";
import { ConsoleProvider, useConsoleSize } from "../components/ConsoleProvider";
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
const shellLayout = style({
  display: "grid",
  gridTemplateColumns: {
    default: [size(224), "minmax(0, 1fr)"],
    "@media (max-width: 48rem)": ["minmax(0, 1fr)"]
  },
  height: "screen",
  minHeight: 0,
  minWidth: 0,
  overflow: "hidden",
  boxSizing: "border-box",
  paddingStart: { default: 12, "@media (max-width: 48rem)": 0 },
  color: "neutral",
  backgroundColor: "layer-1"
});

const sidebarLayout = style({
  display: { default: "flex", "@media (max-width: 48rem)": "none" },
  flexDirection: "column",
  height: "full",
  minHeight: 0,
  minWidth: 0,
  overflow: "hidden",
  boxSizing: "border-box",
  backgroundColor: "inherit",
  borderWidth: 0
});


const brandHeaderLayout = style({
  display: "flex",
  alignItems: "center",
  gap: 8,
  minWidth: 0,
  minHeight: size(64),
  padding: 16,
  boxSizing: "border-box",
  backgroundColor: "inherit",
  borderWidth: 0
});

const brandLinkLayout = style({
  display: "flex",
  alignItems: "center",
  minWidth: 0,
  gap: 8,
  color: "neutral",
  textDecoration: "none"
});

const navigationLayout = style({
  display: "flex",
  flexDirection: "column",
  flexGrow: 1,
  minHeight: 0,
  minWidth: 0,
  height: "full",
  padding: 16,
  overflowY: "auto",
  overscrollBehaviorY: "contain",
  "--console-nav-selected-background": { type: "backgroundColor", value: "accent-900/10" }
});


const sideNavLayout = style({ height: "full", minHeight: 0 });

const mainLayout = style({
  display: "grid",
  height: "full",
  minHeight: 0,
  minWidth: 0,
  overflow: "hidden",
  boxSizing: "border-box",
  backgroundColor: "inherit"
});


const headerLayout = style({
  position: "sticky",
  top: 0,
  zIndex: 1,
  display: "grid",
  gridTemplateColumns: {
    default: ["minmax(0, 1fr)", "auto"],
    "@media (max-width: 64rem)": ["minmax(0, 1fr)"]
  },
  alignItems: "center",
  gap: 16,
  minHeight: size(64),
  paddingX: {
    default: 32,
    "@media (max-width: 48rem)": 16
  },
  paddingY: 16,
  boxSizing: "border-box",
  borderWidth: 0,
  backgroundColor: "inherit"
});

const headerTitleLayout = style({
  minWidth: 0,
  color: "neutral-subdued",
  font: "ui-lg",
  overflowWrap: "anywhere"
});

const controlsLayout = style({
  display: { default: "flex", "@media (max-width: 48rem)": "grid" },
  gridTemplateColumns: ["minmax(0, 1fr)", "minmax(0, 1fr)"],
  flexWrap: "wrap",
  alignItems: "center",
  gap: 8,
  minWidth: 0,
  gridColumn: { default: "auto", "@media (max-width: 64rem)": "1" },
  width: { default: "auto", "@media (max-width: 64rem)": "full" }
});

const contentLayout = style({
  minHeight: 0,
  minWidth: 0,
  marginEnd: {
    default: 12,
    "@media (max-width: 48rem)": 8
  },
  padding: {
    default: 32,
    "@media (max-width: 48rem)": 16
  },
  overflow: "auto",
  overscrollBehavior: "contain",
  boxSizing: "border-box",
  borderTopStartRadius: "xl",
  borderTopEndRadius: "xl",
  backgroundColor: "layer-2"
});

const innerLayout = style({
  width: "full",
  maxWidth: size(1248),
  marginX: "auto",
  minWidth: 0
});


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
  sections,
  selectedGroupSearch
}: Readonly<{
  sections: readonly NavigationSection[];
  selectedGroupSearch: string;
}>) {
  const { t } = useTranslation();
  const location = useLocation();
  const currentSection = sections.find(({ items }) =>
    items.some((item) => item.path === location.pathname)
  );
  const currentGroup = currentSection?.group.id;
  const selectedRoute = `${location.pathname}${selectedGroupSearch}`;
  const [expansion, setExpansion] = useState(() => ({
    route: selectedRoute,
    group: currentGroup ?? null
  }));
  const routeChanged = expansion.route !== selectedRoute;
  const expandedGroup = routeChanged ? currentGroup ?? null : expansion.group;
  // SideNav synchronizes focus during render, so route and expansion must agree.
  if (routeChanged) {
    setExpansion({ route: selectedRoute, group: expandedGroup });
  }


  const expandedKeys = expandedGroup === null ? [] : [expandedGroup];

  function updateExpandedGroups(keys: Set<Key>): void {
    const validKeys = [...keys].filter((key): key is NavigationGroupID =>
      typeof key === "string" && sections.some(({ group }) => group.id === key)
    );
    setExpansion({ route: selectedRoute, group: validKeys.at(-1) ?? null });
  }

  return (
    <Content UNSAFE_className="console-navigation" styles={navigationLayout}>
      <nav className={style({ display: "flex", flexDirection: "column", flexGrow: 1, minHeight: 0 })} aria-label={t("navigation.label")}>
        <SideNav
          aria-label={t("navigation.label")}
          selectedRoute={selectedRoute}
          expandedKeys={expandedKeys}
          onExpandedChange={updateExpandedGroups}
          styles={sideNavLayout}
        >
        {sections.map(({ group, items }) => (
          <SideNavItem
            key={group.id}
            id={group.id}
            textValue={t(group.labelKey)}
            data-navigation-group={group.id}
          >
            <SideNavItemContent>{t(group.labelKey)}</SideNavItemContent>
            {items.map((item) => {
              const href = `${item.path}${selectedGroupSearch}`;

              return (
                <SideNavItem key={item.path} id={item.path} href={href} textValue={t(item.labelKey)}>
                  <SideNavItemContent>
                    <SideNavItemLink>
                      <Content styles={style({ gridArea: "icon", display: "flex", alignItems: "center", marginEnd: "text-to-visual" })}>
                        <Icon name={item.icon} />
                      </Content>
                      <Text>{t(item.labelKey)}</Text>
                    </SideNavItemLink>
                  </SideNavItemContent>
                </SideNavItem>
              );
            })}
          </SideNavItem>
        ))}
      </SideNav>
      </nav>
    </Content>
  );
}

function ShellContent() {
  const { t } = useTranslation();
  const location = useLocation();
  const session = useConsoleSession();
  const controlSize = useConsoleSize("L");
  const [navigationOpen, setNavigationOpen] = useState(false);
  useEffect(() => {
    setNavigationOpen(false);
  }, [location.key]);
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
      <Content data-app-shell data-shell-variant={shellVariant}>
        <Header data-entry-utilities>
          <UtilityControls variant="chrome" />
        </Header>
        <main data-entry-main>
          <Outlet />
        </main>
      </Content>
    );
  }

  return (
    <Content
      data-app-shell
      data-shell-variant={shellVariant}
      UNSAFE_className="console-shell"
      styles={shellLayout}
    >
      <Content UNSAFE_className="console-sidebar" styles={sidebarLayout}>
        <aside className={style({ display: "flex", flexDirection: "column", height: "full", minHeight: 0 })}>
          <Header UNSAFE_className="console-brand" styles={brandHeaderLayout}>
            <Link href={`/home${selectedGroupSearch}`} variant="secondary" isStandalone isQuiet>
              <Content styles={brandLinkLayout}>
                <Icon name="shieldCheck" />
                <Text>{t("app.name")}</Text>
              </Content>
            </Link>
          </Header>
          <ConsoleNavigation
            sections={visibleNavigationSections}
            selectedGroupSearch={selectedGroupSearch}
          />
        </aside>
      </Content>
      <Content UNSAFE_className="console-main" styles={mainLayout}>
        <main className={style({ display: "grid", gridTemplateRows: ["auto", "minmax(0, 1fr)"], minHeight: 0, minWidth: 0 })}>
          <Header UNSAFE_className="console-header" styles={headerLayout} data-console-header>
            <Content UNSAFE_className="console-mobile-navigation" data-mobile-navigation styles={style({ display: { default: "none", "@media (max-width: 48rem)": "block" } })}>
              <DialogTrigger isOpen={navigationOpen} onOpenChange={setNavigationOpen}>
                <Button size={controlSize} variant="secondary" aria-haspopup="dialog" data-console-control data-control-size={controlSize}>
                  <Icon name="layoutDashboard" />
                  <Text>{t("shell.mobileNavigation")}</Text>
                </Button>
                <Popover
                  aria-label={t("navigation.label")}
                  UNSAFE_className="console-mobile-panel"
                  size="S"
                  styles={style({ maxHeight: "[70dvh]" })}
                >
                  <ConsoleNavigation
                    sections={visibleNavigationSections}
                    selectedGroupSearch={selectedGroupSearch}
                  />
                </Popover>
              </DialogTrigger>
            </Content>
            <Text UNSAFE_className="console-header-title" styles={headerTitleLayout} data-header-title>
              {currentNavigationItem ? t(currentNavigationItem.labelKey) : t("app.name")}
            </Text>
            <Content UNSAFE_className="console-controls" styles={controlsLayout}>
              <GroupSwitcher />
              <UtilityControls variant="chrome" />
            </Content>
          </Header>
          <Content UNSAFE_className="console-content" styles={contentLayout}>
            <Content key={location.pathname} UNSAFE_className="console-inner" styles={innerLayout}>
              <Outlet />
            </Content>
          </Content>
        </main>
      </Content>
    </Content>
  );
}

export function AppShell() {
  return <ConsoleProvider><ShellContent /></ConsoleProvider>;
}
