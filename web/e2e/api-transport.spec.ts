import { expect, test } from "@playwright/test";

test("transport carries session CSRF and preserves protocol failures", async ({ page }) => {
  await page.context().addCookies([
    {
      name: "vestibule_console_session",
      value: "transport-session",
      url: "http://127.0.0.1:4173"
    }
  ]);
  await page.route("**/api/transport/**", async (route) => {
    const request = route.request();
    const path = new URL(request.url()).pathname;

    if (path === "/api/transport/write") {
      expect(request.method()).toBe("POST");
      expect(request.headers()["x-csrf-token"]).toBe("transport-csrf");
      expect(request.headers().cookie ?? "").toContain(
        "vestibule_console_session=transport-session"
      );
      expect(JSON.parse(request.postData() ?? "")).toEqual({ expected: "pending" });
      await route.fulfill({
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ accepted: true })
      });
      return;
    }

    if (path === "/api/transport/server-error") {
      await route.fulfill({
        status: 409,
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ error: { code: "challenge_conflict" } })
      });
      return;
    }

    if (path === "/api/transport/not-json") {
      await route.fulfill({
        status: 503,
        headers: { "Content-Type": "text/plain" },
        body: "unavailable"
      });
      return;
    }

    if (path === "/api/transport/invalid-json") {
      await route.fulfill({
        headers: { "Content-Type": "application/json" },
        body: "not-json"
      });
      return;
    }

    if (path === "/api/transport/network") {
      await route.abort("failed");
      return;
    }

    throw new Error(`Unexpected API request: ${request.method()} ${path}`);
  });

  await page.goto("/queue");
  const results = await page.evaluate(async () => {
    // The transport must load in Chromium so this test observes its fetch credentials.
    const { createApiTransport } = await import("/src/lib/api.ts");
    const transport = createApiTransport(() => ({ csrfToken: "transport-csrf" }));
    const write = await transport.request("/api/transport/write", {
      method: "POST",
      body: { expected: "pending" },
      parse: (payload: unknown) => payload
    });
    const serverError = await transport.request("/api/transport/server-error", {
      parse: (payload: unknown) => payload
    });
    const nonJson = await transport.request("/api/transport/not-json", {
      parse: (payload: unknown) => payload
    });
    const invalidJson = await transport.request("/api/transport/invalid-json", {
      parse: (payload: unknown) => payload
    });
    const network = await transport.request("/api/transport/network", {
      parse: (payload: unknown) => payload
    });

    return {
      write: write.ok ? write.data : { kind: write.error.kind },
      serverError: serverError.ok
        ? { kind: "success" }
        : serverError.error.kind === "api"
          ? { kind: serverError.error.kind, code: serverError.error.code }
          : { kind: serverError.error.kind },
      nonJson: nonJson.ok ? { kind: "success" } : { kind: nonJson.error.kind },
      invalidJson: invalidJson.ok ? { kind: "success" } : { kind: invalidJson.error.kind },
      network: network.ok ? { kind: "success" } : { kind: network.error.kind }
    };
  });

  expect(results).toEqual({
    write: { accepted: true },
    serverError: { kind: "api", code: "challenge_conflict" },
    nonJson: { kind: "non-json" },
    invalidJson: { kind: "invalid-json" },
    network: { kind: "network" }
  });
});
