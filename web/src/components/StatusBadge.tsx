import type { PropsWithChildren } from "react";
import { Icon, type IconName } from "../icons";

export const statusTones = ["ok", "info", "pending", "error", "neutral"] as const;

export type StatusTone = (typeof statusTones)[number];

type StatusBadgeProps = PropsWithChildren<{
  tone: StatusTone;
}>;
const iconByTone: Record<StatusTone, IconName> = {
  ok: "circleCheck",
  info: "info",
  pending: "loaderCircle",
  error: "circleAlert",
  neutral: "circleMinus"
};


export function StatusBadge({ tone, children }: StatusBadgeProps) {
  return (
    <span data-slot="badge" data-status={tone}>
      <Icon name={iconByTone[tone]} />
      {children}
    </span>
  );
}
