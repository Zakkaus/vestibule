import { lazy, Suspense } from "react";
import { Button, Text } from "@react-spectrum/s2/Button";
import { useTranslation } from "react-i18next";
import { createBrowserRouter, RouterProvider } from "react-router-dom";

import { EntryScreen } from "../features/entry";
import { HomeLanding } from "../features/home/HomeLanding";
import { Icon } from "../icons";
import { AppShell } from "./AppShell";

const AuditScreen = lazy(() => import("../features/audit").then((module) => ({ default: module.AuditScreen })));
const BypassScreen = lazy(() => import("../features/bypass").then((module) => ({ default: module.BypassScreen })));
const CapabilitiesScreen = lazy(() => import("../features/capabilities").then((module) => ({ default: module.CapabilitiesScreen })));
const DiagnosticsScreen = lazy(() => import("../features/diagnostics").then((module) => ({ default: module.DiagnosticsScreen })));
const FeedsScreen = lazy(() => import("../features/feeds").then((module) => ({ default: module.FeedsScreen })));
const GroupListScreen = lazy(() => import("../features/groups/GroupListScreen").then((module) => ({ default: module.GroupListScreen })));
const HomeScreen = lazy(() => import("../features/home").then((module) => ({ default: module.HomeScreen })));
const ModerationScreen = lazy(() => import("../features/moderation").then((module) => ({ default: module.ModerationScreen })));
const MessagesScreen = lazy(() => import("../features/messages").then((module) => ({ default: module.MessagesScreen })));
const OwnerLimitsScreen = lazy(() => import("../features/owner").then((module) => ({ default: module.OwnerLimitsScreen })));
const QueueScreen = lazy(() => import("../features/queue").then((module) => ({ default: module.QueueScreen })));
const PreferencesScreen = lazy(() => import("../features/preferences").then((module) => ({ default: module.PreferencesScreen })));
const VerificationScreen = lazy(() => import("../features/verification").then((module) => ({ default: module.VerificationScreen })));
const QuestionsScreen = lazy(() => import("../features/questions").then((module) => ({ default: module.QuestionsScreen })));
const StatsScreen = lazy(() => import("../features/stats").then((module) => ({ default: module.StatsScreen })));
const VersionScreen = lazy(() => import("../features/version").then((module) => ({ default: module.VersionScreen })));

const entryHandle = {
  shell: "entry"
} as const;

const consoleHandle = {
  shell: "console"
} as const;

const router = createBrowserRouter([
  {
    path: "/",
    element: <AppShell />,
    children: [
      {
        index: true,
        element: <HomeLanding />,
        handle: entryHandle
      },
      {
        path: "home",
        element: <HomeScreen />,
        handle: consoleHandle
      },
      {
        path: "queue",
        element: <QueueScreen />,
        handle: consoleHandle
      },
      {
        path: "audit",
        element: <AuditScreen />,
        handle: consoleHandle
      },
      {
        path: "stats",
        element: <StatsScreen />,
        handle: consoleHandle
      },
      {
        path: "diagnostics",
        element: <DiagnosticsScreen />,
        handle: consoleHandle
      },
      {
        path: "version",
        element: <VersionScreen />,
        handle: consoleHandle
      },
      {
        path: "verification",
        element: <VerificationScreen />,
        handle: consoleHandle
      },
      {
        path: "bypass",
        element: <BypassScreen />,
        handle: consoleHandle
      },
      {
        path: "questions",
        element: <QuestionsScreen />,
        handle: consoleHandle
      },
      {
        path: "feeds",
        element: <FeedsScreen />,
        handle: consoleHandle
      },
      {
        path: "groups",
        element: <GroupListScreen />,
        handle: consoleHandle
      },
      {
        path: "moderation",
        element: <ModerationScreen />,
        handle: consoleHandle
      },
      {
        path: "messages",
        element: <MessagesScreen />,
        handle: consoleHandle
      },
      {
        path: "capabilities",
        element: <CapabilitiesScreen />,
        handle: consoleHandle
      },
      {
        path: "owner",
        element: <OwnerLimitsScreen />,
        handle: consoleHandle
      },
      {
        path: "preferences",
        element: <PreferencesScreen />,
        handle: consoleHandle
      },
      {
        path: "*",
        element: <EntryScreen />,
        handle: entryHandle
      }
    ].map((route) => ({ ...route, errorElement: <RouteError /> }))
  }
]);

function RouteLoading() {
  const { t } = useTranslation();
  return (
    <section data-entry-page data-entry-state="loading" aria-busy="true" aria-labelledby="route-loading-title">
      <div data-slot="card">
        <h1 id="route-loading-title">
          <span data-state-heading><Icon name="loaderCircle" />{t("entry.loading.title")}</span>
        </h1>
        <p data-entry-copy aria-live="polite">{t("entry.loading.description")}</p>
      </div>
    </section>
  );
}

function RouteError() {
  const { t } = useTranslation();
  return (
    <section data-slot="card" data-route-error role="alert" aria-labelledby="route-error-title">
      <h1 id="route-error-title">
        <span data-state-heading><Icon name="circleAlert" />{t("routeError.title")}</span>
      </h1>
      <p>{t("routeError.description")}</p>
      <Button variant="primary" onPress={() => window.location.reload()}>
        <Icon name="refreshCw" />
        <Text>{t("routeError.reload")}</Text>
      </Button>
    </section>
  );
}

export function App() {
  return <Suspense fallback={<RouteLoading />}><RouterProvider router={router} /></Suspense>;
}
