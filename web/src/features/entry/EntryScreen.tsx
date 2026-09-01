import { useTranslation } from "react-i18next";
import { Navigate, useSearchParams } from "react-router-dom";

import { useConsoleSession } from "../../app/session";
import type { ApiRequestError } from "../../lib/api";
import { entryFixtureFor, entryFixtures, type EntryFixture } from "./fixtures";

type EntryFixtureContentProps = Readonly<{
  fixture: EntryFixture;
  transportFailure?: ApiRequestError;
}>;

function EntryFixtureContent({ fixture, transportFailure }: EntryFixtureContentProps) {
  const { t } = useTranslation();

  return (
    <section
      data-entry-page
      data-entry-state={fixture.id}
      data-entry-transport-failure={transportFailure?.kind}
      aria-labelledby="entry-title"
    >
      <div data-slot="card">
        <h1 id="entry-title">{t(fixture.titleKey, fixture.interpolation)}</h1>
        <p data-entry-copy>{t(fixture.descriptionKey, fixture.interpolation)}</p>
        <section aria-labelledby="entry-next-steps">
          <h2 id="entry-next-steps">{t("entry.nextSteps")}</h2>
          <ol data-entry-steps>
            {fixture.stepKeys.map((stepKey) => (
              <li key={stepKey}>{t(stepKey, fixture.interpolation)}</li>
            ))}
          </ol>
        </section>
      </div>
    </section>
  );
}

function EntryLoading() {
  const { t } = useTranslation();

  return (
    <section
      data-entry-page
      data-entry-state="loading"
      aria-busy="true"
      aria-labelledby="entry-title"
    >
      <div data-slot="card">
        <h1 id="entry-title">{t("entry.loading.title")}</h1>
        <p data-entry-copy aria-live="polite">
          {t("entry.loading.description")}
        </p>
      </div>
    </section>
  );
}

function EntryUnavailable({ error }: Readonly<{ error: ApiRequestError }>) {
  const { t } = useTranslation();

  return (
    <section
      data-entry-page
      data-entry-state="unavailable"
      data-entry-transport-failure={error.kind}
      aria-labelledby="entry-title"
    >
      <div data-slot="card">
        <h1 id="entry-title">{t("entry.unavailable.title")}</h1>
        <p data-entry-copy role="alert">
          {t("entry.unavailable.description")}
        </p>
      </div>
    </section>
  );
}

function fixtureForBlockedSession(
  error: ApiRequestError,
  stateHint: string | null
): EntryFixture | undefined {
  const hintedFixture = entryFixtures.find((fixture) => fixture.id === stateHint);
  if (hintedFixture) {
    return hintedFixture;
  }

  if (error.kind === "non-json" && import.meta.env.DEV) {
    return entryFixtureFor(null);
  }

  if (error.kind !== "api") {
    return undefined;
  }

  switch (error.code) {
    case "authentication_expired":
    case "authentication_invalid":
    case "init_data_replayed":
      return entryFixtureFor(null);
    default:
      return undefined;
  }
}

export function EntryScreen() {
  const [searchParams] = useSearchParams();
  const session = useConsoleSession();

  if (session.state === "ready") {
    return <Navigate to="/groups" replace />;
  }

  if (session.state === "no-groups") {
    const fixture = entryFixtureFor("no-groups");
    return (
      <EntryFixtureContent
        fixture={{
          ...fixture,
          interpolation: {
            ...fixture.interpolation,
            accountId: session.session.subject.telegramId
          }
        }}
      />
    );
  }

  if (session.state === "loading" || session.state === "checking-groups") {
    return <EntryLoading />;
  }

  if (session.state === "groups-unavailable") {
    return <EntryUnavailable error={session.error} />;
  }

  const fixture = fixtureForBlockedSession(session.error, searchParams.get("state"));
  if (fixture) {
    return <EntryFixtureContent fixture={fixture} transportFailure={session.error} />;
  }

  return <EntryUnavailable error={session.error} />;
}
