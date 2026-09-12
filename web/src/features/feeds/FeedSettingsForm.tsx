import { Button } from "@react-spectrum/s2/Button";
import { Text } from "@react-spectrum/s2";
import type { FormEvent, ReactNode } from "react";
import { useTranslation } from "react-i18next";

import { AppSelect, type AppSelectOption } from "../../components/AppSelect";
import { useConsoleSize } from "../../components/ConsoleProvider";
import { StatusBadge } from "../../components/StatusBadge";
import { Icon } from "../../icons";
import type {
  FeedLanguage,
  FeedSettingSource,
  FeedSettings,
  Setting
} from "./api";
import type { FeedDraft, FeedFieldError } from "./form";

const languageMessageKeys: Readonly<Record<FeedLanguage, string>> = {
  "": "feeds.languages.default",
  zh: "feeds.languages.zh",
  "zh-Hant": "feeds.languages.zhHant",
  en: "feeds.languages.en",
  ja: "feeds.languages.ja",
  ru: "feeds.languages.ru"
};

const sourceMessageKeys: Readonly<Record<FeedSettingSource, string>> = {
  "factory default": "feeds.source.factoryDefault",
  "user file": "feeds.source.userFile",
  "chat override": "feeds.source.chatOverride"
};

type FeedSettingsFormProps = Readonly<{
  settings: FeedSettings;
  draft: FeedDraft;
  errors: FeedFieldError;
  saveBarErrorKey?: string;
  saving: boolean;
  hasChanges: boolean;
  onSubmit: (event: FormEvent<HTMLFormElement>) => void;
  onDraftChange: (draft: FeedDraft) => void;
  onRestoreFactory: () => void;
}>;

type SettingRowProps = Readonly<{
  field: string;
  controlID: string;
  labelKey: string;
  descriptionKey: string;
  source: FeedSettingSource;
  errorKey?: string;
  children: (describedBy: string, labelledBy: string) => ReactNode;
}>;

function SourceBadge({ source }: Readonly<{ source: FeedSettingSource }>) {
  const { t } = useTranslation();
  return (
    <StatusBadge tone="neutral">
      <span data-feed-setting-source={source}>
        {t("feeds.source.value", { source: t(sourceMessageKeys[source]) })}
      </span>
    </StatusBadge>
  );
}

function SettingRow({
  field,
  controlID,
  labelKey,
  descriptionKey,
  source,
  errorKey,
  children
}: SettingRowProps) {
  const { t } = useTranslation();
  const descriptionID = `${controlID}-description`;
  const errorID = `${controlID}-error`;
  const labelID = `${controlID}-label`;
  const describedBy = errorKey ? `${descriptionID} ${errorID}` : descriptionID;
  return (
    <div data-slot="setting" data-feed-setting={field}>
      <div data-setting-copy>
        <div data-setting-heading>
          <label id={labelID} htmlFor={controlID}>{t(labelKey)}</label>
          <SourceBadge source={source} />
        </div>
        <p id={descriptionID} data-setting-description>{t(descriptionKey)}</p>
        {errorKey ? <p id={errorID} data-slot="field-error" role="alert">{t(errorKey)}</p> : null}
      </div>
      <div data-setting-control>{children(describedBy, labelID)}</div>
    </div>
  );
}

function TextSetting({
  field,
  controlID,
  labelKey,
  descriptionKey,
  setting,
  value,
  errorKey,
  saving,
  onChange
}: Readonly<{
  field: string;
  controlID: string;
  labelKey: string;
  descriptionKey: string;
  setting: Setting<string>;
  value: string;
  errorKey?: string;
  saving: boolean;
  onChange: (value: string) => void;
}>) {
  return (
    <SettingRow
      field={field}
      controlID={controlID}
      labelKey={labelKey}
      descriptionKey={descriptionKey}
      source={setting.source}
      errorKey={errorKey}
    >
      {(describedBy) => (
        <input
          id={controlID}
          data-slot="input"
          type="text"
          value={value}
          readOnly={saving}
          aria-invalid={errorKey ? "true" : undefined}
          aria-describedby={describedBy}
          onChange={(event) => onChange(event.currentTarget.value)}
        />
      )}
    </SettingRow>
  );
}

