import type { ApiRequestError } from "../../lib/api";

const errorMessageKeys: Readonly<Record<string, string>> = {
  authentication_expired: "diagnostics.errors.authenticationExpired",
  authentication_invalid: "diagnostics.errors.authenticationInvalid",
  diagnostics_unavailable: "diagnostics.errors.unavailable"
};

export function diagnosticsErrorMessageKey(error: ApiRequestError, fallback = "diagnostics.errors.loadUnavailable"): string {
  if (error.kind === "network") {
    return "diagnostics.errors.network";
  }
  if (error.kind === "api") {
    return errorMessageKeys[error.code] ?? fallback;
  }
  return "diagnostics.errors.invalidResponse";
}
