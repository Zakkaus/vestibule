import {
  useCallback,
  useEffect,
  useMemo,
  useRef,
  useState,
  type Dispatch,
  type MutableRefObject,
  type SetStateAction
} from "react";
import { useSearchParams } from "react-router-dom";

import {
  consoleApi,
  retryConsoleAccess,
  useConsoleSession,
  type ConsoleSessionState
} from "../../app/session";
import type { ApiRequestError } from "../../lib/api";
import {
  loadQuestionSettings,
  saveQuestionSettings,
  type QuestionSettingField,
  type QuestionSettings
} from "./api";
import {
  draftFromSettings,
  hasQuestionDraftChanges,
  sparseQuestionChanges,
  validateQuestionsDraft,
  type QuestionsDraft,
  type QuestionsValidation
} from "./model";

export type QuestionsScreenState =
  | Readonly<{ kind: "loading" }>
  | Readonly<{ kind: "loaded"; settings: QuestionSettings }>
  | Readonly<{ kind: "unavailable"; error: ApiRequestError }>
  | Readonly<{ kind: "group-required" }>
  | Readonly<{ kind: "no-groups" }>;

export type QuestionsFeedback =
  | Readonly<{ kind: "saved" }>
  | Readonly<{ kind: "conflict" }>
  | Readonly<{ kind: "error"; error: ApiRequestError }>;

const accessRevocationCodes: Readonly<Record<string, true>> = {
  authentication_expired: true,
  authentication_invalid: true,
  chat_access_denied: true,
  chat_not_found: true
};

export type QuestionsController = Readonly<{
  state: QuestionsScreenState;
  draft: QuestionsDraft | null;
  validation: QuestionsValidation;
  restored: ReadonlySet<QuestionSettingField>;
  saving: boolean;
  hasChanges: boolean;
  updateDraft: (draft: QuestionsDraft, fields: readonly QuestionSettingField[]) => void;
  restore: (field: QuestionSettingField) => void;
  restoreFallback: () => void;
  save: () => Promise<void>;
  reload: () => void;
  feedback: QuestionsFeedback | null;
}>;

type LoadSetters = Readonly<{
  setState: Dispatch<SetStateAction<QuestionsScreenState>>;
  setDraft: Dispatch<SetStateAction<QuestionsDraft | null>>;
  setRestored: Dispatch<SetStateAction<ReadonlySet<QuestionSettingField>>>;
  setAttemptedSave: Dispatch<SetStateAction<boolean>>;
  setSaving: Dispatch<SetStateAction<boolean>>;
  setFeedback: Dispatch<SetStateAction<QuestionsFeedback | null>>;
}>;

type SaveContext = Readonly<{
  state: Readonly<{ kind: "loaded"; settings: QuestionSettings }>;
  draft: QuestionsDraft;
  validation: QuestionsValidation;
  restored: ReadonlySet<QuestionSettingField>;
  chatID: string;
  activeScopeRef: MutableRefObject<string>;
  saveSequenceRef: MutableRefObject<number>;
  setters: LoadSetters;
}>;

function stateBeforeLoad(
  session: ConsoleSessionState,
  chatID: string | undefined
): QuestionsScreenState | undefined {
  if (session.state === "loading" || session.state === "checking-groups") {
    return { kind: "loading" };
  }
  if (session.state === "blocked" || session.state === "groups-unavailable") {
    return { kind: "unavailable", error: session.error };
  }
  if (session.state === "no-groups") {
    return { kind: "no-groups" };
  }
  if (!chatID) {
    return { kind: "group-required" };
  }
  return undefined;
}

function useQuestionSettingsLoad(
  session: ConsoleSessionState,
  chatID: string | undefined,
  reloadVersion: number,
  activeScopeRef: MutableRefObject<string>,
  saveSequenceRef: MutableRefObject<number>,
  setters: LoadSetters
): void {
  useEffect(() => {
    const scope = `${session.state}:${chatID ?? ""}:${reloadVersion}`;
    activeScopeRef.current = scope;
    saveSequenceRef.current += 1;
    setters.setSaving(false);
    setters.setDraft(null);
    setters.setRestored(new Set());
    setters.setAttemptedSave(false);
    setters.setFeedback(null);
    let active = true;
    const immediateState = stateBeforeLoad(session, chatID);

    if (immediateState) {
      setters.setState(immediateState);
      return () => {
        active = false;
      };
    }

    setters.setState({ kind: "loading" });
    void loadQuestionSettings(consoleApi, chatID!).then((result) => {
      if (!active || activeScopeRef.current !== scope) {
        return;
      }
      if (!result.ok) {
        setters.setState({ kind: "unavailable", error: result.error });
        return;
      }
      setters.setState({ kind: "loaded", settings: result.data });
      setters.setDraft(draftFromSettings(result.data));
    });

    return () => {
      active = false;
    };
  }, [activeScopeRef, chatID, reloadVersion, saveSequenceRef, session, setters]);
}

