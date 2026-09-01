import { useTranslation } from "react-i18next";

import { UtilityControls } from "../../components/UtilityControls";

export function PreferencesScreen() {
  const { t } = useTranslation();

  return (
    <section data-preferences-page aria-labelledby="preferences-title">
      <header data-page-heading>
        <h1 id="preferences-title">{t("preferences.title")}</h1>
        <p>{t("preferences.description")}</p>
      </header>
      <section data-slot="card" data-preference-local aria-labelledby="preferences-interface-title">
        <div>
          <h2 id="preferences-interface-title">{t("preferences.interface.title")}</h2>
          <p>{t("preferences.interface.description")}</p>
        </div>
        <UtilityControls />
      </section>
    </section>
  );
}
