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
  selectedGroupSearch
}: Readonly<{ sections: readonly NavigationSection[]; selectedGroupSearch: string }>) {
  const { t } = useTranslation();
  const location = useLocation();

  return (
    <nav className="nav" aria-label={t("navigation.label")}>
      {sections.map(({ group, items }) => (
        <div key={group.id} className="nav-group" data-navigation-group={group.id}>
          <span className="nav-label">{t(group.labelKey)}</span>
          {items.map((item) => {
            const isActive = location.pathname === item.path;

            return (
              <Link
                key={item.path}
                className="nav-item"
                to={{ pathname: item.path, search: selectedGroupSearch }}
                aria-current={isActive ? "page" : undefined}
                data-active={isActive ? "" : undefined}
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
    <div data-app-shell data-shell-variant={shellVariant} className="shell">
      <aside className="shell-aside" data-admin>
        <Link
          className="brand"
          to={{ pathname: "/groups", search: selectedGroupSearch }}
        >
          {/* Placeholder mark: the same borrowed icon as the favicon. */}
          <Icon name="shieldCheck" />
          <span className="name">{t("app.name")}</span>
        </Link>
        <div className="rule" />
        <ConsoleNavigation
          sections={visibleNavigationSections}
          selectedGroupSearch={selectedGroupSearch}
        />
      </aside>
      <div className="shell-main">
        <header className="shell-header" data-console-header>
          <details data-mobile-navigation>
            <summary>{t("shell.mobileNavigation")}</summary>
            <ConsoleNavigation
              sections={visibleNavigationSections}
              selectedGroupSearch={selectedGroupSearch}
            />
          </details>
          <span data-header-title>
            {currentNavigationItem ? t(currentNavigationItem.labelKey) : t("app.name")}
          </span>
          <div data-header-controls>
            <GroupSwitcher />
            <UtilityControls variant="chrome" />
          </div>
        </header>
        <main className="shell-content">
          <div className="shell-inner">
            <Outlet />
          </div>
        </main>
      </div>
    </div>
  );
}
