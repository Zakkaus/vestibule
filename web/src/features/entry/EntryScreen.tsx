import { useTranslation } from "react-i18next";
import { useSearchParams } from "react-router-dom";

import { entryFixtureFor } from "./fixtures";

export function EntryScreen() {
  const { t } = useTranslation();
  const [searchParams] = useSearchParams();
  const fixture = entryFixtureFor(searchParams.get("state"));

  return (
    <section data-entry-page data-entry-state={fixture.id} aria-labelledby="entry-title">
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