async function submitQuestionSettings(context: SaveContext): Promise<void> {
  const { state, draft, validation, restored, chatID, activeScopeRef, saveSequenceRef, setters } = context;
  if (!validation.values) {
    return;
  }

  const scope = activeScopeRef.current;
  const sequence = saveSequenceRef.current + 1;
  saveSequenceRef.current = sequence;
  setters.setSaving(true);
  setters.setFeedback(null);
  const changes = sparseQuestionChanges(state.settings, draft, restored, validation.values);
  const result = await saveQuestionSettings(consoleApi, chatID, state.settings.revision, changes);
  if (activeScopeRef.current !== scope || saveSequenceRef.current !== sequence) {
    return;
  }

  if (result.ok) {
    setters.setState({ kind: "loaded", settings: result.data });
    setters.setDraft(draftFromSettings(result.data));
    setters.setRestored(new Set());
    setters.setAttemptedSave(false);
    setters.setFeedback({ kind: "saved" });
    setters.setSaving(false);
    return;
  }

  if (result.error.kind === "api" && result.error.code === "settings_conflict") {
    const latest = await loadQuestionSettings(consoleApi, chatID);
    if (activeScopeRef.current !== scope || saveSequenceRef.current !== sequence) {
      return;
    }
    if (latest.ok) {
      setters.setState({ kind: "loaded", settings: latest.data });
      setters.setDraft(draftFromSettings(latest.data));
      setters.setRestored(new Set());
      setters.setAttemptedSave(false);
      setters.setFeedback({ kind: "conflict" });
    } else {
      setters.setState({ kind: "unavailable", error: latest.error });
    }
    setters.setSaving(false);
    return;
  }

  if (result.error.kind === "api" && accessRevocationCodes[result.error.code]) {
    setters.setState({ kind: "unavailable", error: result.error });
    setters.setSaving(false);
    return;
  }

  setters.setFeedback({ kind: "error", error: result.error });
  setters.setSaving(false);
}

export function useQuestionSettings(): QuestionsController {
  const session = useConsoleSession();
  const [searchParams] = useSearchParams();
  const selectedGroupID = searchParams.get("group");
  const chatID =
    selectedGroupID !== null && /^-?\d+$/.test(selectedGroupID) ? selectedGroupID : undefined;
  const [state, setState] = useState<QuestionsScreenState>({ kind: "loading" });
  const [draft, setDraft] = useState<QuestionsDraft | null>(null);
  const [restored, setRestored] = useState<ReadonlySet<QuestionSettingField>>(new Set());
  const [attemptedSave, setAttemptedSave] = useState(false);
  const [saving, setSaving] = useState(false);
  const [feedback, setFeedback] = useState<QuestionsFeedback | null>(null);
  const [reloadVersion, setReloadVersion] = useState(0);
  const activeScopeRef = useRef("");
  const saveSequenceRef = useRef(0);
  const setters = useMemo<LoadSetters>(
    () => ({ setState, setDraft, setRestored, setAttemptedSave, setSaving, setFeedback }),
    []
  );

  useQuestionSettingsLoad(
    session,
    chatID,
    reloadVersion,
    activeScopeRef,
    saveSequenceRef,
    setters
  );

  const validation = draft
    ? validateQuestionsDraft(draft)
    : { questionErrors: {}, fallbackQuestionErrors: {} };
  const visibleValidation = attemptedSave
    ? validation
    : { ...validation, questionErrors: {}, fallbackQuestionErrors: {}, fallbackListError: undefined };
  const hasChanges =
    state.kind === "loaded" && draft
      ? hasQuestionDraftChanges(state.settings, draft, restored)
      : false;

  const updateDraft = useCallback((next: QuestionsDraft, fields: readonly QuestionSettingField[]) => {
    setDraft(next);
    setRestored((current) => {
      const updated = new Set(current);
      fields.forEach((field) => updated.delete(field));
      return updated;
    });
    setFeedback(null);
  }, []);

  const restore = useCallback((field: QuestionSettingField) => {
    if (state.kind !== "loaded" || state.settings[field].source !== "chat override") {
      return;
    }
    setRestored((current) => new Set(current).add(field));
    setFeedback(null);
  }, [state]);

  const restoreFallback = useCallback(() => {
    if (state.kind !== "loaded") {
      return;
    }
    setRestored((current) => {
      const updated = new Set(current);
      if (state.settings.fallback_builtin.source === "chat override") {
        updated.add("fallback_builtin");
      }
      if (state.settings.fallback_questions.source === "chat override") {
        updated.add("fallback_questions");
      }
      return updated;
    });
    setFeedback(null);
  }, [state]);

  const save = useCallback(async () => {
    setAttemptedSave(true);
    if (state.kind !== "loaded" || !draft || !chatID || saving || !hasChanges) {
      return;
    }
    await submitQuestionSettings({
      state,
      draft,
      validation,
      restored,
      chatID,
      activeScopeRef,
      saveSequenceRef,
      setters
    });
  }, [chatID, draft, hasChanges, restored, saving, setters, state, validation]);

  return {
    state,
    draft,
    validation: visibleValidation,
    restored,
    saving,
    hasChanges,
    updateDraft,
    restore,
    restoreFallback,
    save,
    reload: () => {
      if (!retryConsoleAccess(session)) {
        setReloadVersion((version) => version + 1);
      }
    },
    feedback
  };
}
