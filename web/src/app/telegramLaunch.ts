function removeTelegramParameters(parameters: URLSearchParams): boolean {
  let changed = false;
  for (const key of [...parameters.keys()]) {
    if (key.startsWith("tgWebApp")) {
      parameters.delete(key);
      changed = true;
    }
  }
  return changed;
}

function captureTelegramLaunchData(): string | undefined {
  const url = new URL(window.location.href);
  const fragment = new URLSearchParams(url.hash.slice(1));
  const initData = fragment.get("tgWebAppData") || url.searchParams.get("tgWebAppData") || undefined;
  const fragmentChanged = removeTelegramParameters(fragment);
  const queryChanged = removeTelegramParameters(url.searchParams);
  if (fragmentChanged) {
    url.hash = fragment.toString();
  }
  if (fragmentChanged || queryChanged) {
    window.history.replaceState(window.history.state, "", url);
  }
  return initData;
}

// Capture before router initialization can replace the launch URL.
export const telegramLaunchInitData = captureTelegramLaunchData();
