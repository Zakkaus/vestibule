import { Button } from "@react-spectrum/s2/Button";
import { DialogTrigger } from "@react-spectrum/s2/Dialog";
import { Popover } from "@react-spectrum/s2/Popover";
import { Text } from "@react-spectrum/s2/Text";
import { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import {
  Link,
  Outlet,
  useLocation,
  useMatches
} from "react-router-dom";

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
  selectedGroupSearch,
  onNavigate
}: Readonly<{
  sections: readonly NavigationSection[];
  selectedGroupSearch: string;
  onNavigate?: () => void;
}>) {
  const { t } = useTranslation();
  const location = useLocation();

  return (
    <nav className="console-navigation" aria-label={t("navigation.label")}>
      {sections.map(({ group, items }) => (
        <div key={group.id} className="console-nav-group" data-navigation-group={group.id}>
          <span className="console-nav-label">{t(group.labelKey)}</span>
          {items.map((item) => {
            const isActive = location.pathname === item.path;

            return (
              <Link
                key={item.path}
                className="console-nav-link"
                to={{ pathname: item.path, search: selectedGroupSearch }}
                aria-current={isActive ? "page" : undefined}
                onClick={onNavigate}
              >
                <Icon name={item.icon} />
                {t(item.labelKey)}
              </Link>
            );
          })}
        </div>
      ))}
    </nav>
  );
}


function ShellContent() {
  const { t } = useTranslation();
  const location = useLocation();
  const session = useConsoleSession();
  const [navigationOpen, setNavigationOpen] = useState(false);
  const size = useConsoleSize("L");
  useEffect(() => { setNavigationOpen(false); }, [location.pathname]);
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
    <div data-app-shell data-shell-variant={shellVariant} className="console-shell">
      <aside className="console-sidebar">
        <Link
          className="console-brand"
          to={{ pathname: "/home", search: selectedGroupSearch }}
        >
          <Icon name="shieldCheck" />
          <span>{t("app.name")}</span>
        </Link>
        <ConsoleNavigation
          sections={visibleNavigationSections}
          selectedGroupSearch={selectedGroupSearch}
        />
      </aside>
      <div className="console-main">
        <header className="console-header" data-console-header>
          <div className="console-mobile-navigation" data-mobile-navigation>
            <DialogTrigger isOpen={navigationOpen} onOpenChange={setNavigationOpen}>
              <Button size={size} variant="secondary" aria-haspopup="dialog" data-console-control data-control-size={size}>
                <Icon name="layoutDashboard" /><Text>{t("shell.mobileNavigation")}</Text>
              </Button>
              <Popover aria-label={t("navigation.label")} UNSAFE_className="console-mobile-panel">
                <ConsoleNavigation
                  sections={visibleNavigationSections}
                  selectedGroupSearch={selectedGroupSearch}
                  onNavigate={() => setNavigationOpen(false)}
                />
              </Popover>
            </DialogTrigger>
          </div>
          <span data-header-title>
            {currentNavigationItem ? t(currentNavigationItem.labelKey) : t("app.name")}
          </span>
          <div className="console-controls">
            <GroupSwitcher />
            <UtilityControls variant="chrome" />
          </div>
        </header>
        <main className="console-content">
          <div key={location.pathname} className="console-inner">
            <Outlet />
          </div>
        </main>
      </div>
    </div>
  );
}

export function AppShell() {
  return <ConsoleProvider><ShellContent /></ConsoleProvider>;
}
