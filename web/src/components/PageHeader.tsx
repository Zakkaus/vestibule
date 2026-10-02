import { Header } from "@react-spectrum/s2";
import { style } from "@react-spectrum/s2/style" with { type: "macro" };
import type { ComponentProps } from "react";
import { HubPages } from "../app/ConsoleHubs";

export function PageHeader({ children, ...props }: ComponentProps<typeof Header>) {
  return <Header {...props} data-page-heading styles={props.styles ?? style({ display: "flex", flexDirection: "column", gap: 8 })}>
    {children}
    <HubPages />
  </Header>;
}
