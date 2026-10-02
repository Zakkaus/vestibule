import { Button, ButtonGroup, InlineAlert, Text, ToastQueue } from "@react-spectrum/s2";
import { useEffect, useRef, type ReactNode } from "react";
import { useTranslation } from "react-i18next";
import type { ApiRequestError } from "../lib/api";

export type FeedbackLevel = "neutral" | "info" | "positive" | "negative" | "warning";
export function writeOutcomeUnknown(error: ApiRequestError): boolean {
  return error.kind !== "api";
}

export function Feedback({ level, message, children, onRefetch, unknown = false, ...attributes }: Readonly<{
  level: FeedbackLevel; message: string; children?: ReactNode; onRefetch?: () => void; unknown?: boolean;
}> & Record<`data-${string}`, string | boolean | undefined>) {
  const { t } = useTranslation();
  const text = unknown ? t("drafts.unknownOutcome") : message;
  const tone = unknown ? "info" : level;
  const previous = useRef("");
  const inline = tone === "warning" || tone === "negative" || Boolean(onRefetch);
  useEffect(() => {
    const key = `${tone}:${text}`;
    if (tone === "warning" || previous.current === key) return;
    previous.current = key;
    ToastQueue[tone](text, { timeout: 5000, ...(inline ? {} : attributes) });
  }, [text, tone]);
  if (!inline) return null;
  return (
    <InlineAlert {...attributes} data-feedback-level={tone} variant={tone === "warning" ? "notice" : tone === "info" ? "informative" : "negative"}>
      <Text>{unknown ? text : children ?? text}</Text>
      {onRefetch ? <ButtonGroup><Button variant="secondary" onPress={onRefetch}>{t("drafts.refetch")}</Button></ButtonGroup> : null}
    </InlineAlert>
  );
}
