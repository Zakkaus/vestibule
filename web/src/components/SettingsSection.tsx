import { Content, Heading } from "@react-spectrum/s2";
import { style } from "@react-spectrum/s2/style" with { type: "macro" };
import type { ReactNode } from "react";

export function SettingsSection({ id, title, description, children, ...attributes }: Readonly<{
  id: string; title: string; description: string; children: ReactNode;
}> & Record<`data-${string}`, string | boolean | undefined>) {
  return (
    <section {...attributes} aria-labelledby={id} className={style({ display: "flex", flexDirection: "column", gap: 12, padding: 16, backgroundColor: "layer-1", borderRadius: "lg", minWidth: 0 })}>
      <Heading id={id} level={2} styles={style({ marginY: 0 })}>{title}</Heading>
      <Content styles={style({ color: "neutral-subdued" })}>{description}</Content>
      {children}
    </section>
  );
}
