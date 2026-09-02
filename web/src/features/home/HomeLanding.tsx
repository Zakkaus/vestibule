import { Navigate } from "react-router-dom";

import { useConsoleSession } from "../../app/session";
import { EntryScreen } from "../entry";

export function HomeLanding() {
  const session = useConsoleSession();
  if (session.state === "ready") {
    const firstChat = session.chats[0];
    if (firstChat) {
      const search = new URLSearchParams({ group: firstChat.id });
      return <Navigate to={`/home?${search.toString()}`} replace />;
    }
  }

  return <EntryScreen />;
}
