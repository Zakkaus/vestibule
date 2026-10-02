import type { ReactNode } from "react";
import { style } from "@react-spectrum/s2/style" with { type: "macro" };

export function SettingsFieldRow({ children, ...attributes }: Readonly<{ children: ReactNode }> & Record<`data-${string}`, string | boolean | undefined>) {
  return <div {...attributes} className={style({ display: "flex", flexDirection: "column", alignItems: "start", minWidth: 0, gap: 4 })}>{children}</div>;
}
