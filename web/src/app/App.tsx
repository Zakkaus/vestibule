import { createBrowserRouter, RouterProvider } from "react-router-dom";

import { AuditScreen } from "../features/audit";
import { BypassScreen } from "../features/bypass";
import { CapabilitiesScreen } from "../features/capabilities";
import { DiagnosticsScreen } from "../features/diagnostics";
import { EntryScreen } from "../features/entry";
import { FeedsScreen } from "../features/feeds";
import { GroupListScreen } from "../features/groups";
import { HomeLanding, HomeScreen } from "../features/home";
import { ModerationScreen } from "../features/moderation";
import { MessagesScreen } from "../features/messages";
import { QueueScreen } from "../features/queue";
import { PreferencesScreen } from "../features/preferences";
import { VerificationScreen } from "../features/verification";
import { QuestionsScreen } from "../features/questions";
import { StatsScreen } from "../features/stats";
import { VersionScreen } from "../features/version";
import { AppShell } from "./AppShell";

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
        path: "preferences",
        element: <PreferencesScreen />,
        handle: consoleHandle
      },
      {
        path: "*",
        element: <EntryScreen />,
        handle: entryHandle
      }
    ]
  }
]);

export function App() {
  return <RouterProvider router={router} />;
}
