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
  onRemoved,
}: {
  posts: Post[] | null;
  accounts: Account[];
  /** A post came back changed (a target sent again, or the post moved). */
  onChanged: (post: Post) => void;
  /** A post was cancelled before it was sent. */
  onRemoved: (id: string) => void;
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
              {unsent(p) && <Unsent post={p} onChanged={onChanged} onRemoved={onRemoved} />}
            </li>
          ))}
        </ol>
      )}
    </section>
  );
}

/**
 * Nothing has gone out yet, so the post can still be cancelled or moved. The
 * server decides for real; this only hides buttons that would be refused.
 */
function unsent(p: Post): boolean {
  return p.targets.length > 0 && p.targets.every((t) => t.status === "pending");
}

/** The value a datetime-local input wants: local time, to the minute. */
function localInput(at: Date): string {
  const local = new Date(at.getTime() - at.getTimezoneOffset() * 60_000);
  return local.toISOString().slice(0, 16);
}

function Unsent({
  post,
  onChanged,
  onRemoved,
}: {
  post: Post;
  onChanged: (post: Post) => void;
  onRemoved: (id: string) => void;
}) {
  const [mode, setMode] = useState<"idle" | "moving" | "cancelling">("idle");
  const [at, setAt] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const path = { params: { path: { postId: post.id } } };

  async function move() {
    setBusy(true);
    setError("");
    const { data, error } = await api.PATCH("/v1/posts/{postId}", {
      ...path,
      body: { scheduled_at: new Date(at).toISOString() },
    });
    setBusy(false);
    if (data) {
      setMode("idle");
      onChanged(data);
    } else {
      setError(messageOf(error));
    }
  }

  async function cancel() {
    setBusy(true);
    setError("");
    const { error, response } = await api.DELETE("/v1/posts/{postId}", path);
    setBusy(false);
    // Already gone is what was asked for.
    if (response.ok || response.status === 404) onRemoved(post.id);
    else setError(messageOf(error));
  }

  return (
    <div className="post-actions">
      {mode === "idle" && (
        <>
          <button
            className="quiet"
            onClick={() => {
              setAt(localInput(new Date(post.scheduled_at)));
              setError("");
              setMode("moving");
            }}
          >
            Reschedule
          </button>
          <button
            className="quiet"
            onClick={() => {
              setError("");
              setMode("cancelling");
            }}
          >
            Cancel post
          </button>
        </>
      )}
      {mode === "moving" && (
        <>
          <label className="schedule">
            <span>Publish at</span>
            <input type="datetime-local" value={at} onChange={(e) => setAt(e.target.value)} />
          </label>
          <button className="quiet" disabled={busy || !at} onClick={move}>
            {busy ? "Saving…" : "Save"}
          </button>
          <button className="quiet" disabled={busy} onClick={() => setMode("idle")}>
            Keep time
          </button>
        </>
      )}
      {mode === "cancelling" && (
        <>
          <span className="reason caution">This removes the post. It will not be sent.</span>
          <button className="quiet" disabled={busy} onClick={cancel}>
            {busy ? "Cancelling…" : "Cancel post"}
          </button>
          <button className="quiet" disabled={busy} onClick={() => setMode("idle")}>
            Keep post
          </button>
        </>
      )}
      {error && (
        <span className="reason error" role="alert">
          {sentence(error)}
        </span>
      )}
    </div>
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

  // A handoff gets its postmark too: the user sent it from the platform's own app,
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
      : t.status === "drafted"
        ? "Draft"
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
        {/* No postmark: nothing is public until its author posts it from the platform's app. */}
        {t.status === "drafted" && (
          <span className="reason">
            Waiting in {platform}
            {t.published_at ? ` since ${time.format(new Date(t.published_at))}` : ""}. Open the notification in the{" "}
            {platform} app to add a caption and post it.
          </span>
        )}
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
        {/* No account: disconnected, or a handoff (WhatsApp, Reddit), which is sent again from the phone. */}
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
