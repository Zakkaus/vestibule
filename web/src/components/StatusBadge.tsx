import type { PropsWithChildren } from "react";
import { Badge } from "@mantine/core";
import { Icon, type IconName } from "../icons";

export const statusTones = ["ok", "info", "pending", "error", "neutral"] as const;

export type StatusTone = (typeof statusTones)[number];

type StatusBadgeProps = PropsWithChildren<{
  tone: StatusTone;
  presentation?: "library";
}>;
const iconByTone: Record<StatusTone, IconName> = {
  ok: "circleCheck",
  info: "info",
  pending: "loaderCircle",
  error: "circleAlert",
  neutral: "circleMinus"
};

const colorByTone: Record<StatusTone, string> = {
  ok: "teal",
  info: "blue",
  pending: "yellow",
  error: "red",
  neutral: "gray"
};


export function StatusBadge({ tone, children, presentation }: StatusBadgeProps) {
  if (presentation === "library") {
    return (
      <Badge variant="filled" color={colorByTone[tone]} leftSection={<Icon name={iconByTone[tone]} />}>
        {children}
      </Badge>
    );
  }
  return (
    <span data-slot="badge" data-status={tone}>
      <Icon name={iconByTone[tone]} />
      {children}
    </span>
  );
}