function SwitchSetting({
  field,
  controlID,
  labelKey,
  descriptionKey,
  setting,
  value,
  errorKey,
  saving,
  onChange
}: Readonly<{
  field: string;
  controlID: string;
  labelKey: string;
  descriptionKey: string;
  setting: Setting<boolean>;
  value: boolean;
  errorKey?: string;
  saving: boolean;
  onChange: (value: boolean) => void;
}>) {
  return (
    <SettingRow
      field={field}
      controlID={controlID}
      labelKey={labelKey}
      descriptionKey={descriptionKey}
      source={setting.source}
      errorKey={errorKey}
    >
      {(describedBy, labelledBy) => (
        <button
          id={controlID}
          type="button"
          data-slot="switch"
          role="switch"
          aria-checked={value}
          aria-labelledby={labelledBy}
          aria-describedby={describedBy}
          aria-disabled={saving ? "true" : undefined}
          onClick={() => {
            if (!saving) onChange(!value);
          }}
        />
      )}
    </SettingRow>
  );
}

function RepositoryRow({
  repository,
  index,
  source,
  errors,
  saving,
  onChange,
  onRemove
}: Readonly<{
  repository: FeedDraft["github_repos"][number];
  index: number;
  source: FeedSettingSource;
  errors: FeedFieldError;
  saving: boolean;
  onChange: (repository: FeedDraft["github_repos"][number]) => void;
  onRemove: () => void;
}>) {
  const { t } = useTranslation();
  const size = useConsoleSize("L");
  const base = `github_repos[${index}]`;
  const rowID = `feeds-repository-${index}`;
  const rowErrorKey = errors[base];
  const repositoryErrorKey = errors[`${base}.repo`] ?? rowErrorKey;
  const branchErrorKey = errors[`${base}.branch`];
  const issuesErrorKey = errors[`${base}.issues`];
  const pullsErrorKey = errors[`${base}.pulls`];
  const issuesErrorID = `${rowID}-issues-error`;
  const pullsErrorID = `${rowID}-pulls-error`;
  const rowDescriptionID = `${rowID}-description`;
  const rowErrorID = `${rowID}-error`;
  const rowDescribedBy = rowErrorKey ? `${rowDescriptionID} ${rowErrorID}` : rowDescriptionID;
  const fieldDescription = (name: string) => `${rowID}-${name}-description`;

  return (
    <article data-feed-repository={String(index)} data-feed-repository-source={source}>
      <header data-feed-repository-heading>
        <div data-setting-heading>
          <h3>{t("feeds.repositories.item", { number: index + 1 })}</h3>
          <SourceBadge source={source} />
        </div>
        <Button
          type="button"
          variant="secondary"
          size={size}
          data-slot="button"
          isDisabled={saving}
          onPress={onRemove}
        >
          <Icon name="x" />
          <Text>{t("feeds.actions.removeRepository")}</Text>
        </Button>
      </header>
      <p id={rowDescriptionID} data-setting-description>{t("feeds.repositories.itemDescription")}</p>
      {rowErrorKey ? <p id={rowErrorID} data-slot="field-error" role="alert">{t(rowErrorKey)}</p> : null}
      <div data-feed-repository-fields>
        <label>
          <span>{t("feeds.repositories.repository")}</span>
          <input
            id={`${rowID}-repo`}
            data-slot="input"
            type="text"
            value={repository.repo}
            readOnly={saving}
            aria-invalid={repositoryErrorKey ? "true" : undefined}
            aria-describedby={`${rowDescribedBy} ${fieldDescription("repo")}${repositoryErrorKey && !rowErrorKey ? ` ${rowID}-repo-error` : ""}`}
            onChange={(event) => onChange({ ...repository, repo: event.currentTarget.value })}
          />
          <small id={fieldDescription("repo")}>{t("feeds.repositories.repositoryDescription")}</small>
          {repositoryErrorKey && !rowErrorKey ? <p id={`${rowID}-repo-error`} data-slot="field-error" role="alert">{t(repositoryErrorKey)}</p> : null}
        </label>
        <label>
          <span>{t("feeds.repositories.branch")}</span>
          <input
            id={`${rowID}-branch`}
            data-slot="input"
            type="text"
            value={repository.branch}
            readOnly={saving}
            aria-invalid={branchErrorKey ? "true" : undefined}
            aria-describedby={`${rowDescribedBy} ${fieldDescription("branch")}`}
            onChange={(event) => onChange({ ...repository, branch: event.currentTarget.value })}
          />
          <small id={fieldDescription("branch")}>{t("feeds.repositories.branchDescription")}</small>
          {branchErrorKey ? <p id={`${rowID}-branch-error`} data-slot="field-error" role="alert">{t(branchErrorKey)}</p> : null}
        </label>
        <label data-feed-repository-switch>
          <span>{t("feeds.repositories.issues")}</span>
          <button
            type="button"
            data-slot="switch"
            role="switch"
            aria-checked={repository.issues}
            aria-invalid={issuesErrorKey ? "true" : undefined}
            aria-disabled={saving ? "true" : undefined}
            aria-describedby={`${rowDescribedBy} ${fieldDescription("issues")}${issuesErrorKey ? ` ${issuesErrorID}` : ""}`}
            onClick={() => {
              if (!saving) onChange({ ...repository, issues: !repository.issues });
            }}
          />
          <small id={fieldDescription("issues")}>{t("feeds.repositories.issuesDescription")}</small>
          {issuesErrorKey ? <p id={issuesErrorID} data-slot="field-error" role="alert">{t(issuesErrorKey)}</p> : null}
        </label>
        <label data-feed-repository-switch>
          <span>{t("feeds.repositories.pulls")}</span>
          <button
            type="button"
            data-slot="switch"
            role="switch"
            aria-checked={repository.pulls}
            aria-invalid={pullsErrorKey ? "true" : undefined}
            aria-disabled={saving ? "true" : undefined}
            aria-describedby={`${rowDescribedBy} ${fieldDescription("pulls")}${pullsErrorKey ? ` ${pullsErrorID}` : ""}`}
            onClick={() => {
              if (!saving) onChange({ ...repository, pulls: !repository.pulls });
            }}
          />
          <small id={fieldDescription("pulls")}>{t("feeds.repositories.pullsDescription")}</small>
          {pullsErrorKey ? <p id={pullsErrorID} data-slot="field-error" role="alert">{t(pullsErrorKey)}</p> : null}
        </label>
      </div>
    </article>
  );
}

