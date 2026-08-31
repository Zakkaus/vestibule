import { createBrowserRouter, RouterProvider } from "react-router-dom";

import { EntryScreen } from "../features/entry";
import { GroupListScreen } from "../features/groups";
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
        path: "groups",
        element: <GroupListScreen />,
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
