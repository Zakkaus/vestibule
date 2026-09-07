import { useTranslation } from "react-i18next";
import { useSearchParams } from "react-router-dom";

import { useConsoleSession } from "../../app/session";
import { HomeStateContent } from "./HomeState";
import { useHomeData } from "./useHomeData";

export function HomeScreen() {
  const { t } = useTranslation();
  const [searchParams] = useSearchParams();
  const session = useConsoleSession();
  const chatID = searchParams.get("group") ?? undefined;
  const controller = useHomeData(session, chatID);
  const isBusy = controller.state.kind === "loading";

  return (
    <section
      data-console-page
      data-home-page
      data-home-state={controller.state.kind}
      aria-busy={isBusy || undefined}
      aria-labelledby="home-title"
    >
      <header data-page-heading data-home-heading>
        <h1 id="home-title">{t("home.title")}</h1>
        <p>{t("home.description")}</p>
      </header>
      <HomeStateContent
        state={controller.state}
        chatID={chatID}
        reload={controller.reload}
      />
    </section>
  );
}
