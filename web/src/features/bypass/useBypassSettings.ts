import {
  useCallback,
  useEffect,
  useReducer,
  useRef,
  useState,
  type Dispatch,
  type MutableRefObject
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
  loadBypassSettings,
  saveBypassSettings,
  type BypassSettings
} from "./api";
import {
  evaluateBypassForm,
  formFromSettings,
  sourceForField,
  type BypassEvaluation,
  type BypassField,
  type BypassForm,
  type BypassTextField,
  type RestoringFields
} from "./model";

export type BypassFeedback =
  | Readonly<{ kind: "saved" }>
  | Readonly<{ kind: "conflict" }>
  | Readonly<{ kind: "error"; error: ApiRequestError }>;

export type BypassReadyState = Readonly<{
  kind: "ready";
  settings: BypassSettings;
  form: BypassForm;
  restoring: RestoringFields;
  saving: boolean;
  feedback: BypassFeedback | null;
}>;

export type BypassScreenState =
  | Readonly<{ kind: "loading" }>
  | BypassReadyState
  | Readonly<{ kind: "unavailable"; error: ApiRequestError }>
  | Readonly<{ kind: "group-required" }>
  | Readonly<{ kind: "no-groups" }>;

type EditAction =
  | Readonly<{ type: "edit-text"; field: BypassTextField; value: string }>
  | Readonly<{ type: "edit-fail-open"; value: boolean }>;

type BypassAction =
  | Readonly<{ type: "replace"; state: BypassScreenState }>
  | EditAction
  | Readonly<{ type: "restore"; field: BypassField; enabled: boolean }>
  | Readonly<{ type: "discard" }>
  | Readonly<{ type: "save-started" }>
  | Readonly<{ type: "save-succeeded"; settings: BypassSettings }>
  | Readonly<{ type: "save-conflicted"; settings: BypassSettings }>
  | Readonly<{ type: "save-failed"; feedback: BypassFeedback }>;

export type BypassController = Readonly<{
  state: BypassScreenState;
  evaluation: BypassEvaluation | undefined;
  editText: (field: BypassTextField, value: string) => void;
  editFailOpen: (value: boolean) => void;
  setRestoring: (field: BypassField, enabled: boolean) => void;
  discard: () => void;
  save: () => void;
  reload: () => void;
}>;

function readyState(settings: BypassSettings): BypassReadyState {
  return {
    kind: "ready",
    settings,
    form: formFromSettings(settings),
    restoring: {},
    saving: false,
    feedback: null
  };
}

function editReadyState(state: BypassReadyState, action: EditAction): BypassReadyState {
  const field = action.type === "edit-text" ? action.field : "requiredChannelFailOpen";
  if (state.saving || sourceForField(state.settings, field) === "user file") {
    return state;
  }

  const restoring = { ...state.restoring };
  delete restoring[field];
  const form =
    action.type === "edit-text"
      ? ({ ...state.form, [action.field]: action.value } as BypassForm)
      : { ...state.form, requiredChannelFailOpen: action.value };
  return { ...state, form, restoring, feedback: null };
}

function bypassReducer(state: BypassScreenState, action: BypassAction): BypassScreenState {
  if (action.type === "replace") {
    return action.state;
  }
  if (state.kind !== "ready") {
    return state;
  }

  switch (action.type) {
    case "edit-text":
    case "edit-fail-open":
      return editReadyState(state, action);
    case "restore": {
      if (state.saving || sourceForField(state.settings, action.field) !== "chat override") {
        return state;
      }
      const restoring = { ...state.restoring };
      if (action.enabled) {
        restoring[action.field] = true;
      } else {
        delete restoring[action.field];
      }
      return { ...state, restoring, feedback: null };
    }
    case "discard":
      return readyState(state.settings);
    case "save-started":
      return { ...state, saving: true, feedback: null };
    case "save-succeeded":
      return { ...readyState(action.settings), feedback: { kind: "saved" } };
    case "save-conflicted":
      return { ...readyState(action.settings), feedback: { kind: "conflict" } };
    case "save-failed":
      return { ...state, saving: false, feedback: action.feedback };
  }
}

function stateBeforeLiveLoad(
  session: ConsoleSessionState,
  chatID: string | undefined
): BypassScreenState | undefined {
  if (session.state === "loading" || session.state === "checking-groups") {
    return { kind: "loading" };
  }
  if (session.state === "blocked" || session.state === "groups-unavailable") {
    return { kind: "unavailable", error: session.error };
  }
  if (session.state === "no-groups") {
    return { kind: "no-groups" };
  }
  return chatID ? undefined : { kind: "group-required" };
}

