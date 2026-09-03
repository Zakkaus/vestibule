import { useEffect, useState } from "react";

import { createApiTransport, objectFromPayload, nonEmptyStringFromPayload } from "../../lib/api";

const transport = createApiTransport(() => undefined);

function botUsernameFromPayload(payload: unknown): string | undefined {
  const body = objectFromPayload(payload);
  if (!body) {
    return undefined;
  }
  const username = nonEmptyStringFromPayload(body.bot_username);
  return username === undefined ? "" : `@${username.replace(/^@/, "")}`;
}

/**
 * The handle this deployment answers on. It is read rather than compiled in:
 * every instance runs a different bot, and a name baked into the bundle names
 * somebody else's. Undefined means the answer has not arrived; the empty string
 * means the instance is unclaimed and has no bot yet.
 */
export function useInstanceBot(): string | undefined {
  const [botUsername, setBotUsername] = useState<string | undefined>(undefined);

  useEffect(() => {
    let cancelled = false;
    void transport
      .request("/api/instance", { parse: botUsernameFromPayload })
      .then((result) => {
        if (!cancelled) {
          setBotUsername(result.ok ? result.data : "");
        }
      });
    return () => {
      cancelled = true;
    };
  }, []);

  return botUsername;
}
