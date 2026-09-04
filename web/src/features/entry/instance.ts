import { useEffect, useState } from "react";

import { createApiTransport, nonEmptyStringFromPayload, objectFromPayload } from "../../lib/api";

const transport = createApiTransport(() => undefined);

function botUsernameFromPayload(payload: unknown): string | undefined {
  const body = objectFromPayload(payload);
  if (!body) {
    return undefined;
  }
  const username = nonEmptyStringFromPayload(body.bot_username);
  return username === undefined ? "" : `@${username.replace(/^@/, "")}`;
}

// One request per page load, shared by every caller. Which bot this instance
// runs cannot change while the page is open, and the entry screen asks for it
// from two places; without this, mounting the screen made four identical
// requests for one unchanging answer.
let pending: Promise<string | undefined> | undefined;

function readInstanceBot(): Promise<string | undefined> {
  pending ??= transport
    .request("/api/instance", { parse: botUsernameFromPayload })
    // A failed request is not an unclaimed instance. Reading it as one would put
    // "nobody has claimed this" in front of someone whose console is merely
    // unreachable, and send them looking for an install link consumed weeks ago.
    .then((result) => (result.ok ? result.data : undefined));
  return pending;
}

/**
 * The handle this deployment answers on. It is read rather than compiled in:
 * every instance runs a different bot, and a name baked into the bundle names
 * somebody else's. Undefined means the answer has not arrived or did not come;
 * the empty string means the instance is unclaimed and has no bot yet.
 */
export function useInstanceBot(): string | undefined {
  const [botUsername, setBotUsername] = useState<string | undefined>(undefined);

  useEffect(() => {
    let cancelled = false;
    void readInstanceBot().then((username) => {
      if (!cancelled) {
        setBotUsername(username);
      }
    });
    return () => {
      cancelled = true;
    };
  }, []);

  return botUsername;
}