function useBypassLoad(
  session: ConsoleSessionState,
  chatID: string | undefined,
  reloadVersion: number,
  dispatch: Dispatch<BypassAction>,
  activeScopeRef: MutableRefObject<string>,
  saveInFlightRef: MutableRefObject<boolean>
): void {
  useEffect(() => {
    const scope = `${session.state}:${chatID ?? ""}:${reloadVersion}`;
    activeScopeRef.current = scope;
    saveInFlightRef.current = false;
    const immediateState = stateBeforeLiveLoad(session, chatID);
    if (immediateState) {
      dispatch({ type: "replace", state: immediateState });
      return;
    }

    let active = true;
    dispatch({ type: "replace", state: { kind: "loading" } });
    void loadBypassSettings(consoleApi, chatID!).then((result) => {
      if (!active || activeScopeRef.current !== scope) {
        return;
      }
      dispatch({
        type: "replace",
        state: result.ok ? readyState(result.data) : { kind: "unavailable", error: result.error }
      });
    });
    return () => {
      active = false;
    };
  }, [activeScopeRef, chatID, dispatch, reloadVersion, saveInFlightRef, session]);
}

async function submitBypassSettings(
  state: BypassReadyState,
  evaluation: BypassEvaluation,
  chatID: string | undefined,
  dispatch: Dispatch<BypassAction>,
  activeScopeRef: MutableRefObject<string>,
  saveInFlightRef: MutableRefObject<boolean>
): Promise<void> {
  if (!evaluation.valid || evaluation.count === 0 || saveInFlightRef.current || !chatID) {
    return;
  }

  const scope = activeScopeRef.current;
  saveInFlightRef.current = true;
  dispatch({ type: "save-started" });

  try {
    const result = await saveBypassSettings(
      consoleApi,
      chatID,
      state.settings.revision,
      evaluation.changes
    );
    if (activeScopeRef.current !== scope) {
      return;
    }
    if (result.ok) {
      dispatch({ type: "save-succeeded", settings: result.data });
      return;
    }
    if (result.error.kind === "api" && result.error.code === "settings_conflict") {
      const currentSettings = await loadBypassSettings(consoleApi, chatID);
      if (activeScopeRef.current !== scope) {
        return;
      }
      if (currentSettings.ok) {
        dispatch({ type: "save-conflicted", settings: currentSettings.data });
      } else {
        dispatch({ type: "replace", state: { kind: "unavailable", error: currentSettings.error } });
      }
      return;
    }
    dispatch({ type: "save-failed", feedback: { kind: "error", error: result.error } });
  } finally {
    if (activeScopeRef.current === scope) {
      saveInFlightRef.current = false;
    }
  }
}

export function useBypassSettings(): BypassController {
  const session = useConsoleSession();
  const [searchParams] = useSearchParams();
  const selectedGroupID = searchParams.get("group");
  const chatID =
    selectedGroupID !== null && /^-?\d+$/.test(selectedGroupID) ? selectedGroupID : undefined;
  const [state, dispatch] = useReducer(bypassReducer, { kind: "loading" });
  const [reloadVersion, setReloadVersion] = useState(0);
  const activeScopeRef = useRef("");
  const saveInFlightRef = useRef(false);
  useBypassLoad(
    session,
    chatID,
    reloadVersion,
    dispatch,
    activeScopeRef,
    saveInFlightRef
  );

  const evaluation =
    state.kind === "ready"
      ? evaluateBypassForm(state.settings, state.form, state.restoring)
      : undefined;
  const save = useCallback(() => {
    if (state.kind === "ready" && evaluation) {
      void submitBypassSettings(
        state,
        evaluation,
        chatID,
        dispatch,
        activeScopeRef,
        saveInFlightRef
      );
    }
  }, [chatID, evaluation, state]);

  return {
    state,
    evaluation,
    editText: (field, value) => dispatch({ type: "edit-text", field, value }),
    editFailOpen: (value) => dispatch({ type: "edit-fail-open", value }),
    setRestoring: (field, enabled) => dispatch({ type: "restore", field, enabled }),
    discard: () => dispatch({ type: "discard" }),
    save,
    reload: () => {
      if (!retryConsoleAccess(session)) {
        setReloadVersion((currentVersion) => currentVersion + 1);
      }
    }
  };
}
