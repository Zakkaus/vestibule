import type { PropsWithChildren } from "react";

export const statusTones = ["ok", "info", "pending", "error", "neutral"] as const;

export type StatusTone = (typeof statusTones)[number];

type StatusBadgeProps = PropsWithChildren<{
  tone: StatusTone;
}>;

export function StatusBadge({ tone, children }: StatusBadgeProps) {
  return (
    <span data-slot="badge" data-status={tone}>
      {children}
    </span>
  );
}
