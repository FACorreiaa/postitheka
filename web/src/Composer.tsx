import { useId, useMemo, useRef, useState } from "react";
import { type Account, api, type Issue, messageOf, type PlatformInfo, type Post, putFile } from "./api";
import { ALL_BRANDS, type BrandFilter, brandsOf, matchesBrand, resolveFilter } from "./brands";
import { readVideoFacts, type VideoFacts } from "./lib/video-facts";
import { countText, type Draft, platformNames, problems } from "./rules";

const BRAND_KEY = "postitheka.brand";

// A convenience, not state: which brand was showing last, on this browser.
function readBrand(): BrandFilter {
  try {
    const brand = localStorage.getItem(BRAND_KEY);
    return brand ? { kind: "brand", brand } : ALL_BRANDS;
  } catch {
    return ALL_BRANDS;
  }
}

function writeBrand(filter: BrandFilter) {
  try {
    if (filter.kind === "brand") localStorage.setItem(BRAND_KEY, filter.brand);
    else localStorage.removeItem(BRAND_KEY);
  } catch {
    // No storage: the choice lasts for this page only.
  }
}

interface Attachment {
  localId: string;
  file: File;
  preview: string;
  progress: number;
  mediaId?: string;
  error?: string;
  /** Length and size read from a video, once known (VID-001). */
  video?: VideoFacts;
}

interface Props {
  accounts: Account[] | null;
  platforms: PlatformInfo[];
  onConnect: () => void;
  onPublished: (post: Post) => void;
}

