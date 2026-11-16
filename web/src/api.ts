import createClient, { type Middleware } from "openapi-fetch";
import type { components, paths } from "./schema";

// Generated from api/openapi.yaml (`npm run api:gen`); the server is generated
// from the same file, so a contract change breaks both builds, not production.
export type Account = components["schemas"]["Account"];
export type PlatformInfo = components["schemas"]["PlatformInfo"];
export type Rules = components["schemas"]["Rules"];
export type Post = components["schemas"]["Post"];
export type Target = components["schemas"]["Target"];
export type Issue = components["schemas"]["Issue"];

const TOKEN_KEY = "postitheka.token";

export function readToken(): string | null {
  try {
    return localStorage.getItem(TOKEN_KEY);
  } catch {
    return null;
  }
}

export function writeToken(token: string | null) {
  try {
    if (token) localStorage.setItem(TOKEN_KEY, token);
    else localStorage.removeItem(TOKEN_KEY);
  } catch {
    // Private mode without storage: the token lasts for this page only.
  }
}

export const api = createClient<paths>({ baseUrl: "" });

let onUnauthorized: () => void = () => {};
export function setUnauthorizedHandler(fn: () => void) {
  onUnauthorized = fn;
}

const auth: Middleware = {
  onRequest({ request }) {
    const token = readToken();
    if (token) request.headers.set("Authorization", `Bearer ${token}`);
    return request;
  },
  onResponse({ response }) {
    if (response.status === 401) onUnauthorized();
    return response;
  },
};
api.use(auth);

/** The server's error message, or a generic one when it sent none. */
export function messageOf(error: unknown): string {
  if (error && typeof error === "object" && "message" in error && typeof error.message === "string") {
    return error.message;
  }
  return "The server did not respond. Check that it is running.";
}

/**
 * Uploads a file straight to storage with the presigned URL the API returned.
 * XHR rather than fetch because fetch still reports no upload progress.
 */
export function putFile(
  url: string,
  headers: Record<string, string>,
  file: File,
  onProgress: (fraction: number) => void,
): Promise<void> {
  return new Promise((resolve, reject) => {
    const xhr = new XMLHttpRequest();
    xhr.open("PUT", url);
    for (const [k, v] of Object.entries(headers)) xhr.setRequestHeader(k, v);
    xhr.upload.onprogress = (e) => {
      if (e.lengthComputable) onProgress(e.loaded / e.total);
    };
    xhr.onload = () =>
      xhr.status >= 200 && xhr.status < 300 ? resolve() : reject(new Error(`Upload failed (${xhr.status})`));
    xhr.onerror = () => reject(new Error("Upload failed: storage could not be reached"));
    xhr.send(file);
  });
}

/** What `GET /client-config` says: the 3D mascot's model URL (BRD-001), the web Services ID for
 * Sign in with Apple (AUTH-001) and the web client id for Sign in with Google (AUTH-002), each
 * null when unset. Outside the spec, since every spec
 * route needs a token; nothing in it is secret. Fetched once per page; a server that does not
 * answer reads as everything unset, since nothing in it is worth delaying sign-in for. */
export type ClientConfig = {
  mascot3dModelUrl: string | null;
  appleWebClientId: string | null;
  googleWebClientId: string | null;
};

let clientConfig: Promise<ClientConfig> | null = null;

export function fetchClientConfig(): Promise<ClientConfig> {
  clientConfig ??= fetch("/client-config")
    .then((response) => response.json())
    .then((body: Partial<ClientConfig>) => ({
      mascot3dModelUrl: body.mascot3dModelUrl || null,
      appleWebClientId: body.appleWebClientId || null,
      googleWebClientId: body.googleWebClientId || null,
    }))
    .catch(() => ({ mascot3dModelUrl: null, appleWebClientId: null, googleWebClientId: null }));
  return clientConfig;
}

export function fetchMascot3dModelUrl(): Promise<string | null> {
  return fetchClientConfig().then((c) => c.mascot3dModelUrl);
}

/** Trades an Apple or Google ID token for an API token at `POST /auth/apple` or
 * `POST /auth/google`, outside the spec for the same reason as `/client-config`. `nonce` is the
 * raw one; the provider was given its hash. Resolves to the token, or rejects with a message
 * written for people. */
export async function exchangeIdToken(provider: "apple" | "google", idToken: string, nonce: string): Promise<string> {
  let response: Response;
  try {
    response = await fetch(`/auth/${provider}`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ id_token: idToken, nonce, client: "web" }),
    });
  } catch {
    throw new Error("The server did not respond. Check your connection.");
  }
  const body = (await response.json().catch(() => ({}))) as { token?: string; message?: string };
  if (response.ok && body.token) return body.token;
  throw new Error(body.message || `Sign-in failed (${response.status}). Try again.`);
}
