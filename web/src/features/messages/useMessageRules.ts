import { useEffect, useRef, useState } from "react";

import { consoleApi } from "../../app/session";
import type { ApiRequestError } from "../../lib/api";
import {
  loadMessageRules,
  replaceMessageRuleCollection,
  updateMessageRule,
  type MessageRule
} from "./api";

export type MessageRulesState =
  | Readonly<{ kind: "loading" }>
  | Readonly<{ kind: "loaded"; items: readonly MessageRule[] }>
  | Readonly<{ kind: "unavailable"; error: ApiRequestError }>;

export type MessageRulesBusy =
  | Readonly<{ kind: "item"; id: string }>
  | Readonly<{ kind: "collection"; collection: string }>
  | null;

export type MessageRulesFeedback =
  | Readonly<{ kind: "saved-item" }>
  | Readonly<{ kind: "saved-order"; collection: string }>
  | Readonly<{ kind: "conflict" }>
  | Readonly<{ kind: "error"; error: ApiRequestError }>;

export type MessageRulesController = Readonly<{
  state: MessageRulesState;
  busy: MessageRulesBusy;
  feedback: MessageRulesFeedback | null;
  retry: () => void;
  toggle: (rule: MessageRule) => void;
  move: (rule: MessageRule, direction: "up" | "down") => void;
}>;

const accessRevocationCodes: Readonly<Record<string, true>> = {
  authentication_expired: true,
  authentication_invalid: true,
  chat_access_denied: true,
  chat_access_unavailable: true,
  chat_not_found: true
};

function replaceCollection(
  items: readonly MessageRule[],
  collection: string,
  replacement: readonly MessageRule[]
): readonly MessageRule[] {
  const next: MessageRule[] = [];
  let inserted = false;
  for (const item of items) {
    if (item.collection === collection) {
      if (!inserted) {
        next.push(...replacement);
        inserted = true;
      }
      continue;
    }
    next.push(item);
  }
  return next;
}

export function useMessageRules(chatID: string | undefined, active: boolean): MessageRulesController {
  const [state, setState] = useState<MessageRulesState>({ kind: "loading" });
  const [busy, setBusy] = useState<MessageRulesBusy>(null);
  const [feedback, setFeedback] = useState<MessageRulesFeedback | null>(null);
  const [reloadVersion, setReloadVersion] = useState(0);
  const activeScopeRef = useRef("");
  const operationSequenceRef = useRef(0);

  useEffect(() => {
    const scope = `${active ? "active" : "inactive"}:${chatID ?? ""}:${reloadVersion}`;
    activeScopeRef.current = scope;
    operationSequenceRef.current += 1;
    setState({ kind: "loading" });
    setBusy(null);
    setFeedback(null);

    if (!active || !chatID) {
      return undefined;
    }

    let mounted = true;
    void loadMessageRules(consoleApi, chatID).then((result) => {
      if (!mounted || activeScopeRef.current !== scope) {
        return;
      }
      if (!result.ok) {
        setState({ kind: "unavailable", error: result.error });
        return;
      }
      setState({ kind: "loaded", items: result.data });
    });

    return () => {
      mounted = false;
    };
  }, [active, chatID, reloadVersion]);

  async function toggle(rule: MessageRule): Promise<void> {
    if (state.kind !== "loaded" || !chatID || busy) {
      return;
    }
    const current = state.items.find((item) => item.id === rule.id);
    if (!current) {
      return;
    }

    const scope = activeScopeRef.current;
    const sequence = operationSequenceRef.current + 1;
    operationSequenceRef.current = sequence;
    setBusy({ kind: "item", id: current.id });
    setFeedback(null);
    const result = await updateMessageRule(consoleApi, chatID, current, {
      ...current,
      enabled: !current.enabled
    });

    if (activeScopeRef.current !== scope || operationSequenceRef.current !== sequence) {
      return;
    }
    if (result.ok) {
      setState({
        kind: "loaded",
        items: state.items.map((item) => (item.id === result.data.id ? result.data : item))
      });
      setFeedback({ kind: "saved-item" });
      setBusy(null);
      return;
    }
    if (result.error.kind === "api" && result.error.code === "rule_conflict") {
      const currentRules = await loadMessageRules(consoleApi, chatID);
      if (activeScopeRef.current !== scope || operationSequenceRef.current !== sequence) {
        return;
      }
      if (currentRules.ok) {
        setState({ kind: "loaded", items: currentRules.data });
        setFeedback({ kind: "conflict" });
        setBusy(null);
        return;
      }
      setState({ kind: "unavailable", error: currentRules.error });
      setBusy(null);
      return;
    }
    if (result.error.kind === "api" && accessRevocationCodes[result.error.code]) {
      setState({ kind: "unavailable", error: result.error });
      setBusy(null);
      return;
    }
    setFeedback({ kind: "error", error: result.error });
    setBusy(null);
  }

  async function move(rule: MessageRule, direction: "up" | "down"): Promise<void> {
    if (state.kind !== "loaded" || !chatID || busy) {
      return;
    }
    const expected = state.items.filter((item) => item.collection === rule.collection);
    const index = expected.findIndex((item) => item.id === rule.id);
    const targetIndex = direction === "up" ? index - 1 : index + 1;
    if (index < 0 || targetIndex < 0 || targetIndex >= expected.length) {
      return;
    }

    const next = [...expected];
    const current = next[index];
    const target = next[targetIndex];
    if (!current || !target) {
      return;
    }
    next[index] = target;
    next[targetIndex] = current;
    const scope = activeScopeRef.current;
    const sequence = operationSequenceRef.current + 1;
    operationSequenceRef.current = sequence;
    setBusy({ kind: "collection", collection: rule.collection });
    setFeedback(null);
    const result = await replaceMessageRuleCollection(
      consoleApi,
      chatID,
      rule.collection,
      expected,
      next
    );

    if (activeScopeRef.current !== scope || operationSequenceRef.current !== sequence) {
      return;
    }
    if (result.ok) {
      setState({
        kind: "loaded",
        items: replaceCollection(state.items, rule.collection, result.data)
      });
      setFeedback({ kind: "saved-order", collection: rule.collection });
      setBusy(null);
      return;
    }
    if (result.error.kind === "api" && result.error.code === "rule_conflict") {
      const currentRules = await loadMessageRules(consoleApi, chatID);
      if (activeScopeRef.current !== scope || operationSequenceRef.current !== sequence) {
        return;
      }
      if (currentRules.ok) {
        setState({ kind: "loaded", items: currentRules.data });
        setFeedback({ kind: "conflict" });
        setBusy(null);
        return;
      }
      setState({ kind: "unavailable", error: currentRules.error });
      setBusy(null);
      return;
    }
    if (result.error.kind === "api" && accessRevocationCodes[result.error.code]) {
      setState({ kind: "unavailable", error: result.error });
      setBusy(null);
      return;
    }
    setFeedback({ kind: "error", error: result.error });
    setBusy(null);
  }

  return {
    state,
    busy,
    feedback,
    retry: () => setReloadVersion((version) => version + 1),
    toggle,
    move
  };
}