export function Composer({ accounts, platforms, onConnect, onPublished }: Props) {
  const [text, setText] = useState("");
  const [files, setFiles] = useState<Attachment[]>([]);
  const [selected, setSelected] = useState<Set<string>>(new Set());
  const [scheduleAt, setScheduleAt] = useState("");
  // What is set for one destination alone, by account id, then option key.
  const [options, setOptions] = useState<Record<string, Record<string, string>>>({});
  const [serverIssues, setServerIssues] = useState<Issue[]>([]);
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  // The Idempotency-Key for the post being published. It is kept while the
  // post is unchanged, so pressing Publish again after a lost response finds
  // the post the first press created instead of publishing it twice.
  const attempt = useRef<{ body: string; key: string } | null>(null);
  const textId = useId();

  const rulesFor = useMemo(() => new Map(platforms.map((p) => [p.platform as string, p.rules])), [platforms]);
  const draft: Draft = {
    text,
    media: files.map((f) => ({ contentType: f.file.type, size: f.file.size, ...f.video })),
  };

  const [brandChoice, setBrandChoice] = useState<BrandFilter>(readBrand);
  const brands = useMemo(() => brandsOf(accounts ?? []), [accounts]);
  const brand = resolveFilter(accounts ?? [], brandChoice);
  const shown = (accounts ?? []).filter((a) => matchesBrand(a, brand));

  // Only what is on screen can be published to. A stamp affixed under one
  // brand and then hidden by another would send a post somewhere the person
  // can no longer see.
  // Nor can an account whose credentials the platform refused (ACC-001): the
  // server would reject the post, so its stamp cannot be affixed.
  const chosen = shown.filter((a) => selected.has(a.id) && !a.needs_reconnect);

  function showBrand(next: BrandFilter) {
    setBrandChoice(next);
    writeBrand(next);
    setServerIssues([]);
  }
  const blocked = chosen.some((a) => {
    const rules = rulesFor.get(a.platform);
    return rules && problems(rules, { ...draft, options: options[a.id] }).length > 0;
  });
  const uploading = files.some((f) => !f.mediaId && !f.error);
  const failedUpload = files.some((f) => f.error);
  const empty = text.trim() === "" && files.length === 0;
  const canPublish = !busy && chosen.length > 0 && !empty && !blocked && !uploading && !failedUpload;
  // Why Publish is disabled, in the order a person would fix it. A disabled
  // button alone said nothing, and the first production user could not tell
  // that a stamp had to be clicked (2026-11-16).
  const waitingFor = busy || canPublish
    ? null
    : uploading
      ? "Publish unlocks when uploads finish."
      : failedUpload
        ? "Remove the file that failed to upload."
        : empty
          ? "Write something or add a photo or video."
          : shown.length > 0 && chosen.length === 0
            ? "Click a stamp to choose where it goes."
            : blocked
              ? "Fix what a chosen stamp objects to."
              : null;

  function patch(localId: string, change: Partial<Attachment>) {
    setFiles((prev) => prev.map((f) => (f.localId === localId ? { ...f, ...change } : f)));
  }

  // One file at a time, in the order chosen: each `await` in this loop is meant to wait.
  async function attach(list: FileList | null) {
    for (const file of Array.from(list ?? [])) {
      const item: Attachment = {
        localId: crypto.randomUUID(),
        file,
        preview: URL.createObjectURL(file),
        progress: 0,
      };
      setFiles((prev) => [...prev, item]);

      // Read before reserving: the server takes these with the upload and
      // cannot measure the file itself.
      // oxlint-disable-next-line no-await-in-loop -- one file at a time
      const video = file.type.startsWith("video/") ? await readVideoFacts(item.preview) : {};
      patch(item.localId, { video });

      // oxlint-disable-next-line no-await-in-loop -- one file at a time
      const { data, error: err } = await api.POST("/v1/media", {
        body: { content_type: file.type, size_bytes: file.size, duration_ms: video.durationMs, width: video.width, height: video.height },
      });
      if (!data) {
        patch(item.localId, { error: messageOf(err) });
        continue;
      }
      try {
        // oxlint-disable-next-line no-await-in-loop -- one file at a time
        await putFile(data.upload_url, data.headers, file, (p) => patch(item.localId, { progress: p }));
        patch(item.localId, { mediaId: data.media_id, progress: 1 });
      } catch (e) {
        // putFile rejects with an Error, whose message is written for people.
        patch(item.localId, { error: messageOf(e) });
      }
    }
  }

  // Previews are object URLs, which hold the file in memory until revoked.
  function detach(localId: string) {
    setFiles((prev) => {
      const gone = prev.find((f) => f.localId === localId);
      if (gone) URL.revokeObjectURL(gone.preview);
      return prev.filter((f) => f.localId !== localId);
    });
  }

  function toggle(id: string) {
    setServerIssues([]);
    setSelected((prev) => {
      const next = new Set(prev);
      if (next.has(id)) next.delete(id);
      else next.add(id);
      return next;
    });
  }

  async function publish() {
    setBusy(true);
    setError("");
    setServerIssues([]);
    const body = {
      text,
      media_ids: files.map((f) => f.mediaId!),
      account_ids: chosen.map((a) => a.id),
      // Only for where the post goes: the server refuses options for an
      // account that is not a destination.
      options: Object.fromEntries(chosen.map((a) => [a.id, options[a.id] ?? {}])),
      scheduled_at: scheduleAt ? new Date(scheduleAt).toISOString() : undefined,
    };
    const signature = JSON.stringify(body);
    if (attempt.current?.body !== signature) {
      attempt.current = { body: signature, key: crypto.randomUUID() };
    }

    let result;
    try {
      result = await api.POST("/v1/posts", {
        body,
        params: { header: { "Idempotency-Key": attempt.current.key } },
      });
    } catch {
      setBusy(false);
      setError("The connection dropped before the server answered. Press Publish again; it will not post twice.");
      return;
    }
    const { data, error: err, response } = result;
    setBusy(false);

    if (data) {
      attempt.current = null;
      onPublished(data);
      files.forEach((f) => URL.revokeObjectURL(f.preview));
      setText("");
      setFiles([]);
      setOptions({});
      setScheduleAt("");
      return;
    }
    if (response.status === 422 && err && "issues" in err) {
      setServerIssues(err.issues);
      return;
    }
    setError(messageOf(err));
  }

  const generalIssues = serverIssues.filter((i) => !i.account_id);

  return (
    <section className="composer" aria-labelledby={`${textId}-h`}>
      <h2 id={`${textId}-h`} className="visually-hidden">
        New post
      </h2>

      <label htmlFor={textId} className="visually-hidden">
        Post text
      </label>
      <textarea
        id={textId}
        className="sheet"
        placeholder="What do you want to say?"
        value={text}
        onChange={(e) => {
          setText(e.target.value);
          setServerIssues([]);
        }}
        rows={7}
      />

      <div className="attachments">
        {files.map((f) => (
          <figure key={f.localId} className={f.error ? "thumb failed" : "thumb"}>
            {f.file.type.startsWith("video/") ? (
              <video src={f.preview} muted playsInline aria-label={f.file.name} />
            ) : (
              <img src={f.preview} alt={f.file.name} />
            )}
            {!f.mediaId && !f.error && <progress value={f.progress} max={1} aria-label={`Uploading ${f.file.name}`} />}
            {f.error && <figcaption role="alert">{f.error}</figcaption>}
            <button className="remove" onClick={() => detach(f.localId)} aria-label={`Remove ${f.file.name}`}>
              ×
            </button>
          </figure>
        ))}
        <label className="add-media">
          <input
            type="file"
            accept="image/*,video/*"
            multiple
            onChange={(e) => {
              void attach(e.target.files);
              e.target.value = "";
            }}
          />
          Add photos or video
        </label>
      </div>

      <fieldset className="destinations">
        <legend>Send to</legend>
        {accounts === null ? (
          <p className="help">Loading your accounts…</p>
        ) : accounts.length === 0 ? (
          <div className="empty">
            <p>Connect a channel to start posting.</p>
            <button className="primary" onClick={onConnect}>
              Connect Discord
            </button>
          </div>
        ) : (
          <>
            {brands.length > 0 && (
              <div className="brands" role="group" aria-label="Brand">
                <button type="button" aria-pressed={brand.kind === "all"} onClick={() => showBrand(ALL_BRANDS)}>
                  All
                </button>
                {brands.map((b) => (
                  <button
                    key={b}
                    type="button"
                    aria-pressed={brand.kind === "brand" && brand.brand === b}
                    onClick={() => showBrand({ kind: "brand", brand: b })}
                  >
                    {b}
                  </button>
                ))}
              </div>
            )}
            <ul className="stamps">
              {shown.map((a) => {
                const rules = rulesFor.get(a.platform);
                const own = serverIssues.filter((i) => i.account_id === a.id).map((i) => i.message);
                return (
                  <li key={a.id}>
                    <Stamp
                      account={a}
                      used={countText(text, rules?.text_counting)}
                      limit={rules?.max_text_length}
                      problems={
                        selected.has(a.id) ? [...(rules ? problems(rules, { ...draft, options: options[a.id] }) : []), ...own] : []
                      }
                      on={selected.has(a.id) && !a.needs_reconnect}
                      onToggle={() => toggle(a.id)}
                      onReconnect={onConnect}
                      fields={rules?.options ?? []}
                      values={options[a.id] ?? {}}
                      onField={(key, value) => {
                        setServerIssues([]);
                        setOptions((prev) => ({ ...prev, [a.id]: { ...prev[a.id], [key]: value } }));
                      }}
                    />
                  </li>
                );
              })}
            </ul>
          </>
        )}
      </fieldset>

      {(generalIssues.length > 0 || error) && (
        <ul className="error" role="alert">
          {generalIssues.map((i) => (
            <li key={i.field + i.message}>{i.message}</li>
          ))}
          {error && <li>{error}</li>}
        </ul>
      )}

      <div className="send">
        <label className="schedule">
          <span>Publish at</span>
          <input type="datetime-local" value={scheduleAt} onChange={(e) => setScheduleAt(e.target.value)} />
        </label>
        <button className="primary" disabled={!canPublish} onClick={publish}>
          {busy ? "Publishing…" : scheduleAt ? "Schedule" : "Publish"}
        </button>
      </div>
      {waitingFor && <p className="help publish-reason">{waitingFor}</p>}
    </section>
  );
}

