import { useEffect, useRef, useState } from "react";
import { type ClientConfig, exchangeIdToken, fetchClientConfig } from "./api";

// Each provider's own script draws its button, so the logo and wording follow its rules. A
// script is loaded only when the server has that provider's client id; with none, nothing is
// fetched from Apple or Google.
const APPLE_JS = "https://appleid.cdn-apple.com/appleauth/static/jsapi/appleid/1/en_US/appleid.auth.js";
const GOOGLE_JS = "https://accounts.google.com/gsi/client";

type AppleAuth = { auth: { init: (options: Record<string, unknown>) => void } };
type AppleSuccess = CustomEvent<{ authorization: { id_token: string } }>;
type AppleFailure = CustomEvent<{ error: string }>;
type GoogleID = {
  accounts: {
    id: {
      initialize: (options: Record<string, unknown>) => void;
      renderButton: (parent: HTMLElement, options: Record<string, unknown>) => void;
    };
  };
};

function loadScript<T>(src: string, global: string): Promise<T> {
  const ready = () => (window as unknown as Record<string, T | undefined>)[global];
  return new Promise((resolve, reject) => {
    const existing = ready();
    if (existing) return resolve(existing);
    const script = document.createElement("script");
    script.src = src;
    script.async = true;
    script.onload = () => {
      const loaded = ready();
      if (loaded) resolve(loaded);
      else reject(new Error("Sign-in did not load."));
    };
    script.onerror = () => reject(new Error("Sign-in could not be reached."));
    document.head.append(script);
  });
}

/** A random string for one sign-in, and its SHA-256 as lowercase hex. The provider gets the hash,
 * the server the string, so a token taken from another sign-in does not verify here. */
async function makeNonce(): Promise<{ raw: string; hashed: string }> {
  const hex = (bytes: Uint8Array) => Array.from(bytes, (b) => b.toString(16).padStart(2, "0")).join("");
  const raw = hex(crypto.getRandomValues(new Uint8Array(32)));
  return { raw, hashed: hex(new Uint8Array(await crypto.subtle.digest("SHA-256", new TextEncoder().encode(raw)))) };
}

type Props = { onSignIn: (token: string) => void; onError: (message: string) => void };

/** Sign in with Apple (AUTH-001). */
function AppleButton({ clientId, onSignIn, onError }: Props & { clientId: string }) {
  const handlers = useRef({ onSignIn, onError });
  handlers.current = { onSignIn, onError };

  useEffect(() => {
    let raw = "";
    const success = (e: Event) => {
      exchangeIdToken("apple", (e as AppleSuccess).detail.authorization.id_token, raw).then(
        (token) => handlers.current.onSignIn(token),
        (err: Error) => handlers.current.onError(err.message),
      );
    };
    const failure = (e: Event) => {
      // Closing Apple's window is not an error to show.
      if ((e as AppleFailure).detail?.error !== "popup_closed_by_user")
        handlers.current.onError("Apple sign-in did not finish. Try again.");
    };
    document.addEventListener("AppleIDSignInOnSuccess", success);
    document.addEventListener("AppleIDSignInOnFailure", failure);
    void Promise.all([loadScript<AppleAuth>(APPLE_JS, "AppleID"), makeNonce()])
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
      .catch((err: Error) => handlers.current.onError(err.message));
    return () => {
      document.removeEventListener("AppleIDSignInOnSuccess", success);
      document.removeEventListener("AppleIDSignInOnFailure", failure);
    };
  }, [clientId]);

  return <div id="appleid-signin" data-color="white" data-border="false" data-type="sign-in" data-height="40" />;
}

/** Sign in with Google (AUTH-002), through Google Identity Services. */
function GoogleButton({ clientId, onSignIn, onError }: Props & { clientId: string }) {
  const handlers = useRef({ onSignIn, onError });
  handlers.current = { onSignIn, onError };
  const slot = useRef<HTMLDivElement>(null);

  useEffect(() => {
    void Promise.all([loadScript<GoogleID>(GOOGLE_JS, "google"), makeNonce()])
      .then(([google, nonce]) => {
        google.accounts.id.initialize({
          client_id: clientId,
          nonce: nonce.hashed,
          callback: (response: { credential: string }) => {
            exchangeIdToken("google", response.credential, nonce.raw).then(
              (token) => handlers.current.onSignIn(token),
              (err: Error) => handlers.current.onError(err.message),
            );
          },
        });
        if (slot.current) {
          // Google takes a width in pixels; 375 at most, as wide as Apple's button gets.
          const width = Math.min(375, Math.round(slot.current.getBoundingClientRect().width));
          // Google's defaults are the ones wanted: standard, outline (white like Apple's), large,
          // "Sign in with Google", rectangular.
          google.accounts.id.renderButton(slot.current, {
            width,
            // English, as Apple's script is and the rest of the page.
            locale: "en",
          });
        }
      })
      .catch((err: Error) => handlers.current.onError(err.message));
  }, [clientId]);

  return <div className="google" ref={slot} />;
}

/** The provider buttons, each shown only when `/client-config` names its client id. */
export function ProviderSignIn({ onSignIn }: { onSignIn: (token: string) => void }) {
  const [config, setConfig] = useState<ClientConfig | null>(null);
  const [error, setError] = useState("");

  useEffect(() => {
    void fetchClientConfig().then(setConfig);
  }, []);

  if (!config?.appleWebClientId && !config?.googleWebClientId) return null;
  const props = { onSignIn, onError: setError };
  return (
    <div className="providers">
      {config.appleWebClientId && <AppleButton clientId={config.appleWebClientId} {...props} />}
      {config.googleWebClientId && <GoogleButton clientId={config.googleWebClientId} {...props} />}
      {error && (
        <p className="error" role="alert">
          {error}
        </p>
      )}
      <p className="divider">or with an API token</p>
    </div>
  );
}
