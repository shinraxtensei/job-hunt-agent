// Per-browser preferences. localStorage can be missing or throw (private mode, blocked
// storage): every access is wrapped, and the page works without it.
export function readPref(key: string): string | null {
  try {
    return window.localStorage.getItem(key);
  } catch {
    return null;
  }
}

export function writePref(key: string, value: string): void {
  try {
    window.localStorage.setItem(key, value);
  } catch {
    // Not stored: the choice still applies until the page is reloaded.
  }
}
