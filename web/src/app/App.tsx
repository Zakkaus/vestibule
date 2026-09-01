import { createBrowserRouter, RouterProvider } from "react-router-dom";

import { EntryScreen } from "../features/entry";
import { GroupListScreen } from "../features/groups";
import { QueueScreen } from "../features/queue";
import { PreferencesScreen } from "../features/preferences";
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
        path: "groups",
        element: <GroupListScreen />,
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
