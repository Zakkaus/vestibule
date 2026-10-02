import { Link } from "react-aria-components";
import { style } from "@react-spectrum/s2/style" with { type: "macro" };
import { createContext, useContext, useEffect, useState } from "react";
import { useLocation } from "react-router-dom";
import { useTranslation } from "react-i18next";
import { Icon } from "../icons";
import type { NavigationSection } from "./ConsoleNavigation";

export const HubContext = createContext<Readonly<{ sections: readonly NavigationSection[]; search: string; narrow: boolean }>>({ sections: [], search: "", narrow: false });
const hubPageStorage = "verify-console-hub-pages";
function readLastPages(): Record<string, string> {
  try { return JSON.parse(sessionStorage.getItem(hubPageStorage) ?? "{}") ?? {}; }
  catch { return {}; }
}

export function HubBar() {
  const { sections, search } = useContext(HubContext);
  const { pathname } = useLocation();
  const { t } = useTranslation();
  const [last, setLast] = useState(readLastPages);
  const current = sections.find(section => section.items.some(item => item.path === pathname));
  useEffect(() => {
    if (!current) return;
    setLast(previous => ({ ...previous, [current.group.id]: pathname }));
  }, [current?.group.id, pathname]);
  useEffect(() => {
    try { sessionStorage.setItem(hubPageStorage, JSON.stringify(last)); } catch { /* Memory still works without storage. */ }
  }, [last]);
  return (
    <nav data-hub-bar aria-label={t("navigation.label")} className={style({ display: { default: "none", "@media (max-width: 1023px)": "flex" }, position: "fixed", bottom: 0, insetX: 0, zIndex: 3, backgroundColor: "layer-1", paddingX: 8 })}>
      {sections.map(section => {
        const destination = section.items.find(item => item.path === last[section.group.id]) ?? section.items[0];
        if (!destination) return null;
        const selected = current?.group.id === section.group.id;
        return <Link key={section.group.id} href={`${destination.path}${search}`} aria-current={selected ? "page" : undefined} data-hub={section.group.id}
          className={style({ flexGrow: 1, flexBasis: 0, minWidth: 0, height: "[64px]", display: "flex", flexDirection: "column", justifyContent: "center", alignItems: "center", gap: 4, font: "body-xs", whiteSpace: "nowrap", textDecoration: "none", color: "neutral" })}>
          <span className={style({ display: "flex", justifyContent: "center", alignItems: "center", width: "[56px]", height: "[32px]", borderRadius: "pill", backgroundColor: { default: "transparent", isSelected: "accent-subtle" } })({ isSelected: selected })}><Icon name={section.items[0]!.icon} /></span>
          {t(section.group.labelKey)}
        </Link>;
      })}
    </nav>
  );
}

export function HubPages() {
  const { sections, search, narrow } = useContext(HubContext);
  const { pathname } = useLocation();
  const { t } = useTranslation();
  const current = sections.find(section => section.items.some(item => item.path === pathname));
  if (!current || !narrow) return null;
  return <nav data-hub-pages aria-label={t(current.group.labelKey)} className={style({ display: { default: "none", "@media (max-width: 1023px)": "flex" }, flexWrap: "wrap", gap: 8, marginTop: 8 })}>
    {current.items.map(item => <Link key={item.path} href={`${item.path}${search}`} data-navigation-item={item.path} aria-current={item.path === pathname ? "page" : undefined}
      className={style({ whiteSpace: "nowrap", textDecoration: "none", color: "neutral", font: "body-sm", fontWeight: { default: "normal", isSelected: "bold" } })({ isSelected: item.path === pathname })}>{t(item.labelKey)}</Link>)}
  </nav>;
}