interface StampProps {
  account: Account;
  used: number;
  limit: number | undefined;
  problems: string[];
  on: boolean;
  onToggle: () => void;
  /** Opens Accounts, where an account that needs it is connected again. */
  onReconnect: () => void;
  /** What this platform lets a post set for one destination; shown once affixed. */
  fields: PlatformInfo["rules"]["options"];
  values: Record<string, string>;
  onField: (key: string, value: string) => void;
}

function Stamp({ account, used, limit, problems: issues, on, onToggle, onReconnect, fields, values, onField }: StampProps) {
  const over = limit !== undefined && used > limit;
  if (account.needs_reconnect) {
    return (
      <div className="stamp-wrap">
        <button className="stamp stale" aria-pressed={false} disabled>
          <span className="platform">{platformNames[account.platform] ?? account.platform}</span>
          <span className="name">{account.display_name}</span>
        </button>
        <p className="stamp-issues">
          Needs reconnecting.{" "}
          <button className="link" onClick={onReconnect}>
            Open Accounts
          </button>
        </p>
      </div>
    );
  }
  return (
    <div className="stamp-wrap">
      <button
        className={`stamp${on ? " on" : ""}${on && issues.length ? " trouble" : ""}`}
        aria-pressed={on}
        onClick={onToggle}
      >
        <span className="platform">{platformNames[account.platform] ?? account.platform}</span>
        <span className="name">{account.display_name}</span>
        {on && limit !== undefined && (
          <span className={over ? "count over" : "count"}>
            {used.toLocaleString()} / {limit.toLocaleString()}
          </span>
        )}
      </button>
      {on &&
        fields.map((f) => (
          <label key={f.key} className="stamp-option">
            {f.label}
            {!f.required && " (optional)"}
            <input value={values[f.key] ?? ""} onChange={(e) => onField(f.key, e.target.value)} />
          </label>
        ))}
      {issues.length > 0 && (
        <ul className="stamp-issues">
          {issues.map((p) => (
            <li key={p}>{p}</li>
          ))}
        </ul>
      )}
    </div>
  );
}
