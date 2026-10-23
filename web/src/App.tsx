import { useCallback, useEffect, useRef, useState } from "react";
import { Accounts, type ConnectOutcome } from "./Accounts";
import { type Account, api, messageOf, type PlatformInfo, type Post, readToken, setUnauthorizedHandler, writeToken } from "./api";
import lockupDark from "./assets/lockup-horizontal-on-dark.svg";
import lockupLight from "./assets/lockup-horizontal-on-light.svg";
import { Composer } from "./Composer";
import { platformNames } from "./rules";
import { Sent } from "./Sent";

// The lockup's colours are baked into each file, so the theme picks the file.
// Sources live in brand/; these are copies.
function Logo() {
  return (
    <picture>
      <source srcSet={lockupDark} media="(prefers-color-scheme: dark)" />
      <img className="logo" src={lockupLight} alt="Postitheka" />
    </picture>
  );
}

export function App() {
  const [token, setToken] = useState(readToken);

  const signOut = useCallback(() => {
    writeToken(null);
    setToken(null);
  }, []);
  useEffect(() => setUnauthorizedHandler(signOut), [signOut]);

  if (!token) {
    return (
      <SignIn
        onSignIn={(t) => {
          writeToken(t);
          setToken(t);
        }}
      />
    );
  }
  return <Workspace onSignOut={signOut} />;
}

function SignIn({ onSignIn }: { onSignIn: (token: string) => void }) {
  const [value, setValue] = useState("");
  const [error, setError] = useState("");

  return (
    <main className="signin">
      <h1 className="wordmark">
        <Logo />
      </h1>
      <p className="lede">Write once. Post everywhere you’re connected.</p>
      <form
        onSubmit={async (e) => {
          e.preventDefault();
          const token = value.trim();
          writeToken(token);
          const { error: err, response } = await api.GET("/v1/accounts");
          if (response.ok) return onSignIn(token);
          writeToken(null);
          setError(response.status === 401 ? "That token isn’t recognised. Mint a new one and paste it again." : messageOf(err));
        }}
      >
        <label htmlFor="token">API token</label>
        <input
          id="token"
          type="password"
          autoComplete="off"
          spellCheck={false}
          placeholder="pk_…"
          value={value}
          onChange={(e) => setValue(e.target.value)}
          aria-describedby="token-help"
          required
        />
        <p id="token-help" className="help">
          Mint one with <code>task user:create -- you@example.com</code> in the server’s directory.
        </p>
        {error && (
          <p className="error" role="alert">
            {error}
          </p>
        )}
        <button className="primary" type="submit">
          Sign in
        </button>
      </form>
    </main>
  );
}

function Workspace({ onSignOut }: { onSignOut: () => void }) {
  const [accounts, setAccounts] = useState<Account[] | null>(null);
  const [platforms, setPlatforms] = useState<PlatformInfo[]>([]);
  const [posts, setPosts] = useState<Post[] | null>(null);
  const [loadError, setLoadError] = useState("");
  const accountsDialog = useRef<HTMLDialogElement>(null);
  const [outcome] = useState(readConnectOutcome);

  // Back from a platform's consent page: show how it went, then drop the
  // query so a reload does not show it again.
  useEffect(() => {
    if (!outcome) return;
    window.history.replaceState(null, "", window.location.pathname);
    accountsDialog.current?.showModal();
  }, [outcome]);

  const loadAccounts = useCallback(async () => {
    const { data, error } = await api.GET("/v1/accounts");
    if (data) setAccounts(data);
    else setLoadError(messageOf(error));
  }, []);

  const loadPosts = useCallback(async () => {
    const { data } = await api.GET("/v1/posts", { params: { query: { limit: 30 } } });
    if (data) setPosts(data);
  }, []);

  useEffect(() => {
    void loadAccounts();
    void loadPosts();
    void api.GET("/v1/platforms").then(({ data }) => data && setPlatforms(data));
  }, [loadAccounts, loadPosts]);

  // Poll only while something is still on its way. A finished history has
  // nothing to refresh, and an idle tab should not keep the server busy.
  // Not handing_off: that waits on a phone, which may never report back, and
  // polling for it would never stop.
  const inFlight = posts?.some((p) => p.targets.some((t) => t.status === "pending" || t.status === "publishing"));
  useEffect(() => {
    if (!inFlight) return;
    const id = setInterval(() => void loadPosts(), 2000);
    return () => clearInterval(id);
  }, [inFlight, loadPosts]);

  return (
    <>
      <header className="bar">
        <span className="wordmark">
          <Logo />
        </span>
        <nav>
          <button className="quiet" onClick={() => accountsDialog.current?.showModal()}>
            Accounts{accounts && accounts.length > 0 ? ` (${accounts.length})` : ""}
          </button>
          <button className="quiet" onClick={onSignOut}>
            Sign out
          </button>
        </nav>
      </header>

      {loadError && (
        <p className="error banner" role="alert">
          {loadError}
        </p>
      )}

      <main className="desk">
        <Composer
          accounts={accounts}
          platforms={platforms}
          onConnect={() => accountsDialog.current?.showModal()}
          onPublished={(post) => setPosts((prev) => [post, ...(prev ?? [])])}
        />
        <Sent posts={posts} accounts={accounts ?? []} />
      </main>

      <Accounts
        ref={accountsDialog}
        accounts={accounts ?? []}
        platforms={platforms}
        outcome={outcome}
        onChanged={loadAccounts}
      />
    </>
  );
}

// The redirect names no platform on failure: the server only knows the
// outcome, and the platform's own message never travels in a URL.
const connectFailed = "Not connected: the platform did not accept the sign-in. Try again.";
const connectFailures: Record<string, string> = {
  cancelled: "Not connected: access was not granted on the platform’s page.",
  expired: "Not connected: the sign-in page was open too long. Try again.",
  failed: connectFailed,
};

function readConnectOutcome(): ConnectOutcome | null {
  const q = new URLSearchParams(window.location.search);
  const connected = q.get("connected");
  if (connected) return { ok: true, text: `${platformNames[connected] ?? connected} connected.` };
  const failure = q.get("connect_error");
  if (failure) return { ok: false, text: connectFailures[failure] ?? connectFailed };
  return null;
}
