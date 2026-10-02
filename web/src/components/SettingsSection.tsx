import { Content, Heading } from "@react-spectrum/s2";
import { style } from "@react-spectrum/s2/style" with { type: "macro" };
import type { ReactNode } from "react";

export function SettingsSection({ id, title, description, children, ...attributes }: Readonly<{
  id: string; title: string; description: string; children: ReactNode;
}> & Record<`data-${string}`, string | boolean | undefined>) {
  return (
    <section {...attributes} aria-labelledby={id} className={style({ display: "flex", flexDirection: "column", gap: 12, padding: 16, backgroundColor: "layer-1", borderWidth: 1, borderStyle: "solid", borderColor: "gray-300", borderRadius: "xl", minWidth: 0 })}>
      <Heading id={id} level={2} styles={style({ marginY: 0, font: "heading-sm" })}>{title}</Heading>
      <Content styles={style({ color: "neutral-subdued", font: "body-sm" })}>{description}</Content>
      <div data-settings-field-grid className={style({ display: "grid", gridTemplateColumns: "[repeat(auto-fill,minmax(min(100%,240px),1fr))]", gap: 12, alignItems: "start", minWidth: 0 })}>{children}</div>
    </section>
  );
}
