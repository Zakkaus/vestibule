import { useTranslation } from "react-i18next";
import { Navigate, useSearchParams } from "react-router-dom";

import { retryConsoleSession, useConsoleSession } from "../../app/session";
import type { ApiRequestError } from "../../lib/api";
import { Icon } from "../../icons";
import {
  entryFixtureFor,
  entryFixtures,
  unclaimedEntryFixture,
  type EntryFixture
} from "./fixtures";
import { useInstanceBot } from "./instance";

type EntryFixtureContentProps = Readonly<{
  fixture: EntryFixture;
  botUsername?: string;
  transportFailure?: ApiRequestError;
}>;

function EntryFixtureContent({ fixture, botUsername, transportFailure }: EntryFixtureContentProps) {
  const { t } = useTranslation();
  // Every deployment runs its own bot, so the handle is read from the instance
  // rather than compiled into the bundle. Until the answer arrives the copy says
  // "this instance's bot"; once it arrives empty, the screen has already switched
  // to the unclaimed state, which names no bot at all.
  const interpolation = {
    ...fixture.interpolation,
    botUsername: botUsername ? botUsername : t("entry.thisBot")
  };

  return (
    <section
      data-entry-page
      data-entry-state={fixture.id}
      data-entry-transport-failure={transportFailure?.kind}
      aria-labelledby="entry-title"
    >
      <div data-slot="card">
        <h1 id="entry-title">
          <span data-state-heading>
            <Icon name={fixture.icon} />
            {t(fixture.titleKey, interpolation)}
          </span>
        </h1>
        <p data-entry-copy>{t(fixture.descriptionKey, interpolation)}</p>
        <section aria-labelledby="entry-next-steps">
          <h2 id="entry-next-steps">{t("entry.nextSteps")}</h2>
          <ol data-entry-steps>
            {fixture.stepKeys.map((stepKey) => (
              <li key={stepKey}>
                <Icon name="chevronRight" />
                <span>{t(stepKey, interpolation)}</span>
              </li>
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
        <h1 id="entry-title">
          <span data-state-heading>
            <Icon name="loaderCircle" />
            {t("entry.loading.title")}
          </span>
        </h1>
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
        <h1 id="entry-title">
          <span data-state-heading>
            <Icon name="circleAlert" />
            {t("entry.unavailable.title")}
          </span>
        </h1>
        <p data-entry-copy role="alert">
          {t("entry.unavailable.description")}
        </p>
        <button
          type="button"
          data-slot="button"
          data-variant="outline"
          data-size="sm"
          onClick={() => {
            void retryConsoleSession();
          }}
        >
          <Icon name="refreshCw" />
          {t("entry.unavailable.retry")}
        </button>
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
  const botUsername = useInstanceBot();

  // An unclaimed instance has no bot, so none of the states below apply: there
  // is nothing to open in Telegram and no session anyone could hold. The empty
  // string is the answer having arrived and being empty, which is how the server
  // reports an instance nobody has claimed.
  if (botUsername === "" && searchParams.get("state") === null) {
    return <EntryFixtureContent fixture={unclaimedEntryFixture} botUsername={botUsername} />;
  }

  if (session.state === "ready") {
    return <Navigate to="/groups" replace />;
  }

  if (session.state === "no-groups") {
    const fixture = entryFixtureFor("no-groups");
    return (
      <EntryFixtureContent
        botUsername={botUsername}
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
    return (
      <EntryFixtureContent
        fixture={fixture}
        botUsername={botUsername}
        transportFailure={session.error}
      />
    );
  }

  return <EntryUnavailable error={session.error} />;
}
