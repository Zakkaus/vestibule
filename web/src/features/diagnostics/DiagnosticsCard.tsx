import type { ReactNode } from "react";
import { useTranslation } from "react-i18next";

export function DiagnosticsCard({
  id,
  titleKey,
  descriptionKey,
  children
}: Readonly<{
  id: string;
  titleKey: string;
  descriptionKey: string;
  children: ReactNode;
}>) {
  const { t } = useTranslation();
  return (
    <section data-slot="card" data-diagnostics-section={id} aria-labelledby={`diagnostics-${id}-title`}>
      <header data-diagnostics-section-heading>
        <div data-diagnostics-section-copy>
          <h2 id={`diagnostics-${id}-title`}>{t(titleKey)}</h2>
          <p>{t(descriptionKey)}</p>
        </div>
      </header>
      {children}
    </section>
  );
}

export function DetailRow({
  labelKey,
  name,
  children
}: Readonly<{ labelKey: string; name: string; children: ReactNode }>) {
  const { t } = useTranslation();
  return (
    <div data-diagnostics-value={name}>
      <dt>{t(labelKey)}</dt>
      <dd>{children}</dd>
    </div>
  );
}
