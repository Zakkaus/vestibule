import { useTranslation } from "react-i18next";
import {
  Link,
  Outlet,
  useLocation,
  useMatches
} from "react-router-dom";

import { UtilityControls } from "../components/UtilityControls";
import { GroupSwitcher } from "../features/groups";

type ShellVariant = "entry" | "console";

type RouteHandle = {
  shell?: ShellVariant;
};

const navigationItems = [
  {
    path: "/queue",
    labelKey: "navigation.queue"
  },
  {
    path: "/groups",
    labelKey: "navigation.groups"
  }
] as const;

function ConsoleNavigation() {
  const { t } = useTranslation();
  const location = useLocation();

  return (
    <nav className="nav" aria-label={t("navigation.label")}>
      <div className="nav-group">
        <span className="nav-label">{t("navigation.workspace")}</span>
        {navigationItems.map((item) => {
          const isActive = location.pathname === item.path;

          return (
            <Link
              key={item.path}
              className="nav-item"
              to={item.path}
              aria-current={isActive ? "page" : undefined}
              data-active={isActive ? "" : undefined}
            >
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
  const currentNavigationItem = navigationItems.find((item) => item.path === location.pathname);
  const matches = useMatches();
  const routeHandle = matches.at(-1)?.handle as RouteHandle | undefined;
  const shellVariant = routeHandle?.shell ?? "entry";

  document.title = t("app.title");

  if (shellVariant === "entry") {
    return (
      <div data-app-shell data-shell-variant={shellVariant}>
        <header data-entry-utilities>
          <UtilityControls />
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
        <Link className="brand" to="/groups">
          <span className="name">{t("app.name")}</span>
        </Link>
        <div className="rule" />
        <ConsoleNavigation />
      </aside>
      <div className="shell-main">
        <header className="shell-header" data-console-header>
          <details data-mobile-navigation>
            <summary>{t("shell.mobileNavigation")}</summary>
            <ConsoleNavigation />
          </details>
          <span data-header-title>
            {currentNavigationItem ? t(currentNavigationItem.labelKey) : t("app.name")}
          </span>
          <div data-header-controls>
            <GroupSwitcher />
            <UtilityControls />
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
