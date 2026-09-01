import { useCallback, useEffect, useReducer, useRef, useState } from "react";
import { useSearchParams } from "react-router-dom";

import {
  consoleApi,
  useConsoleSession,
  type ConsoleSessionState
} from "../../app/session";
import type { ApiRequestError } from "../../lib/api";
import {
  loadModerationSettings,
  saveModerationSettings,
  type ModerationSettings
} from "./api";
import {
  applyModerationFixtureChanges,
  moderationFixtureSettings
} from "./fixtures";
import {
  evaluateModerationForm,
  formFromSettings,
  sourceForField,
  type ModerationEvaluation,
  type ModerationField,
  type ModerationForm,
  type RestoringFields
} from "./model";

const FIXTURE_SAVE_DELAY_MS = 500;

type ReadyOrigin = "live" | "fixture";

export type ModerationFeedback =
  | Readonly<{ kind: "saved" }>
  | Readonly<{ kind: "conflict" }>
  | Readonly<{ kind: "error"; error: ApiRequestError }>;

export type ModerationReadyState = Readonly<{
  kind: "ready";
  origin: ReadyOrigin;
  settings: ModerationSettings;
  form: ModerationForm;
  restoring: RestoringFields;
  saving: boolean;
  feedback: ModerationFeedback | null;
}>;

export type ModerationScreenState =
  | Readonly<{ kind: "loading" }>
  | ModerationReadyState
  | Readonly<{ kind: "unavailable"; error: ApiRequestError }>
  | Readonly<{ kind: "group-required" }>
  | Readonly<{ kind: "no-groups" }>;

type EditAction =
  | Readonly<{ type: "edit"; field: "warnLimit"; value: string }>
  | Readonly<{ type: "edit"; field: "antispamEnabled"; value: boolean }>
  | Readonly<{ type: "edit"; field: "adminLogChatID"; value: string }>;

type ModerationAction =
  | Readonly<{ type: "replace"; state: ModerationScreenState }>
  | EditAction
  | Readonly<{ type: "restore"; field: ModerationField; enabled: boolean }>
  | Readonly<{ type: "discard" }>
  | Readonly<{ type: "save-started" }>
  | Readonly<{ type: "save-succeeded"; settings: ModerationSettings }>
  | Readonly<{ type: "save-failed"; feedback: ModerationFeedback }>;

export type ModerationController = Readonly<{
  state: ModerationScreenState;
  evaluation: ModerationEvaluation | undefined;
  editWarnLimit: (value: string) => void;
  editAntispamEnabled: (value: boolean) => void;
  editAdminLogChatID: (value: string) => void;
  setRestoring: (field: ModerationField, enabled: boolean) => void;
  discard: () => void;
  save: () => void;
  reload: () => void;
}>;

function readyState(origin: ReadyOrigin, settings: ModerationSettings): ModerationReadyState {
  return {
    kind: "ready",
    origin,
    settings,
    form: formFromSettings(settings),
    restoring: {},
    saving: false,
    feedback: null
  };
}

function editReadyState(state: ModerationReadyState, action: EditAction): ModerationReadyState {
  if (state.saving || sourceForField(state.settings, action.field) === "user file") {
    return state;
  }
  const restoring = { ...state.restoring };
  delete restoring[action.field];
  const form = { ...state.form, [action.field]: action.value } as ModerationForm;
  return { ...state, form, restoring, feedback: null };
}

