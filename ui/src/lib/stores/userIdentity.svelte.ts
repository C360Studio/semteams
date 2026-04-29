// Lightweight client identity store. Used by agentApi to populate the
// X-User-Id header on identity-aware backend calls (currently
// POST /teams-dispatch/loops/{id}/approval per semstreams beta.22).
//
// This is NOT authentication. The header is a plaintext claim — anyone
// with browser access can change it. Real auth (OAuth/JWT/mTLS) belongs
// in a follow-on layer that runs outside this store and overwrites the
// header before it reaches the backend. ADR-030 Phase 3 upstream tracks
// the bypass-token threat model.

const STORAGE_KEY = "semteams.userIdentity";
const DEFAULT_IDENTITY = "ui-anonymous";

// browser tells us whether we're in SSR (no window/localStorage) or
// client-side rendering. Mirrors SvelteKit's $app/environment guard
// without the import (this module is consumed from non-SvelteKit
// contexts too — vitest, node-side test runners).
const browser = typeof window !== "undefined" && typeof localStorage !== "undefined";

function readStored(): string {
  if (!browser) return DEFAULT_IDENTITY;
  try {
    const raw = localStorage.getItem(STORAGE_KEY);
    return raw && raw.trim() !== "" ? raw : DEFAULT_IDENTITY;
  } catch {
    // Private browsing / disabled storage — fall back to the default.
    return DEFAULT_IDENTITY;
  }
}

function writeStored(value: string): void {
  if (!browser) return;
  try {
    if (value === DEFAULT_IDENTITY) {
      localStorage.removeItem(STORAGE_KEY);
    } else {
      localStorage.setItem(STORAGE_KEY, value);
    }
  } catch {
    // Best-effort; the in-memory state still updates.
  }
}

function createUserIdentityStore() {
  let value = $state(readStored());

  return {
    get value(): string {
      return value;
    },
    /**
     * Update the identity. Empty / whitespace-only values reset to
     * the default. The backend's xUserIDIdentityMiddleware sanitises
     * non-printable characters, so any string a user can type via the
     * keyboard is safe to pass through.
     */
    set(next: string): void {
      const trimmed = next.trim();
      const v = trimmed === "" ? DEFAULT_IDENTITY : trimmed;
      value = v;
      writeStored(v);
    },
    /** Reset to the default ("ui-anonymous") and clear localStorage. */
    reset(): void {
      value = DEFAULT_IDENTITY;
      writeStored(DEFAULT_IDENTITY);
    },
  };
}

export const userIdentity = createUserIdentityStore();

export const USER_IDENTITY_DEFAULT = DEFAULT_IDENTITY;
export const USER_IDENTITY_STORAGE_KEY = STORAGE_KEY;
