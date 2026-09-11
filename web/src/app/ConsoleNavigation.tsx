import { Content, Text } from "@react-spectrum/s2";
import type { ReactNode } from "react";
import {
  SideNav,
  SideNavHeader,
  SideNavItem,
  SideNavItemContent,
  SideNavItemLink,
  SideNavSection,
  type SideNavItemLinkProps
} from "@react-spectrum/s2/SideNav";
import { size, style } from "@react-spectrum/s2/style" with { type: "macro" };
import { useLocation } from "react-router-dom";
import { useTranslation } from "react-i18next";

import { Icon, type IconName } from "../icons";
export type NavigationCapability = "instance-status";

export type NavigationGroupID =
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

export type NavigationItem = Readonly<{
  path: string;
  labelKey: string;
  icon: IconName;
  group: NavigationGroupID;
  capability?: NavigationCapability;
}>;

export type NavigationSection = Readonly<{
  group: NavigationGroup;
  items: readonly NavigationItem[];
}>;

const navigationGroups: readonly NavigationGroup[] = [
  { id: "daily", labelKey: "navigation.sections.daily" },
  { id: "verification", labelKey: "navigation.sections.verification" },
  { id: "group", labelKey: "navigation.sections.group" },
  { id: "content", labelKey: "navigation.sections.content" },
  { id: "observe", labelKey: "navigation.sections.observe" },
  { id: "console", labelKey: "navigation.sections.console" }
];

export const navigationItems: readonly NavigationItem[] = [
  { path: "/home", labelKey: "home.navigation", icon: "layoutDashboard", group: "daily" },
  { path: "/queue", labelKey: "navigation.queue", icon: "inbox", group: "daily" },
  { path: "/audit", labelKey: "audit.navigation", icon: "clipboardList", group: "daily" },
  { path: "/verification", labelKey: "verification.navigation", icon: "shieldCheck", group: "verification" },
  { path: "/questions", labelKey: "questions.navigation", icon: "circleHelp", group: "verification" },
  { path: "/bypass", labelKey: "bypass.navigation", icon: "shieldOff", group: "verification" },
  { path: "/groups", labelKey: "navigation.groups", icon: "usersRound", group: "group" },
  { path: "/moderation", labelKey: "moderation.navigation", icon: "shieldAlert", group: "group" },
  { path: "/messages", labelKey: "messages.navigation", icon: "messagesSquare", group: "group" },
  { path: "/feeds", labelKey: "feeds.navigation", icon: "rss", group: "content" },
  { path: "/stats", labelKey: "stats.navigation", icon: "chartNoAxesCombined", group: "observe" },
  { path: "/diagnostics", labelKey: "diagnostics.navigation", icon: "activity", group: "observe" },
  { path: "/version", labelKey: "version.navigation", icon: "refreshCw", group: "console", capability: "instance-status" },
  { path: "/capabilities", labelKey: "capabilities.navigation", icon: "slidersHorizontal", group: "console" },
  { path: "/preferences", labelKey: "navigation.preferences", icon: "settings", group: "console" }
];


const navigationLayout = style({
  flexGrow: 1,
  minHeight: 0,
  minWidth: 0,
  height: "full"
});

const navigationFrame = style({
  display: "flex",
  flexGrow: 1,
  minHeight: 0,
  minWidth: 0,
  padding: size(16),
  boxSizing: "border-box"
});

export function navigationSections(items: readonly NavigationItem[]): readonly NavigationSection[] {
  const sections: NavigationSection[] = [];
  for (const group of navigationGroups) {
    const groupItems = items.filter((item) => item.group === group.id);
    if (groupItems.length > 0) sections.push({ group, items: groupItems });
  }
  return sections;
}

// Every section stays open. The console has fifteen destinations in six groups; behind
// an accordion the sidebar showed one group at a time and read as an empty column, and
// finding a screen cost a click before it cost a glance. SideNav carries the section
// header, the row rhythm and the current-item indicator, so none of that is written here.
// SideNavItemLink forwards every prop it receives to react-aria-components' Link, but
// its declared props stop at children. Name what we actually pass rather than casting
// the call site to any.
const NavigationLink = SideNavItemLink as (
  props: SideNavItemLinkProps & { href: string } & Record<`data-${string}`, string>
) => ReactNode;

export function ConsoleNavigation({
  sections,
  selectedGroupSearch,
  idPrefix
}: Readonly<{
  sections: readonly NavigationSection[];
  selectedGroupSearch: string;
  idPrefix: string;
}>) {
  const { t } = useTranslation();
  const location = useLocation();
  const selectedRoute = `${location.pathname}${selectedGroupSearch}`;

  return (
    <Content UNSAFE_className="console-navigation" styles={navigationFrame}>
    <SideNav
      aria-label={t("navigation.label")}
      selectedRoute={selectedRoute}
      styles={navigationLayout}
    >
      {sections.map((section) => (
        <SideNavSection key={section.group.id} id={`${idPrefix}-${section.group.id}`}>
          <SideNavHeader>
            <Text data-navigation-group={section.group.id}>{t(section.group.labelKey)}</Text>
          </SideNavHeader>
          {section.items.map((item) => (
            <SideNavItem
              key={item.path}
              id={`${idPrefix}-${item.path}`}
              // The tree matches selectedRoute against the item's own href, not the
              // link's, so the current destination is marked by the library rather
              // than by a rule of ours.
              href={`${item.path}${selectedGroupSearch}`}
              textValue={t(item.labelKey)}
            >
              <SideNavItemContent>
                <NavigationLink
                  href={`${item.path}${selectedGroupSearch}`}
                  data-navigation-item={item.path}
                >
                  <Icon name={item.icon} />
                  <Text>{t(item.labelKey)}</Text>
                </NavigationLink>
              </SideNavItemContent>
            </SideNavItem>
          ))}
        </SideNavSection>
      ))}
    </SideNav>
    </Content>
  );
}
