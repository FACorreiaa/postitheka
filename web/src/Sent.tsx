import type { Account, Post, Target } from "./api";
import { platformNames } from "./rules";

const time = new Intl.DateTimeFormat(undefined, { hour: "2-digit", minute: "2-digit" });
const day = new Intl.DateTimeFormat(undefined, { day: "numeric", month: "short" });
const full = new Intl.DateTimeFormat(undefined, { dateStyle: "medium", timeStyle: "short" });

export function Sent({ posts, accounts }: { posts: Post[] | null; accounts: Account[] }) {
  const names = new Map(accounts.map((a) => [a.id, a.display_name]));

  return (
    <section className="sent" aria-labelledby="sent-h">
      <h2 id="sent-h">Sent</h2>
      {posts === null ? (
        <p className="help">Loading…</p>
      ) : posts.length === 0 ? (
        <p className="help">Posts you publish appear here, with where each one landed.</p>
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
                  <TargetRow key={t.id} target={t} name={t.account_id ? names.get(t.account_id) : undefined} />
                ))}
              </ul>
            </li>
          ))}
        </ol>
      )}
    </section>
  );
}

function TargetRow({ target: t, name }: { target: Target; name: string | undefined }) {
  const where = `${platformNames[t.platform] ?? t.platform}${name ? `, ${name}` : ""}`;

  if (t.status === "published" && t.published_at) {
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
        </span>
      </li>
    );
  }

  const status =
    t.status === "failed" ? "Not sent" : t.status === "publishing" ? "Sending" : t.last_error ? "Retrying" : "Waiting";
  return (
    <li className={`target ${t.status}`}>
      <span className="pill">{status}</span>
      <span>
        {where}
        {t.last_error && <span className="reason">{sentence(t.last_error)}</span>}
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