function moderationReducer(
  state: ModerationScreenState,
  action: ModerationAction
): ModerationScreenState {
  if (action.type === "replace") {
    return action.state;
  }
  if (state.kind !== "ready") {
    return state;
  }

  switch (action.type) {
    case "edit":
      return editReadyState(state, action);
    case "restore": {
      if (
        state.saving ||
        sourceForField(state.settings, action.field) !== "chat override"
      ) {
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
      return { ...readyState(state.origin, state.settings), feedback: null };
    case "save-started":
      return { ...state, saving: true, feedback: null };
    case "save-succeeded":
      return { ...readyState(state.origin, action.settings), feedback: { kind: "saved" } };
    case "save-failed":
      return { ...state, saving: false, feedback: action.feedback };
  }
}

function stateBeforeLiveLoad(
  session: ConsoleSessionState,
  chatID: string | undefined
): ModerationScreenState | undefined {
  if (session.state === "loading" || session.state === "checking-groups") {
    return { kind: "loading" };
  }
  if (session.state === "blocked" && session.error.kind === "non-json") {
    return readyState("fixture", moderationFixtureSettings);
  }
  if (session.state === "blocked" || session.state === "groups-unavailable") {
    return { kind: "unavailable", error: session.error };
  }
  if (session.state === "no-groups") {
    return { kind: "no-groups" };
  }
  return chatID ? undefined : { kind: "group-required" };
}

function useModerationLoad(
  session: ConsoleSessionState,
  chatID: string | undefined,
  reloadVersion: number,
  dispatch: React.Dispatch<ModerationAction>,
  activeScopeRef: React.MutableRefObject<string>,
  saveInFlightRef: React.MutableRefObject<boolean>
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
    void loadModerationSettings(consoleApi, chatID!).then((result) => {
      if (!active || activeScopeRef.current !== scope) {
        return;
      }
      dispatch({
        type: "replace",
        state: result.ok
          ? readyState("live", result.data)
          : { kind: "unavailable", error: result.error }
      });
    });
    return () => {
      active = false;
    };
  }, [activeScopeRef, chatID, dispatch, reloadVersion, saveInFlightRef, session]);
}

async function fixtureSave(
  state: ModerationReadyState,
  evaluation: ModerationEvaluation
): Promise<ModerationSettings> {
  await new Promise<void>((resolve) => window.setTimeout(resolve, FIXTURE_SAVE_DELAY_MS));
  return applyModerationFixtureChanges(state.settings, evaluation.changes);
}

async function submitModerationSettings(
  state: ModerationReadyState,
  evaluation: ModerationEvaluation,
  chatID: string | undefined,
  dispatch: React.Dispatch<ModerationAction>,
  activeScopeRef: React.MutableRefObject<string>,
  saveInFlightRef: React.MutableRefObject<boolean>
): Promise<void> {
  if (!evaluation.valid || evaluation.count === 0 || saveInFlightRef.current) {
    return;
  }
  const scope = activeScopeRef.current;
  saveInFlightRef.current = true;
  dispatch({ type: "save-started" });

  try {
    if (state.origin === "fixture") {
      const settings = await fixtureSave(state, evaluation);
      if (activeScopeRef.current === scope) {
        dispatch({ type: "save-succeeded", settings });
      }
      return;
    }
    if (!chatID) {
      return;
    }
    const result = await saveModerationSettings(
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
    const feedback: ModerationFeedback =
      result.error.kind === "api" && result.error.code === "settings_conflict"
        ? { kind: "conflict" }
        : { kind: "error", error: result.error };
    dispatch({ type: "save-failed", feedback });
  } finally {
    if (activeScopeRef.current === scope) {
      saveInFlightRef.current = false;
    }
  }
}

export function useModerationSettings(): ModerationController {
  const session = useConsoleSession();
  const [searchParams] = useSearchParams();
  const selectedGroupID = searchParams.get("group");
  const chatID =
    selectedGroupID !== null && /^-?\d+$/.test(selectedGroupID)
      ? selectedGroupID
      : undefined;
  const [state, dispatch] = useReducer(moderationReducer, { kind: "loading" });
  const [reloadVersion, setReloadVersion] = useState(0);
  const activeScopeRef = useRef("");
  const saveInFlightRef = useRef(false);
  useModerationLoad(
    session,
    chatID,
    reloadVersion,
    dispatch,
    activeScopeRef,
    saveInFlightRef
  );

  const evaluation =
    state.kind === "ready"
      ? evaluateModerationForm(state.settings, state.form, state.restoring)
      : undefined;
  const save = useCallback(() => {
    if (state.kind === "ready" && evaluation) {
      void submitModerationSettings(
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
    editWarnLimit: (value) => dispatch({ type: "edit", field: "warnLimit", value }),
    editAntispamEnabled: (value) =>
      dispatch({ type: "edit", field: "antispamEnabled", value }),
    editAdminLogChatID: (value) =>
      dispatch({ type: "edit", field: "adminLogChatID", value }),
    setRestoring: (field, enabled) => dispatch({ type: "restore", field, enabled }),
    discard: () => dispatch({ type: "discard" }),
    save,
    reload: () => setReloadVersion((currentVersion) => currentVersion + 1)
  };
}
