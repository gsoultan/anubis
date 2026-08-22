// Anubis Connect v2 client for the console. One transport, typed service
// clients, automatic bearer attachment and single-flight refresh on expiry.
//
// Dev: vite proxies /anubis.v1.* to :7448 (same-origin, no CORS).
// Prod: same-origin behind the gateway.
import { createClient, type Client, type Interceptor } from "@connectrpc/connect";
import { createConnectTransport } from "@connectrpc/connect-web";
import { AuthService } from "../gen/anubis/v1/auth_pb";
import type { TokenPair } from "../gen/anubis/v1/common_pb";
import { AuthzService } from "../gen/anubis/v1/authz_pb";
import { SessionService } from "../gen/anubis/v1/session_pb";
import { TokenService } from "../gen/anubis/v1/token_pb";
import {
  IdentityAdminService,
  ScopeAdminService,
  AuthzAdminService,
  TenantAdminService,
} from "../gen/anubis/v1/admin_pb";

const STORAGE_KEY = "anubis.tokens";

type StoredTokens = {
  accessToken: string;
  refreshToken: string;
  sessionId: string;
  expiresAt: number; // unix ms
};

let tokens: StoredTokens | null = load();
let refreshing: Promise<void> | null = null;

function load(): StoredTokens | null {
  try {
    const raw = sessionStorage.getItem(STORAGE_KEY);
    return raw ? (JSON.parse(raw) as StoredTokens) : null;
  } catch {
    return null;
  }
}

function store(next: StoredTokens | null) {
  tokens = next;
  if (next) sessionStorage.setItem(STORAGE_KEY, JSON.stringify(next));
  else sessionStorage.removeItem(STORAGE_KEY);
}

export function setTokens(pair: TokenPair) {
  store({
    accessToken: pair.accessToken,
    refreshToken: pair.refreshToken || tokens?.refreshToken || "",
    sessionId: pair.sessionId,
    expiresAt: Date.now() + Number(pair.expiresIn) * 1000,
  });
}

export function clearTokens() {
  store(null);
}

export function isAuthenticated(): boolean {
  return tokens !== null;
}

/** Refresh once even under concurrent callers; theft-detection errors clear
 * the session (the server revoked the family — re-login is the only path). */
async function refreshIfNeeded(): Promise<void> {
  if (!tokens || Date.now() < tokens.expiresAt - 30_000) return;
  if (!refreshing) {
    const rt = tokens.refreshToken;
    refreshing = (async () => {
      try {
        const resp = await bareAuth.refresh({ refreshToken: rt });
        if (resp.tokens) setTokens(resp.tokens);
      } catch (err) {
        clearTokens();
        throw err;
      } finally {
        refreshing = null;
      }
    })();
  }
  await refreshing;
}

const authInterceptor: Interceptor = (next) => async (req) => {
  await refreshIfNeeded();
  if (tokens) req.header.set("Authorization", `Bearer ${tokens.accessToken}`);
  return next(req);
};

const base = createConnectTransport({ baseUrl: "/" });
const authed = createConnectTransport({ baseUrl: "/", interceptors: [authInterceptor] });

/** Unauthenticated client used by the refresh path itself (no interceptor —
 * a refresh must never wait on a refresh). */
const bareAuth = createClient(AuthService, base);

export const api = {
  auth: createClient(AuthService, authed),
  authz: createClient(AuthzService, authed),
  session: createClient(SessionService, authed),
  token: createClient(TokenService, authed),
  identityAdmin: createClient(IdentityAdminService, authed),
  scopeAdmin: createClient(ScopeAdminService, authed),
  authzAdmin: createClient(AuthzAdminService, authed),
  tenantAdmin: createClient(TenantAdminService, authed),
} satisfies Record<string, Client<never> | unknown>;

/** Password sign-in for the console itself. */
export async function login(tenant: string, username: string, password: string) {
  const resp = await api.auth.login({ tenant, username, password, clientId: "console" });
  if (resp.result.case === "tokens" && resp.result.value) {
    setTokens(resp.result.value);
    return { mfa: false as const };
  }
  if (resp.result.case === "mfa") {
    return { mfa: true as const, challenge: resp.result.value };
  }
  throw new Error("unexpected login response");
}

export async function verifyMfa(mfaToken: string, code: string) {
  const resp = await api.auth.verifyMfa({ mfaToken, method: "totp", code });
  if (resp.tokens) setTokens(resp.tokens);
}

export async function logout() {
  try {
    await api.auth.logout({});
  } finally {
    clearTokens();
  }
}
