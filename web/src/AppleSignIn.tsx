import { useEffect, useRef, useState } from "react";
import { exchangeAppleToken, fetchClientConfig } from "./api";

// Apple's own script draws the button, so it follows Apple's rules for the logo and wording.
// Loaded only when the server has a web Services ID; with none, nothing is fetched from Apple.
const APPLE_JS = "https://appleid.cdn-apple.com/appleauth/static/jsapi/appleid/1/en_US/appleid.auth.js";

type AppleAuth = { auth: { init: (options: Record<string, unknown>) => void } };
type AppleSuccess = CustomEvent<{ authorization: { id_token: string } }>;
type AppleFailure = CustomEvent<{ error: string }>;

function loadAppleJS(): Promise<AppleAuth> {
  const ready = () => (window as unknown as { AppleID?: AppleAuth }).AppleID;
  return new Promise((resolve, reject) => {
    const existing = ready();
    if (existing) return resolve(existing);
    const script = document.createElement("script");
    script.src = APPLE_JS;
    script.async = true;
    script.onload = () => {
      const apple = ready();
      if (apple) resolve(apple);
      else reject(new Error("Apple sign-in did not load."));
    };
    script.onerror = () => reject(new Error("Apple sign-in could not be reached."));
    document.head.append(script);
  });
}

/** A random string for one sign-in, and its SHA-256 as lowercase hex. Apple gets the hash, the
 * server the string, so a token taken from another sign-in does not verify here. */
async function makeNonce(): Promise<{ raw: string; hashed: string }> {
  const bytes = crypto.getRandomValues(new Uint8Array(32));
  const raw = btoa(String.fromCharCode(...bytes)).replace(/[+/=]/g, (c) => ({ "+": "-", "/": "_" })[c] ?? "");
  const digest = new Uint8Array(await crypto.subtle.digest("SHA-256", new TextEncoder().encode(raw)));
  const hashed = Array.from(digest, (b) => b.toString(16).padStart(2, "0")).join("");
  return { raw, hashed };
}

/** Sign in with Apple (AUTH-001), shown only when `/client-config` names a web Services ID. */
export function AppleSignIn({ onSignIn }: { onSignIn: (token: string) => void }) {
  const [clientId, setClientId] = useState<string | null>(null);
  const [error, setError] = useState("");
  // A ref, so a new onSignIn from the parent does not set Apple up again.
  const signedIn = useRef(onSignIn);
  signedIn.current = onSignIn;

  useEffect(() => {
    void fetchClientConfig().then((c) => setClientId(c.appleWebClientId));
  }, []);

  useEffect(() => {
    if (!clientId) return;
    let raw = "";
    const success = (e: Event) => {
      setError("");
      exchangeAppleToken((e as AppleSuccess).detail.authorization.id_token, raw).then(
        (token) => signedIn.current(token),
        (err: Error) => setError(err.message),
      );
    };
    const failure = (e: Event) => {
      // Closing Apple's window is not an error to show.
      if ((e as AppleFailure).detail?.error !== "popup_closed_by_user") setError("Apple sign-in did not finish. Try again.");
    };
    document.addEventListener("AppleIDSignInOnSuccess", success);
    document.addEventListener("AppleIDSignInOnFailure", failure);
    void Promise.all([loadAppleJS(), makeNonce()])
      .then(([apple, nonce]) => {
        raw = nonce.raw;
        // Draws the button into #appleid-signin. The popup returns to this page; the Services
        // ID lists this origin's "/" as a return URL.
        apple.auth.init({
          clientId,
          scope: "email",
          redirectURI: `${window.location.origin}/`,
          nonce: nonce.hashed,
          usePopup: true,
        });
      })
      .catch((err: Error) => setError(err.message));
    return () => {
      document.removeEventListener("AppleIDSignInOnSuccess", success);
      document.removeEventListener("AppleIDSignInOnFailure", failure);
    };
  }, [clientId]);

  if (!clientId) return null;
  return (
    <div className="apple">
      <div id="appleid-signin" data-color="white" data-border="false" data-type="sign-in" data-height="44" />
      {error && (
        <p className="error" role="alert">
          {error}
        </p>
      )}
      <p className="divider">or with an API token</p>
    </div>
  );
}
