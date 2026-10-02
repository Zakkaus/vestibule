import { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { Button } from "@react-spectrum/s2/Button";
import { consoleApi } from "../../app/session";
import { StatusBadge } from "../../components/StatusBadge";
import { Icon } from "../../icons";
import type { ApiRequestError } from "../../lib/api";
import { diagnosticsErrorMessageKey } from "./errors";
import { loadProcessSettings, type ProcessSettings, type OverlayConfig } from "./process";
import "./process.css";

const sourceMessageKeys = {
  "factory default": "feeds.source.factoryDefault",
  "user file": "feeds.source.userFile"
} as const;

function SourceBadge({ source }: Readonly<{ source: keyof typeof sourceMessageKeys }>) {
  const { t } = useTranslation();
  return <StatusBadge tone="neutral">{t("feeds.source.value", { source: t(sourceMessageKeys[source]) })}</StatusBadge>;
}
function SectionHeading({ id, titleKey, descriptionKey, source }: Readonly<{
  id: string;
  titleKey: string;
  descriptionKey: string;
  source: keyof typeof sourceMessageKeys;
}>) {
  const { t } = useTranslation();
  return (
    <header data-feeds-section-heading>
      <div data-feeds-section-copy><h2 id={id}>{t(titleKey)}</h2><p>{t(descriptionKey)}</p></div>
      <SourceBadge source={source} />
    </header>
  );
}

function NewsURLSection({ settings }: Readonly<{ settings: ProcessSettings["newsURL"] }>) {
  const { t } = useTranslation();
  return (
    <section data-slot="card" data-feeds-section="news-url" data-process-setting-source={settings.source} aria-labelledby="feeds-news-url-title">
      <SectionHeading id="feeds-news-url-title" titleKey="feeds.newsURL.title" descriptionKey="feeds.newsURL.description" source={settings.source} />
      <div className="surface-raised" data-news-url-value>
        {settings.value ? <code>{settings.value}</code> : <span data-state-heading><Icon name="inbox" />{t("feeds.newsURL.empty")}</span>}
      </div>
    </section>
  );
}

function OverlayItem({ overlay, number }: Readonly<{ overlay: OverlayConfig; number: number }>) {
  const { t } = useTranslation();
  return (
    <article className="surface-raised" data-overlay-item aria-labelledby={`overlay-item-${number}-title`}>
      <h3 id={`overlay-item-${number}-title`}>{overlay.name || overlay.repo}</h3>
      <dl data-overlay-values>
        <div data-overlay-value><dt>{t("feeds.overlays.repository")}</dt><dd><code>{overlay.repo}</code></dd></div>
        <div data-overlay-value><dt>{t("feeds.overlays.branch")}</dt><dd><code>{overlay.branch || t("feeds.overlays.defaultBranch")}</code></dd></div>
      </dl>
    </article>
  );
}

function OverlaySection({ settings }: Readonly<{ settings: ProcessSettings["overlays"] }>) {
  const { t } = useTranslation();
  return (
    <section data-slot="card" data-feeds-section="overlays" data-process-setting-source={settings.source} aria-labelledby="feeds-overlays-title">
      <SectionHeading id="feeds-overlays-title" titleKey="feeds.overlays.title" descriptionKey="feeds.overlays.description" source={settings.source} />
      {settings.value.length === 0 ? (
        <div className="surface-raised" data-overlays-empty><div data-state-heading><Icon name="inbox" /><strong>{t("feeds.overlays.emptyTitle")}</strong></div><p>{t("feeds.overlays.emptyDescription")}</p></div>
      ) : <div data-overlay-list>{settings.value.map((overlay, index) => <OverlayItem key={`${overlay.name}:${index}`} overlay={overlay} number={index + 1} />)}</div>}
    </section>
  );
}

export function ProcessSettingsSection() {
  const { t } = useTranslation();
  const [settings, setSettings] = useState<ProcessSettings | null>(null);
  const [error, setError] = useState<ApiRequestError | null>(null);
  const [reload, setReload] = useState(0);
  useEffect(() => {
    let active = true;
    setError(null);
    void loadProcessSettings(consoleApi).then(result => {
      if (!active) return;
      if (result.ok) setSettings(result.data);
      else setError(result.error);
    });
    return () => { active = false; };
  }, [reload]);
  return (
    <section data-process-settings aria-labelledby="process-settings-title">
      <h2 id="process-settings-title">{t("diagnostics.process.title")}</h2>
      <p>{t("diagnostics.process.description")}</p>
      {error ? <div role="alert">
        <p>{t(error.kind === "api" ? diagnosticsErrorMessageKey(error, "feeds.errors.settingsUnavailable") : "feeds.errors.settingsUnavailable")}</p>
        <Button variant="secondary" onPress={() => setReload(value => value + 1)}>{t("feeds.actions.reload")}</Button>
      </div> : null}
      {!settings && !error ? <p role="status">{t("diagnostics.loading.title")}</p> : null}
      {settings ? <>
        <section data-slot="card" data-process-setting-source={settings.privateQueryPerMin.source}>
          <h3>{t("diagnostics.process.privateQueryPerMin")}</h3>
          <SourceBadge source={settings.privateQueryPerMin.source} />
          <p data-process-private-query-rate>{settings.privateQueryPerMin.value}</p>
        </section>
        <NewsURLSection settings={settings.newsURL} />
        <OverlaySection settings={settings.overlays} />
      </> : null}
    </section>
  );
}
