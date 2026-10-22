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
