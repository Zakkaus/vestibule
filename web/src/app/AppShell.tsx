import { Button } from "@react-spectrum/s2/Button";
import { Content, Header, Link, Popover, Text } from "@react-spectrum/s2";
import { size, style } from "@react-spectrum/s2/style" with { type: "macro" };
import { DialogTrigger } from "@react-spectrum/s2/Dialog";
import { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { Outlet, useLocation, useMatches } from "react-router-dom";

import { UtilityControls } from "../components/UtilityControls";
import { ConsoleProvider, useConsoleSize } from "../components/ConsoleProvider";
import { GroupSwitcher } from "../features/groups";
import { Icon } from "../icons";
import {
  canViewInstanceStatus,
  useConsoleSession,
  type ConsoleSessionState
} from "./session";
import { ConsoleNavigation, navigationItems, navigationSections, type NavigationCapability } from "./ConsoleNavigation";

type ShellVariant = "entry" | "console";

type RouteHandle = {
  shell?: ShellVariant;
};

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
  font: "title",
  color: "neutral",
  textDecoration: "none"
});

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
  justifyContent: "end",
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
  // The panel reaches the bottom of the window on every route. Letting the home page
  // shrink to its content left a fifth of the window showing the shell behind it,
  // and overflow already keeps a long page from pushing the window taller.
  alignSelf: "stretch",
  maxHeight: "none",
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
  paddingBottom: {
    default: { default: 32, "@media (max-width: 48rem)": 16 },
    isHome: 16
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
            idPrefix="desktop"
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
                    idPrefix="mobile"
                  />
                </Popover>
              </DialogTrigger>
            </Content>
            <Content UNSAFE_className="console-controls" styles={controlsLayout}>
              <GroupSwitcher />
              <UtilityControls variant="chrome" />
            </Content>
          </Header>
          <Content
            UNSAFE_className="console-content"
            styles={contentLayout({ isHome: location.pathname === "/home" })}
          >
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
