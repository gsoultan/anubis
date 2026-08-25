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
  ProvisioningService,
  PlatformAdminService,
  PlatformAuthService,
} from "../gen/anubis/v1/admin_pb";

const STORAGE_KEY = "anubis.tokens";
/* The platform token is kept beside the active one. Entering a tenant swaps
   the active token for a tenant-scoped one; without holding the original,
   leaving the tenant again would mean signing in from scratch. */


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

type SessionListener = () => void;
const listeners = new Set<SessionListener>();

/** Subscribe to session changes. The console cannot get away with only
 * observing its own sign-in and sign-out calls: refreshIfNeeded clears the
 * session by itself when the server reports a stolen refresh token, and a
 * shell still painted over a dead session is how an operator ends up
 * clicking through screens that will every one of them fail. */
export function onSessionChange(fn: SessionListener): () => void {
  listeners.add(fn);
  return () => {
    listeners.delete(fn);
  };
}

function store(next: StoredTokens | null) {
  const had = tokens !== null;
  tokens = next;
  if (next) sessionStorage.setItem(STORAGE_KEY, JSON.stringify(next));
  else sessionStorage.removeItem(STORAGE_KEY);
  if (had !== (next !== null)) for (const fn of listeners) fn();
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
  sessionStorage.removeItem(TENANT_KEY);
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

/* Admin calls need the working-tenant header, but on a fresh session the
   default tenant is only chosen once MyTenants answers. Shell components
   mount the moment tokens exist and their queries used to race that choice —
   a wall of headerless requests failing right after sign-in. This gate holds
   tenant-scoped calls at ONE choke point until the tenant is resolved, or
   provably unresolvable (no tenants assigned), or 5s passes (never deadlock
   the console over a nicety). PlatformAuth calls bypass it: MyTenants is
   what RESOLVES the gate and must never wait on it. */
let tenantGate: Promise<void> | null = null;
let openTenantGate: (() => void) | null = null;

function tenantResolved(): void {
  openTenantGate?.();
  openTenantGate = null;
  tenantGate = null;
}

function awaitTenant(): Promise<void> {
  if (currentTenant()) return Promise.resolve();
  if (!tenantGate) {
    tenantGate = new Promise((resolve) => {
      openTenantGate = resolve;
      setTimeout(resolve, 5000);
    });
  }
  return tenantGate;
}

const authInterceptor: Interceptor = (next) => async (req) => {
  await refreshIfNeeded();
  if (!req.service.typeName.startsWith("anubis.v1.PlatformAuth")) {
    await awaitTenant();
  }
  if (tokens) req.header.set("Authorization", `Bearer ${tokens.accessToken}`);
  const tenant = currentTenant();
  if (tenant) req.header.set("X-Anubis-Tenant", tenant);
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
  provisioning: createClient(ProvisioningService, authed),
  platformAdmin: createClient(PlatformAdminService, authed),
  /* Two clients for one service, on purpose. PlatformLogin has to work with
     no token — that is the point of it — while MyTenants is a session call
     and is meaningless without one. Putting the whole service on the
     unauthenticated transport is what made the tenant picker render nothing:
     the call failed, and a failed query looks exactly like "no tenants". */
  platformAuth: createClient(PlatformAuthService, base),
  platformSession: createClient(PlatformAuthService, authed),
} satisfies Record<string, Client<never> | unknown>;

/** Sign in a PLATFORM USER — the console's own door.
 *
 * Not AuthService.Login: that resolves a tenant identity through a realm, and
 * an operator is deliberately not one. A tenant's people sign in through
 * their own page instead (the sign-in page builder), never here. */
export async function platformLogin(username: string, password: string) {
  const resp = await api.platformAuth.platformLogin({ username, password });
  // An operator with a second factor gets a challenge, not a session: a
  // password alone is not enough for an account that runs the installation.
  if (resp.mfaToken) return { mfa: true as const, mfaToken: resp.mfaToken, username: resp.username };
  store({
    accessToken: resp.accessToken,
    // Platform tokens have no refresh yet: the console asks for the password
    // again rather than holding a long-lived operator credential.
    refreshToken: "",
    sessionId: "",
    expiresAt: Date.now() + resp.expiresIn * 1000,
  });
  return { mfa: false as const, username: resp.username, owner: resp.owner };
}

/** Complete a challenge. Only this turns a password into a session. */
export async function platformVerifyMfa(mfaToken: string, code: string) {
  const resp = await api.platformAuth.platformVerifyMfa({ mfaToken, code });
  store({
    accessToken: resp.accessToken,
    refreshToken: "",
    sessionId: "",
    expiresAt: Date.now() + resp.expiresIn * 1000,
  });
  return { username: resp.username, owner: resp.owner };
}

export async function beginTotpEnrolment() {
  const resp = await api.platformSession.beginTotpEnrolment({});
  return { secret: resp.secret, uri: resp.uri };
}

export async function confirmTotpEnrolment(code: string) {
  await api.platformSession.confirmTotpEnrolment({ code });
}


/** Which tenants the signed-in operator may administer. */
export async function myTenants() {
  const resp = await api.platformSession.myTenants({});
  const tenants = resp.tenants.map((t) => ({ slug: t.slug, name: t.name, role: t.role, all: t.all }));
  // An empty answer is also an answer: a fresh installation has no tenants
  // to select, and held requests must proceed to their clean refusals
  // rather than wait out the gate's timeout.
  if (tenants.length === 0) tenantResolved();
  return tenants;
}

/* The tenant an operator is working in. It travels as a request header and is
   checked against their assignments on EVERY call, so revoking somebody's
   access to a tenant takes effect immediately rather than whenever a token
   happens to expire. Nothing here grants anything — it only says which
   tenant is being asked about. */
const TENANT_KEY = "anubis.tenant";

export function currentTenant(): string {
  return sessionStorage.getItem(TENANT_KEY) ?? "";
}

export function setCurrentTenant(slug: string) {
  if (slug) sessionStorage.setItem(TENANT_KEY, slug);
  else sessionStorage.removeItem(TENANT_KEY);
  if (slug) tenantResolved();
  for (const fn of listeners) fn();
}
