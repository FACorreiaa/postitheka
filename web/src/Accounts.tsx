import { type Ref, useState } from "react";
import { type Account, api, messageOf } from "./api";
import { platformNames } from "./rules";

interface Props {
  ref: Ref<HTMLDialogElement>;
  accounts: Account[];
  onChanged: () => Promise<void>;
}

export function Accounts({ ref, accounts, onChanged }: Props) {
  const [url, setUrl] = useState("");
  const [name, setName] = useState("");
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);

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
