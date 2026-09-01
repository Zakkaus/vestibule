import {
  useEffect,
  useMemo,
  useRef,
  useState,
  type FormEvent
} from "react";

import { consoleApi } from "../../app/session";
import type { ApiRequestError } from "../../lib/api";
import {
  loadMessageSettings,
  saveMessageSettings,
  type MessageSettingField,
  type MessageSettings
} from "./api";
import {
  evaluateMessageSettings,
  messageSettingsDraft,
  type MessageSettingsDraft,
  type MessageSettingsEvaluation
} from "./settings";

export type MessageSettingsState =
  | Readonly<{ kind: "loading" }>
  | Readonly<{ kind: "loaded"; settings: MessageSettings }>
  | Readonly<{ kind: "unavailable"; error: ApiRequestError }>;

export type MessageSettingsFeedback =
  | Readonly<{ kind: "saved" }>
  | Readonly<{ kind: "conflict" }>
  | Readonly<{ kind: "error"; error: ApiRequestError }>;

export type MessageSettingsController = Readonly<{
  state: MessageSettingsState;
  draft: MessageSettingsDraft | null;
  evaluation: MessageSettingsEvaluation | null;
  restoring: ReadonlySet<MessageSettingField>;
  saving: boolean;
  feedback: MessageSettingsFeedback | null;
  retry: () => void;
  submit: (event: FormEvent<HTMLFormElement>) => void;
  setDraft: <K extends keyof MessageSettingsDraft>(field: K, value: MessageSettingsDraft[K]) => void;
  setRestoring: (field: MessageSettingField, restoring: boolean) => void;
}>;

const accessRevocationCodes: Readonly<Record<string, true>> = {
  authentication_expired: true,
  authentication_invalid: true,
  chat_access_denied: true,
  chat_access_unavailable: true,
  chat_not_found: true
};

export function useMessageSettings(
  chatID: string | undefined,
  active: boolean
): MessageSettingsController {
  const [state, setState] = useState<MessageSettingsState>({ kind: "loading" });
  const [draft, setDraftState] = useState<MessageSettingsDraft | null>(null);
  const [restoring, setRestoringState] = useState<ReadonlySet<MessageSettingField>>(new Set());
  const [saving, setSaving] = useState(false);
  const [feedback, setFeedback] = useState<MessageSettingsFeedback | null>(null);
  const [reloadVersion, setReloadVersion] = useState(0);
  const activeScopeRef = useRef("");
  const saveSequenceRef = useRef(0);

  useEffect(() => {
    const scope = `${active ? "active" : "inactive"}:${chatID ?? ""}:${reloadVersion}`;
    activeScopeRef.current = scope;
    saveSequenceRef.current += 1;
    setState({ kind: "loading" });
    setDraftState(null);
    setRestoringState(new Set());
    setSaving(false);
    setFeedback(null);

    if (!active || !chatID) {
      return undefined;
    }

    let mounted = true;
    void loadMessageSettings(consoleApi, chatID).then((result) => {
      if (!mounted || activeScopeRef.current !== scope) {
        return;
      }
      if (!result.ok) {
        setState({ kind: "unavailable", error: result.error });
        return;
      }
      setState({ kind: "loaded", settings: result.data });
      setDraftState(messageSettingsDraft(result.data));
    });

    return () => {
      mounted = false;
    };
  }, [active, chatID, reloadVersion]);

  const evaluation = useMemo(() => {
    if (state.kind !== "loaded" || !draft) {
      return null;
    }
    return evaluateMessageSettings(state.settings, draft, restoring);
  }, [draft, restoring, state]);

  function setDraft<K extends keyof MessageSettingsDraft>(
    field: K,
    value: MessageSettingsDraft[K]
  ): void {
    setDraftState((current) => (current ? { ...current, [field]: value } : current));
    setRestoringState((current) => {
      if (!current.has(field)) {
        return current;
      }
      const next = new Set(current);
      next.delete(field);
      return next;
    });
    setFeedback(null);
  }

  function setRestoring(field: MessageSettingField, nextRestoring: boolean): void {
    if (state.kind !== "loaded" || state.settings[field].source !== "chat override" || saving) {
      return;
    }

    setRestoringState((current) => {
      const next = new Set(current);
      if (nextRestoring) {
        next.add(field);
      } else {
        next.delete(field);
      }
      return next;
    });
    setFeedback(null);
  }

  async function save(): Promise<void> {
    if (state.kind !== "loaded" || !chatID || !draft || !evaluation || saving) {
      return;
    }
    if (!evaluation.values || evaluation.changedFields.size === 0) {
      return;
    }

    const scope = activeScopeRef.current;
    const sequence = saveSequenceRef.current + 1;
    saveSequenceRef.current = sequence;
    setSaving(true);
    setFeedback(null);
    const result = await saveMessageSettings(
      consoleApi,
      chatID,
      state.settings.revision,
      evaluation.changes
    );

    if (activeScopeRef.current !== scope || saveSequenceRef.current !== sequence) {
      return;
    }
    if (result.ok) {
      setState({ kind: "loaded", settings: result.data });
      setDraftState(messageSettingsDraft(result.data));
      setRestoringState(new Set());
      setFeedback({ kind: "saved" });
      setSaving(false);
      return;
    }
    if (result.error.kind === "api" && result.error.code === "settings_conflict") {
      const current = await loadMessageSettings(consoleApi, chatID);
      if (activeScopeRef.current !== scope || saveSequenceRef.current !== sequence) {
        return;
      }
      if (current.ok) {
        setState({ kind: "loaded", settings: current.data });
        setDraftState(messageSettingsDraft(current.data));
        setRestoringState(new Set());
        setFeedback({ kind: "conflict" });
        setSaving(false);
        return;
      }
      setState({ kind: "unavailable", error: current.error });
      setSaving(false);
      return;
    }
    if (result.error.kind === "api" && accessRevocationCodes[result.error.code]) {
      setState({ kind: "unavailable", error: result.error });
      setSaving(false);
      return;
    }
    setFeedback({ kind: "error", error: result.error });
    setSaving(false);
  }

  function submit(event: FormEvent<HTMLFormElement>): void {
    event.preventDefault();
    void save();
  }

  return {
    state,
    draft,
    evaluation,
    restoring,
    saving,
    feedback,
    retry: () => setReloadVersion((version) => version + 1),
    submit,
    setDraft,
    setRestoring
  };
}
