import { Content, Header, Link, Text } from "@react-spectrum/s2";
import { style } from "@react-spectrum/s2/style" with { type: "macro" };
import { ToastContainer } from "@react-spectrum/s2/Toast";
import { useEffect, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { Outlet, useLocation, useMatches } from "react-router-dom";
import { UtilityControls } from "../components/UtilityControls";
import { ConsoleProvider } from "../components/ConsoleProvider";
import { DraftProvider } from "./drafts";
import { GroupSwitcher } from "../features/groups";
import { Icon } from "../icons";
import { canViewInstanceStatus, canViewOwner, useConsoleSession, type ConsoleSessionState } from "./session";
import { ConsoleNavigation, navigationItems, navigationSections, type NavigationCapability } from "./ConsoleNavigation";
import { HubBar, HubContext } from "./ConsoleHubs";

const shellLayout = style({ display: "grid", gridTemplateColumns: { default: "[240px minmax(0,1fr)]", "@media (max-width: 1023px)": "[minmax(0,1fr)]" }, gridTemplateRows: "[auto 1fr]", minHeight: "screen", maxWidth: "[1600px]", marginX: "auto", minWidth: 0, backgroundColor: "layer-1", color: "neutral" });
const headerLayout = style({ gridColumnStart: 1, gridColumnEnd: -1, position: "sticky", top: 0, zIndex: 2, display: "grid", gridTemplateColumns: { default: "[216px minmax(0,1fr) auto]", "@media (max-width: 1023px)": "[auto minmax(0,1fr) auto]" }, alignItems: "center", minHeight: "[64px]", gap: { default: 24, "@media (max-width: 399px)": 8 }, paddingX: { default: 24, "@media (max-width: 399px)": 12 }, backgroundColor: "inherit" });
const sidebarLayout = style({ display: { default: "flex", "@media (max-width: 1023px)": "none" }, flexDirection: "column", minWidth: 0, alignSelf: "start", position: "sticky", top: "[64px]" });
const contentLayout = style({ minWidth: 0, marginEnd: { default: 16, "@media (max-width: 1023px)": 0 }, paddingTop: { default: 32, "@media (max-width: 600px)": 20 }, paddingX: { default: 40, "@media (max-width: 600px)": 16 }, borderTopStartRadius: "xl", borderTopEndRadius: "xl", backgroundColor: "layer-2" });
const capabilityChecks: Readonly<Record<NavigationCapability, (state: ConsoleSessionState) => boolean>> = { "instance-status": canViewInstanceStatus, owner: canViewOwner };

function useNarrowLayout() {
  const [narrow, setNarrow] = useState(() => window.matchMedia("(max-width: 1023px)").matches);
  useEffect(() => {
    const media = window.matchMedia("(max-width: 1023px)");
    const update = () => setNarrow(media.matches);
    media.addEventListener("change", update);
    return () => media.removeEventListener("change", update);
  }, []);
  return narrow;
}

function ShellContent() {
  const { t } = useTranslation();
  const location = useLocation();
  const previousPath = useRef(location.pathname);
  const session = useConsoleSession();
  const narrow = useNarrowLayout();
  const selectedGroupId = new URLSearchParams(location.search).get("group");
  const search = selectedGroupId ? `?${new URLSearchParams({ group: selectedGroupId })}` : "";
  const sections = navigationSections(navigationItems.filter(item => !item.capability || capabilityChecks[item.capability](session)));
  const matches = useMatches();
  const handle = matches.at(-1)?.handle as { shell?: "entry" | "console" } | undefined;
  const variant = handle?.shell ?? "entry";
  useEffect(() => { document.title = t("app.title"); }, [t]);
  useEffect(() => {
    if (previousPath.current === location.pathname) return;
    previousPath.current = location.pathname;
    const heading = document.querySelector<HTMLElement>(".console-inner h1");
    if (heading) { heading.tabIndex = -1; heading.focus({ preventScroll: true }); }
  }, [location.pathname]);
  if (variant === "entry") return (
    <Content data-app-shell data-shell-variant="entry">
      <Header data-entry-utilities><UtilityControls variant="chrome" /></Header>
      <main data-entry-main><Outlet /></main>
    </Content>
  );
  return (
    <HubContext.Provider value={{ sections, search, narrow }}>
      <Content data-app-shell data-shell-variant="console" UNSAFE_className="console-shell" styles={shellLayout}>
        <Header data-console-header UNSAFE_className="console-header" styles={headerLayout}>
          <Link href={`/home${search}`} aria-label={t("app.name")} variant="secondary" isStandalone isQuiet>
            <Content styles={style({ display: "flex", alignItems: "center", gap: 8 })}>
              <Icon name="shieldCheck" /><Text styles={style({ display: { default: "block", "@media (max-width: 420px)": "none" }, whiteSpace: "nowrap" })}>{t("app.name")}</Text>
            </Content>
          </Link>
          <div className={style({ minWidth: 0 })}>{narrow ? <GroupSwitcher compact /> : null}</div>
          <UtilityControls variant="console" />
        </Header>
        <Content {...{ role: "complementary" }} UNSAFE_className="console-sidebar" styles={sidebarLayout}>
          <ConsoleNavigation sections={sections} selectedGroupSearch={search} idPrefix="desktop" />
          <div className={style({ padding: 16, minWidth: 0, flexShrink: 0 })}>{!narrow ? <GroupSwitcher /> : null}</div>
        </Content>
        <Content {...{ role: "main" }} UNSAFE_className="console-content" styles={contentLayout}>
          <Content UNSAFE_className="console-inner" styles={style({ width: "full", maxWidth: "[1120px]", marginX: "auto", minWidth: 0 })}><Outlet /></Content>
        </Content>
        <HubBar />
      </Content>
    </HubContext.Provider>
  );
}

export function AppShell() {
  return <ConsoleProvider><DraftProvider><ShellContent /><ToastContainer placement="bottom" data-console-toasts /></DraftProvider></ConsoleProvider>;
}
