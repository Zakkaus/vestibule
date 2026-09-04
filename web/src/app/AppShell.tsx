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

type NavigationItem = Readonly<{
  path: string;
  labelKey: string;
  icon: IconName;
  capability?: NavigationCapability;
}>;

const capabilityChecks: Readonly<
  Record<NavigationCapability, (state: ConsoleSessionState) => boolean>
> = {
  "instance-status": canViewInstanceStatus
};

const navigationItems: readonly NavigationItem[] = [
  {
    path: "/home",
    labelKey: "home.navigation",
    icon: "layoutDashboard"
  },
  {
    path: "/queue",
    labelKey: "navigation.queue",
    icon: "inbox"
  },
  {
    path: "/audit",
    labelKey: "audit.navigation",
    icon: "clipboardList"
  },
  {
    path: "/stats",
    labelKey: "stats.navigation",
    icon: "chartNoAxesCombined"
  },
  {
    path: "/diagnostics",
    labelKey: "diagnostics.navigation",
    icon: "activity"
  },
  {
    path: "/verification",
    labelKey: "verification.navigation",
    icon: "shieldCheck"
  },
  {
    path: "/bypass",
    labelKey: "bypass.navigation",
    icon: "shieldOff"
  },
  {
    path: "/questions",
    labelKey: "questions.navigation",
    icon: "circleHelp"
  },
  {
    path: "/feeds",
    labelKey: "feeds.navigation",
    icon: "rss"
  },
  {
    path: "/groups",
    labelKey: "navigation.groups",
    icon: "usersRound"
  },
  {
    path: "/moderation",
    labelKey: "moderation.navigation",
    icon: "shieldAlert"
  },
  {
    path: "/messages",
    labelKey: "messages.navigation",
    icon: "messagesSquare"
  },
  {
    path: "/version",
    labelKey: "version.navigation",
    icon: "refreshCw",
    capability: "instance-status"
  },
  {
    path: "/capabilities",
    labelKey: "capabilities.navigation",
    icon: "slidersHorizontal"
  },
  {
    path: "/preferences",
    labelKey: "navigation.preferences",
    icon: "settings"
  }
];

function ConsoleNavigation({
  items,
  selectedGroupSearch
}: Readonly<{ items: readonly NavigationItem[]; selectedGroupSearch: string }>) {
  const { t } = useTranslation();
  const location = useLocation();

  return (
    <nav className="nav" aria-label={t("navigation.label")}>
      <div className="nav-group">
        <span className="nav-label">{t("navigation.workspace")}</span>
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
        <ConsoleNavigation items={visibleNavigationItems} selectedGroupSearch={selectedGroupSearch} />
      </aside>
      <div className="shell-main">
        <header className="shell-header" data-console-header>
          <details data-mobile-navigation>
            <summary>{t("shell.mobileNavigation")}</summary>
            <ConsoleNavigation items={visibleNavigationItems} selectedGroupSearch={selectedGroupSearch} />
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
