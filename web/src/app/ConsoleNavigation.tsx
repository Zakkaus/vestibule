import { Accordion } from "@react-spectrum/s2/Accordion";
import { Disclosure, DisclosurePanel } from "@react-spectrum/s2/Disclosure";
import { Content, Heading, Text } from "@react-spectrum/s2";
import { focusRing, size, style } from "@react-spectrum/s2/style" with { type: "macro" };
import { Button } from "react-aria-components/Button";
import { Link } from "react-aria-components/Link";
import { useState } from "react";
import { useLocation } from "react-router-dom";
import { useTranslation } from "react-i18next";
import type { Key } from "@react-spectrum/s2";

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
  display: "flex",
  flexDirection: "column",
  flexGrow: 1,
  minHeight: 0,
  minWidth: 0,
  height: "full",
  padding: size(16),
  overflowY: "auto",
  overscrollBehaviorY: "contain"
});

const navigationAccordionLayout = style({ minWidth: 0 });

const navigationSectionLayout = style({
  minWidth: 0,
  marginTop: { default: size(64), ":first-child": 0 }
});

const navigationItemsLayout = style({
  display: "grid",
  minWidth: 0,
  gap: size(6)
});

const navigationIconLayout = style({
  display: "flex",
  alignItems: "center",
  justifyContent: "center",
  width: size(16),
  height: size(16),
  flexShrink: 0
});

export function navigationSections(items: readonly NavigationItem[]): readonly NavigationSection[] {
  const sections: NavigationSection[] = [];
  for (const group of navigationGroups) {
    const groupItems = items.filter((item) => item.group === group.id);
    if (groupItems.length > 0) sections.push({ group, items: groupItems });
  }
  return sections;
}

type NavigationGroupViewProps = Readonly<{
  section: NavigationSection;
  expandedGroup: NavigationGroupID | null;
  selectedGroupSearch: string;
  pathname: string;
  idPrefix: string;
  translate: (key: string) => string;
}>;

function NavigationGroupView({
  section,
  expandedGroup,
  selectedGroupSearch,
  pathname,
  idPrefix,
  translate
}: NavigationGroupViewProps) {
  const { group, items } = section;
  const isExpanded = expandedGroup === group.id;
  const buttonID = `${idPrefix}-${group.id}-navigation-group`;
  const panelID = `${idPrefix}-${group.id}-navigation-items`;

  return (
    <Disclosure
      id={group.id}
      data-navigation-section={group.id}
      isQuiet
      styles={navigationSectionLayout}
    >
      <Heading level={3} styles={style({ margin: 0 })}>
        <Button
          id={buttonID}
          slot="trigger"
          data-navigation-group={group.id}
          className={style({
            ...focusRing(),
            outlineStyle: { default: "none", ":focus-visible": "solid" },
            display: "flex",
            alignItems: "center",
            width: "full",
            minHeight: size(32),
            gap: size(6),
            padding: 0,
            borderWidth: 0,
            borderRadius: "none",
            color: "neutral-subdued",
            backgroundColor: "transparent",
            textAlign: "start",
            font: "ui-sm",
            fontSize: `[${size(14)}]`,
            fontWeight: "normal",
            lineHeight: `[${size(32)}]`
          })}
        >
          <Content styles={navigationIconLayout}>
            <Icon name={isExpanded ? "chevronDown" : "chevronRight"} />
          </Content>
          <Text>{translate(group.labelKey)}</Text>
        </Button>
      </Heading>
      <DisclosurePanel
        id={panelID}
        role="group"
        aria-labelledby={buttonID}
        data-navigation-items={group.id}
      >
        <Content styles={navigationItemsLayout}>
          {items.map((item) => {
            const href = `${item.path}${selectedGroupSearch}`;
            const isSelected = item.path === pathname;
            return (
              <Link
                key={item.path}
                href={href}
                data-navigation-item={item.path}
                aria-current={isSelected ? "page" : undefined}
                className={style({
                  ...focusRing(),
                  outlineStyle: { default: "none", ":focus-visible": "solid" },
                  textDecoration: { default: "none", "@media (hover: hover)": { ":hover": "underline" } },
                  display: "flex",
                  alignItems: "center",
                  width: "full",
                  minHeight: size(32),
                  boxSizing: "border-box",
                  gap: size(6),
                  font: "ui-sm",
                  fontSize: `[${size(14)}]`,
                  lineHeight: `[${size(32)}]`,
                  borderRadius: "none",
                  color: { default: "neutral-subdued", ":where([aria-current=page])": "neutral" },
                  fontWeight: { default: "normal", ":where([aria-current=page])": "bold" }
                })}
              >
                <Content styles={navigationIconLayout}>
                  <Icon name={item.icon} />
                </Content>
                <Text>{translate(item.labelKey)}</Text>
              </Link>
            );
          })}
        </Content>
      </DisclosurePanel>
    </Disclosure>
  );
}

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

  if (routeChanged) {
    setExpansion({ route: selectedRoute, group: expandedGroup });
  }

  function updateExpandedGroups(keys: Set<Key>): void {
    const expandedSection = sections.find(({ group }) => keys.has(group.id));
    setExpansion({ route: selectedRoute, group: expandedSection?.group.id ?? null });
  }

  return (
    <Content UNSAFE_className="console-navigation" styles={navigationLayout}>
      <nav className={style({ display: "flex", flexDirection: "column", flexGrow: 1, minHeight: 0, minWidth: 0 })} aria-label={t("navigation.label")}>
        <Accordion
          isQuiet
          size="S"
          density="compact"
          allowsMultipleExpanded={false}
          expandedKeys={expandedGroup === null ? [] : [expandedGroup]}
          onExpandedChange={updateExpandedGroups}
          styles={navigationAccordionLayout}
        >
          {sections.map((section) => (
            <NavigationGroupView
              key={section.group.id}
              section={section}
              expandedGroup={expandedGroup}
              selectedGroupSearch={selectedGroupSearch}
              pathname={location.pathname}
              idPrefix={idPrefix}
              translate={t}
            />
          ))}
        </Accordion>
      </nav>
    </Content>
  );
}
