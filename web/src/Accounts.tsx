import { type Ref, useState } from "react";
import { type Account, api, messageOf, type PlatformInfo } from "./api";
import { platformNames } from "./rules";

/** What came back from a platform's consent page, read from the URL. */
export interface ConnectOutcome {
  ok: boolean;
  text: string;
}

interface Props {
  ref: Ref<HTMLDialogElement>;
  accounts: Account[];
  platforms: PlatformInfo[];
  outcome: ConnectOutcome | null;
  onChanged: () => Promise<void>;
}

export function Accounts({ ref, accounts, platforms, outcome, onChanged }: Props) {
  const [url, setUrl] = useState("");
  const [name, setName] = useState("");
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  const [leaving, setLeaving] = useState("");
  const oauth = platforms.filter((p) => p.connect === "oauth");

  // The platform's consent page replaces this one; the server sends the user
  // back to /?connected=… when it is done.
  async function connectOAuth(platform: PlatformInfo["platform"]) {
    setLeaving(platform);
    setError("");
    const { data, error: err } = await api.POST("/v1/accounts/{platform}/connect", {
      params: { path: { platform } },
      body: { client: "web" },
    });
    if (!data) {
      setLeaving("");
      setError(messageOf(err));
      return;
    }
    window.location.assign(data.authorize_url);
  }

  async function connect(e: React.FormEvent) {
    e.preventDefault();
    setBusy(true);
    setError("");
    const { error: err } = await api.POST("/v1/accounts/discord", {
      body: { webhook_url: url.trim(), display_name: name.trim() || undefined },
    });
    setBusy(false);
    if (err) {
      setError(messageOf(err));
      return;
    }
    setUrl("");
    setName("");
    await onChanged();
  }

  async function disconnect(id: string) {
    const { error: err } = await api.DELETE("/v1/accounts/{accountId}", { params: { path: { accountId: id } } });
    if (err) setError(messageOf(err));
    await onChanged();
  }

  return (
    <dialog ref={ref} className="accounts" aria-labelledby="accounts-h">
      <header>
        <h2 id="accounts-h">Accounts</h2>
        <form method="dialog">
          <button className="quiet" aria-label="Close">
            Done
          </button>
        </form>
      </header>

      {accounts.length > 0 && (
        <ul className="account-list">
          {accounts.map((a) => (
            <li key={a.id}>
              <span>
                <strong>{a.display_name}</strong>
                <span className="help">{platformNames[a.platform] ?? a.platform}</span>
              </span>
              <button className="quiet danger" onClick={() => void disconnect(a.id)}>
                Disconnect
              </button>
            </li>
          ))}
        </ul>
      )}

      {outcome && (
        <p className={outcome.ok ? "notice" : "error"} role="status">
          {outcome.text}
        </p>
      )}

      {oauth.length > 0 && (
        <section className="connect">
          <h3>Connect an account</h3>
          <p className="help">You sign in on the platform’s own page; Postitheka never sees your password.</p>
          <div className="connect-buttons">
            {oauth.map((p) => (
              <button
                key={p.platform}
                type="button"
                className="secondary"
                disabled={leaving !== ""}
                onClick={() => void connectOAuth(p.platform)}
              >
                {leaving === p.platform ? "Opening…" : `Connect ${platformNames[p.platform] ?? p.platform}`}
              </button>
            ))}
          </div>
        </section>
      )}

      <form className="connect" onSubmit={connect}>
        <h3>Connect a Discord channel</h3>
        <p className="help">
          In Discord, open the channel’s settings, then Integrations, Webhooks, New Webhook, and Copy Webhook URL.
        </p>
        <label htmlFor="webhook">Webhook URL</label>
        <input
          id="webhook"
          type="url"
          required
          placeholder="https://discord.com/api/webhooks/…"
          value={url}
          onChange={(e) => setUrl(e.target.value)}
        />
        <label htmlFor="webhook-name">Name it (optional)</label>
        <input
          id="webhook-name"
          placeholder="Uses the webhook’s name if blank"
          value={name}
          onChange={(e) => setName(e.target.value)}
        />
        {error && (
          <p className="error" role="alert">
            {error}
          </p>
        )}
        <button className="primary" type="submit" disabled={busy}>
          {busy ? "Connecting…" : "Connect"}
        </button>
      </form>
    </dialog>
  );
}
