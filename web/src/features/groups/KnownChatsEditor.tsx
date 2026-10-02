import { useEffect, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { Button } from "@react-spectrum/s2/Button";
import { TextArea } from "@react-spectrum/s2/TextArea";
import { consoleApi } from "../../app/session";
import { SettingsLimitNotice } from "../../components/SettingsLimitNotice";
import type { ApiRequestError } from "../../lib/api";
import { loadKnownChats, parseKnownChats, saveKnownChats, type KnownChats } from "./knownChats";
import "./knownChats.css";

const sources: Readonly<Record<KnownChats["source"], string>> = {
  "factory default": "capabilities.source.factoryDefault",
  "user file": "capabilities.source.userFile",
  "chat override": "capabilities.source.chatOverride"
};

function errorKey(error: ApiRequestError): string {
  if (error.kind === "network") return "groups.knownChats.unknown";
  if (error.kind !== "api") return "groups.knownChats.unavailable";
  if (error.code === "csrf_invalid") return "capabilities.errors.csrfInvalid";
  if (error.code === "settings_conflict") return "groups.knownChats.conflict";
  return "groups.knownChats.unavailable";
}

function useKnownChats(chatID: string) {
  const [settings, setSettings] = useState<KnownChats | null>(null);
  const [draft, setDraft] = useState("");
  const [restoring, setRestoring] = useState(false);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<ApiRequestError | null>(null);
  const [saved, setSaved] = useState(false);
  const [reload, setReload] = useState(0);
  const [reloading, setReloading] = useState(true);
  const requestSequence = useRef(0);
  const active = useRef(true);
  useEffect(() => {
    active.current = true;
    return () => { active.current = false; };
  }, []);
  useEffect(() => {
    let current = true;
    const sequence = ++requestSequence.current;
    setReloading(true);
    setError(null);
    void loadKnownChats(consoleApi, chatID).then(result => {
      if (!current || sequence !== requestSequence.current) return;
      setReloading(false);
      if (!result.ok) { setError(result.error); return; }
      setSettings(result.data);
      setDraft(result.data.value.join("\n"));
      setRestoring(false);
    });
    return () => { current = false; requestSequence.current++; };
  }, [chatID, reload]);
  const values = parseKnownChats(draft);
  const changed = !!settings && (restoring || draft !== settings.value.join("\n"));
  async function save() {
    if (!settings || saving || reloading || !changed || (!restoring && !values)) return;
    const sequence = ++requestSequence.current;
    setSaving(true);
    setError(null);
    setSaved(false);
    const result = await saveKnownChats(consoleApi, chatID, settings.revision, restoring ? null : values!);
    if (!active.current || sequence !== requestSequence.current) return;
    setSaving(false);
    if (!result.ok) { setError(result.error); return; }
    setSettings(result.data);
    setDraft(result.data.value.join("\n"));
    setRestoring(false);
    setSaved(true);
  }
  const busy = saving || reloading;
  return { settings, draft, restoring, saving, busy, error, saved, changed, valid: restoring || values !== undefined,
    save, reload: () => {
      if (busy) return;
      requestSequence.current++;
      setReloading(true);
      setSaved(false);
      setReload(value => value + 1);
    },
    edit: (value: string) => { if (busy) return; setDraft(value); setSaved(false); },
    restore: () => { if (busy) return; setRestoring(value => !value); setSaved(false); },
    discard: () => { if (busy) return; setDraft(settings?.value.join("\n") ?? ""); setRestoring(false); setError(null); setSaved(false); }
  };
}

export function KnownChatsEditor({ chatID }: Readonly<{ chatID: string }>) {
  const { t } = useTranslation();
  const state = useKnownChats(chatID);
  return (
    <section data-slot="card" data-known-chats-editor aria-labelledby="known-chats-title" aria-busy={state.busy}>
      <h2 id="known-chats-title">{t("groups.knownChats.title")}</h2>
      <p id="known-chats-description">{t("groups.knownChats.description")}</p>
      {!state.settings && !state.error ? <p role="status">{t("groups.loading.title")}</p> : null}
      {state.settings ? <form onSubmit={event => { event.preventDefault(); void state.save(); }}>
        <p data-setting-source={state.settings.source}>{t(sources[state.settings.source])}</p>
        <TextArea id="known-chat-ids" label={t("groups.knownChats.title")} value={state.draft}
          aria-describedby="known-chats-description" isInvalid={!state.valid}
          errorMessage={!state.valid ? t("groups.knownChats.invalid") : undefined}
          isReadOnly={state.busy || state.restoring} onChange={state.edit} />
        {state.settings.source === "chat override" ? <Button variant="secondary" isDisabled={state.busy} onPress={state.restore}>
          {t(state.restoring ? "capabilities.actions.cancelRestore" : "capabilities.actions.restore")}
        </Button> : null}
        <div data-known-chats-actions>
          <Button type="submit" variant="accent" isDisabled={!state.changed || !state.valid || state.busy}>{t(state.saving ? "capabilities.actions.saving" : "capabilities.actions.save")}</Button>
          <Button variant="secondary" isDisabled={!state.changed || state.busy} onPress={state.discard}>{t("capabilities.actions.discard")}</Button>
        </div>
      </form> : null}
      {state.error ? <div role={state.error.kind === "network" ? "status" : "alert"}>
        <SettingsLimitNotice error={state.error} messageKey={errorKey(state.error)} />
        <Button variant="secondary" isDisabled={state.busy} onPress={state.reload}>{t("groups.knownChats.reload")}</Button>
      </div> : null}
      {state.saved ? <p role="status">{t("capabilities.feedback.saved")}</p> : null}
    </section>
  );
}
