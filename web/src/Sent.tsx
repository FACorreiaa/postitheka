import { useState } from "react";
import { type Account, api, messageOf, type Post, type Target } from "./api";
import mascot from "./assets/mascot-256.webp";
import { platformNames } from "./rules";

const time = new Intl.DateTimeFormat(undefined, { hour: "2-digit", minute: "2-digit" });
const day = new Intl.DateTimeFormat(undefined, { day: "numeric", month: "short" });
const full = new Intl.DateTimeFormat(undefined, { dateStyle: "medium", timeStyle: "short" });

export function Sent({
  posts,
  accounts,
  onChanged,
}: {
  posts: Post[] | null;
  accounts: Account[];
  /** A post came back changed (a target sent again). */
  onChanged: (post: Post) => void;
}) {
  const names = new Map(accounts.map((a) => [a.id, a.display_name]));

  return (
    <section className="sent" aria-labelledby="sent-h">
      <h2 id="sent-h">Sent</h2>
      {posts === null ? (
        <p className="help">Loading…</p>
      ) : posts.length === 0 ? (
        <div className="sent-empty">
          <img src={mascot} alt="" width={285} height={256} />
          <p className="help">Posts you publish appear here, with where each one landed.</p>
        </div>
      ) : (
        <ol className="history">
          {posts.map((p) => (
            <li key={p.id} className="letter">
              <p className="excerpt">{p.text || <em>Media only</em>}</p>
              <p className="meta">
                {p.media_ids.length > 0 && `${p.media_ids.length} ${p.media_ids.length === 1 ? "file" : "files"}, `}
                {new Date(p.scheduled_at) > new Date()
                  ? `scheduled for ${full.format(new Date(p.scheduled_at))}`
                  : full.format(new Date(p.created_at))}
              </p>
              <ul className="targets">
                {p.targets.map((t) => (
                  <TargetRow
                    key={t.id}
                    target={t}
                    name={t.account_id ? names.get(t.account_id) : undefined}
                    onChanged={onChanged}
                  />
                ))}
              </ul>
            </li>
          ))}
        </ol>
      )}
    </section>
  );
}

function TargetRow({
  target: t,
  name,
  onChanged,
}: {
  target: Target;
  name: string | undefined;
  onChanged: (post: Post) => void;
}) {
  const platform = platformNames[t.platform] ?? t.platform;
  const where = `${platform}${name ? `, ${name}` : ""}`;
  // An interrupted failure may already be live: say so, and send only when asked twice.
  const [warning, setWarning] = useState(false);
  const [sending, setSending] = useState(false);
  const [error, setError] = useState("");

  async function sendAgain() {
    setSending(true);
    setError("");
    const { data, error } = await api.POST("/v1/targets/{targetId}/retry", {
      params: { path: { targetId: t.id } },
    });
    setSending(false);
    if (data) {
      setWarning(false);
      onChanged(data);
    } else {
      setError(messageOf(error));
    }
  }

  // A handoff gets its postmark too: the user sent it from WhatsApp's own app,
  // so there is no link to show, only where and when.
  if ((t.status === "published" || t.status === "handed_off") && t.published_at) {
    const at = new Date(t.published_at);
    return (
      <li className="target published">
        <Postmark label={platformNames[t.platform] ?? t.platform} day={day.format(at)} time={time.format(at)} />
        <span>
          {t.remote_url ? (
            <a href={t.remote_url} target="_blank" rel="noreferrer">
              {where}
            </a>
          ) : (
            where
          )}
          {t.status === "handed_off" && <span className="reason">Sent from the phone</span>}
        </span>
      </li>
    );
  }

  const status =
    t.status === "failed"
      ? "Not sent"
      : t.status === "publishing"
        ? "Sending"
        : t.status === "handing_off"
          ? "On the phone"
          : t.last_error
            ? "Retrying"
            : "Waiting";
  return (
    <li className={`target ${t.status}`}>
      <span className="pill">{status}</span>
      <span>
        {where}
        {t.last_error && <span className="reason">{sentence(t.last_error)}</span>}
        {warning && (
          <span className="reason caution">
            The last attempt stopped mid-send, so this may already be on {platform}. Check there before sending
            it again.
          </span>
        )}
        {error && (
          <span className="reason error" role="alert">
            {sentence(error)}
          </span>
        )}
        {/* No account: disconnected, or a WhatsApp handoff, which is sent again from the phone. */}
        {t.status === "failed" && t.account_id && (
          <span className="target-actions">
            <button
              className="quiet"
              disabled={sending}
              onClick={() => (t.interrupted && !warning ? setWarning(true) : void sendAgain())}
            >
              {sending ? "Sending…" : warning ? "Send anyway" : "Send again"}
            </button>
            {warning && (
              <button className="quiet" disabled={sending} onClick={() => setWarning(false)}>
                Cancel
              </button>
            )}
          </span>
        )}
      </span>
    </li>
  );
}

/** Server errors are written for logs, lowercase first; show them as sentences. */
function sentence(s: string): string {
  return s.charAt(0).toUpperCase() + s.slice(1);
}

/**
 * A cancellation mark: the ring with the place and date, and the wavy lines a
 * post office strikes across a stamp. It means one thing only — this landed.
 */
function Postmark({ label, day: d, time: t }: { label: string; day: string; time: string }) {
  return (
    <svg className="postmark" viewBox="0 0 120 64" role="img" aria-label={`Published on ${label}, ${d} at ${t}`}>
      <circle cx="32" cy="32" r="29" />
      <circle cx="32" cy="32" r="24.5" className="inner" />
      <text x="32" y="23.5" className="pm-label">
        {label}
      </text>
      <text x="32" y="36" className="pm-day">
        {d}
      </text>
      <text x="32" y="46" className="pm-time">
        {t}
      </text>
      {[20, 29, 38, 47].map((y) => (
        <path key={y} d={`M66 ${y} q6 -4.5 12 0 t12 0 t12 0 t12 0`} />
      ))}
    </svg>
  );
}
