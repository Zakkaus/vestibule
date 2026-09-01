import { createBrowserRouter, RouterProvider } from "react-router-dom";

import { AuditScreen } from "../features/audit";
import { EntryScreen } from "../features/entry";
import { GroupListScreen } from "../features/groups";
import { ModerationScreen } from "../features/moderation";
import { QueueScreen } from "../features/queue";
import { PreferencesScreen } from "../features/preferences";
import { VerificationScreen } from "../features/verification";
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
        element: <EntryScreen />,
        handle: entryHandle
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
        path: "verification",
        element: <VerificationScreen />,
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
