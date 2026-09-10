import { Content, Header, Heading } from "@react-spectrum/s2";
import { style } from "@react-spectrum/s2/style" with { type: "macro" };
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
    <Content
      styles={style({ display: "grid", minWidth: 0, gap: 8 })}
      data-console-page
      data-home-page
      data-home-state={controller.state.kind}
      aria-busy={isBusy || undefined}
      aria-labelledby="home-title"
    >
      <Header data-page-heading data-home-heading styles={style({ display: "grid", gap: 16 })}>
        <Heading level={1} id="home-title" styles={style({ font: "heading-2xl", margin: 0 })}>{t("home.title")}</Heading>
      </Header>
      <HomeStateContent
        state={controller.state}
        chatID={chatID}
        reload={controller.reload}
      />
    </Content>
  );
}