export function FeedSettingsForm({
  settings,
  draft,
  errors,
  saveBarErrorKey,
  saving,
  hasChanges,
  onSubmit,
  onDraftChange,
  onRestoreFactory
}: FeedSettingsFormProps) {
  const { t } = useTranslation();
  const size = useConsoleSize("L");
  const languageOptions: readonly AppSelectOption<FeedLanguage>[] = Object.keys(languageMessageKeys).map((value) => ({
    value: value as FeedLanguage,
    label: t(languageMessageKeys[value as FeedLanguage])
  }));
  const update = <K extends keyof FeedDraft>(field: K, value: FeedDraft[K]) => {
    onDraftChange({ ...draft, [field]: value });
  };
  const updateRepository = (index: number, repository: FeedDraft["github_repos"][number]) => {
    update("github_repos", draft.github_repos.map((current, currentIndex) => currentIndex === index ? repository : current));
  };
  const addRepository = () => update("github_repos", [...draft.github_repos, { repo: "", branch: "", issues: false, pulls: false }]);
  const removeRepository = (index: number) => update("github_repos", draft.github_repos.filter((_, currentIndex) => currentIndex !== index));

  return (
    <form data-feeds-form data-save-state={saving ? "submitting" : "idle"} aria-busy={saving || undefined} noValidate onSubmit={onSubmit}>
      <section data-slot="card" data-feeds-section="feed" aria-labelledby="feeds-form-title">
        <div data-feeds-section-heading>
          <div data-feeds-section-copy>
            <h2 id="feeds-form-title">{t("feeds.form.title")}</h2>
            <p>{t("feeds.form.description")}</p>
          </div>
          <SourceBadge source={settings.githubRepos.source} />
        </div>
        <SettingRow
          field="lang"
          controlID="feeds-language"
          labelKey="feeds.fields.language.label"
          descriptionKey="feeds.fields.language.description"
          source={settings.feed.lang.source}
          errorKey={errors.lang}
        >
          {(describedBy) => (
            <AppSelect
              id="feeds-language"
              aria-label={t("feeds.fields.language.label")}
              aria-describedby={describedBy}
              aria-invalid={errors.lang ? "true" : undefined}
              aria-disabled={saving ? "true" : undefined}
              value={draft.lang}
              options={languageOptions}
              onValueChange={(value) => update("lang", value)}
            />
          )}
        </SettingRow>
        <SettingRow
          field="interval_seconds"
          controlID="feeds-interval-seconds"
          labelKey="feeds.fields.interval.label"
          descriptionKey="feeds.fields.interval.description"
          source={settings.feed.intervalSeconds.source}
          errorKey={errors.interval_seconds}
        >
          {(describedBy) => (
            <input
              id="feeds-interval-seconds"
              data-slot="input"
              data-feeds-number
              type="number"
              inputMode="numeric"
              step="1"
              min="60"
              max="86400"
              value={draft.interval_seconds}
              readOnly={saving}
              aria-invalid={errors.interval_seconds ? "true" : undefined}
              aria-describedby={describedBy}
              onChange={(event) => update("interval_seconds", event.currentTarget.value)}
            />
          )}
        </SettingRow>
        <SwitchSetting field="bugs" controlID="feeds-bugs" labelKey="feeds.fields.bugs.label" descriptionKey="feeds.fields.bugs.description" setting={settings.feed.bugs} value={draft.bugs} errorKey={errors.bugs} saving={saving} onChange={(value) => update("bugs", value)} />
        <SwitchSetting field="news" controlID="feeds-news" labelKey="feeds.fields.news.label" descriptionKey="feeds.fields.news.description" setting={settings.feed.news} value={draft.news} errorKey={errors.news} saving={saving} onChange={(value) => update("news", value)} />
        <TextSetting field="bug_product" controlID="feeds-bug-product" labelKey="feeds.fields.bugProduct.label" descriptionKey="feeds.fields.bugProduct.description" setting={settings.feed.bugProduct} value={draft.bug_product} errorKey={errors.bug_product} saving={saving} onChange={(value) => update("bug_product", value)} />
        <TextSetting field="bug_component" controlID="feeds-bug-component" labelKey="feeds.fields.bugComponent.label" descriptionKey="feeds.fields.bugComponent.description" setting={settings.feed.bugComponent} value={draft.bug_component} errorKey={errors.bug_component} saving={saving} onChange={(value) => update("bug_component", value)} />
        <SwitchSetting field="silent_bugs" controlID="feeds-silent-bugs" labelKey="feeds.fields.silentBugs.label" descriptionKey="feeds.fields.silentBugs.description" setting={settings.feed.silentBugs} value={draft.silent_bugs} errorKey={errors.silent_bugs} saving={saving} onChange={(value) => update("silent_bugs", value)} />
      </section>

      <section data-slot="card" data-feeds-section="github-repos" aria-labelledby="feeds-repositories-title">
        <div data-feeds-section-heading>
          <div data-feeds-section-copy>
            <h2 id="feeds-repositories-title">{t("feeds.repositories.title")}</h2>
            <p>{t("feeds.repositories.description")}</p>
          </div>
          <SourceBadge source={settings.githubRepos.source} />
        </div>
        <div data-feed-repositories>
          {draft.github_repos.map((repository, index) => (
            <RepositoryRow
              key={index}
              repository={repository}
              index={index}
              source={settings.githubRepos.source}
              errors={errors}
              saving={saving}
              onChange={(next) => updateRepository(index, next)}
              onRemove={() => removeRepository(index)}
            />
          ))}
        </div>
        <Button type="button" variant="secondary" size={size} data-slot="button" isDisabled={saving} onPress={addRepository}>
          <Icon name="plus" />
          <Text>{t("feeds.actions.addRepository")}</Text>
        </Button>
      </section>

      <footer data-feeds-savebar data-save-state={saving ? "submitting" : "idle"} aria-label={t("feeds.save.label")}>
        <div data-feeds-save-copy>
          <span aria-live="polite">{t(hasChanges ? "feeds.save.dirty" : "feeds.save.clean")}</span>
          {saveBarErrorKey ? <p role="alert">{t(saveBarErrorKey)}</p> : null}
        </div>
        <div data-feeds-save-actions>
          <Button type="button" variant="secondary" size={size} data-slot="button" isDisabled={saving} onPress={onRestoreFactory}>
            <Icon name="rotateCcw" />
            <Text>{t("feeds.actions.restoreFactory")}</Text>
          </Button>
          <Button type="submit" variant="accent" size={size} data-slot="button" isDisabled={!hasChanges || saving} isPending={saving} aria-disabled={saving ? "true" : undefined}>
            <Icon name="save" />
            <Text>{t(saving ? "feeds.actions.saving" : "feeds.actions.save")}</Text>
          </Button>
        </div>
      </footer>
    </form>
  );
}
