export type ApiResult<T> =
  | Readonly<{ ok: true; data: T }>
  | Readonly<{ ok: false; error: ApiRequestError }>;

export type ApiRequestError =
  | ApiError
  | NetworkError
  | NonJsonResponseError
  | JsonParseError
  | InvalidPayloadError;

export type ApiRequestMethod = "GET" | "POST" | "PUT" | "PATCH" | "DELETE";

export type JsonPayloadParser<T> = (payload: unknown) => T | undefined;

export type ApiRequestOptions<T> = Readonly<{
  method?: ApiRequestMethod;
  body?: unknown;
  parse: JsonPayloadParser<T>;
}>;

export type CsrfSession = Readonly<{
  csrfToken: string;
}>;

export interface ApiTransport {
  request<T>(path: string, options: ApiRequestOptions<T>): Promise<ApiResult<T>>;
}

export class ApiError extends Error {
  readonly kind = "api" as const;

  constructor(
    readonly code: string,
    readonly status: number
  ) {
    super(`API request failed with ${code}`);
    this.name = "ApiError";
  }
}

export class NetworkError extends Error {
  readonly kind = "network" as const;

  constructor(readonly cause: unknown) {
    super("Network request failed");
    this.name = "NetworkError";
  }
}

export class NonJsonResponseError extends Error {
  readonly kind = "non-json" as const;

  constructor(
    readonly status: number,
    readonly contentType: string | null
  ) {
    super("API response was not JSON");
    this.name = "NonJsonResponseError";
  }
}

export class JsonParseError extends Error {
  readonly kind = "invalid-json" as const;

  constructor(
    readonly status: number,
    readonly cause: unknown
  ) {
    super("API response contained invalid JSON");
    this.name = "JsonParseError";
  }
}

export class InvalidPayloadError extends Error {
  readonly kind = "invalid-payload" as const;

  constructor(readonly status: number) {
    super("API response did not match its contract");
    this.name = "InvalidPayloadError";
  }
}

function errorCode(payload: unknown): string | undefined {
  if (
    typeof payload !== "object" ||
    payload === null ||
    Array.isArray(payload) ||
    !("error" in payload) ||
    typeof payload.error !== "object" ||
    payload.error === null ||
    Array.isArray(payload.error) ||
    !("code" in payload.error) ||
    typeof payload.error.code !== "string"
  ) {
    return undefined;
  }

  return payload.error.code;
}

export function createApiTransport(
  readSession: () => CsrfSession | undefined
): ApiTransport {
  return {
    async request<T>(path: string, options: ApiRequestOptions<T>): Promise<ApiResult<T>> {
      const method = options.method ?? "GET";
      const headers = new Headers({ Accept: "application/json" });
      let body: string | undefined;

      if (options.body !== undefined) {
        headers.set("Content-Type", "application/json");
        body = JSON.stringify(options.body);
      }

      if (method !== "GET") {
        const csrfToken = readSession()?.csrfToken;
        if (csrfToken) {
          headers.set("X-CSRF-Token", csrfToken);
        }
      }

      let response: Response;
      try {
        response = await fetch(path, {
          method,
          headers,
          body,
          credentials: "same-origin"
        });
      } catch (cause) {
        return { ok: false, error: new NetworkError(cause) };
      }

      if (
        response.headers.get("Content-Type")?.split(";", 1)[0]?.trim().toLowerCase() !==
        "application/json"
      ) {
        return {
          ok: false,
          error: new NonJsonResponseError(response.status, response.headers.get("Content-Type"))
        };
      }

      let responseText: string;
      try {
        responseText = await response.text();
      } catch (cause) {
        return { ok: false, error: new NetworkError(cause) };
      }

      let payload: unknown;
      try {
        payload = JSON.parse(responseText) as unknown;
      } catch (cause) {
        return { ok: false, error: new JsonParseError(response.status, cause) };
      }

      if (!response.ok) {
        const code = errorCode(payload);
        return {
          ok: false,
          error: code
            ? new ApiError(code, response.status)
            : new InvalidPayloadError(response.status)
        };
      }

      let parsed: T | undefined;
      try {
        parsed = options.parse(payload);
      } catch {
        return { ok: false, error: new InvalidPayloadError(response.status) };
      }

      return parsed === undefined
        ? { ok: false, error: new InvalidPayloadError(response.status) }
        : { ok: true, data: parsed };
    }
  };
}
