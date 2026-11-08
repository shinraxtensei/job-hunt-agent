import { useCallback, useEffect, useRef, useState } from "react";
import { get, post, type Draft, type DraftBullet, type Drafts, type Finding, type Lead } from "./api";
import { Icon } from "./icons";
import { label } from "./labels";

const POLL_MS = 3000;
const POLL_TRIES = 40; // two minutes: the PRD's target for a draft

const STATUS: Record<Draft["status"], string> = {
  requested: "Being written…",
  ready_for_review: "Ready for review",
  blocked: "Blocked: a sentence breaks the evidence rules",
  approved: "Approved",
  discarded: "Discarded",
  failed: "Couldn’t be written",
};

const RULES: Record<string, string> = {
  unsupported: "Not supported by the cited evidence",
  altered_fact: "A title, date or name that isn’t in the evidence",
  inflated_number: "A number that isn’t in the cited evidence",
  learning_as_experience: "A skill you’re only learning, shown as experience",
  familiar_in_bullet: "A familiar skill outside the skills section",
  too_long: "The letter is over 250 words",
  checker_unsupported: "The claim checker found this isn’t in the cited evidence",
  checker_altered_fact: "The claim checker found a changed title, name or date",
  scope_inflation: "A stronger verb than the evidence uses",
  suspicious_input: "The posting’s skills look like instructions (possible injection)",
};

const dates = (s: { start: string | null; end: string | null }) =>
  [s.start, s.end === "present" ? "present" : s.end].filter(Boolean).join(" – ");

// DraftDialog: ask for a tailored CV draft and review it next to its evidence (M4-05).
// Nothing is sent or exported from here: approve and discard only record the decision.
export function DraftDialog({ lead, open, onClose }: { lead: Lead; open: boolean; onClose: () => void }) {
  const ref = useRef<HTMLDialogElement>(null);
  const [data, setData] = useState<Drafts | null>(null);
  const [shown, setShown] = useState(0); // index into data.drafts
  const [error, setError] = useState<string | null>(null);
  const [waiting, setWaiting] = useState(false);

  useEffect(() => {
    const d = ref.current;
    if (!d) return;
    if (open && !d.open) d.showModal();
    if (!open && d.open) d.close();
  }, [open]);

  const load = useCallback(() => get<Drafts>(`/api/leads/${lead.id}/drafts`).then(setData), [lead.id]);

  useEffect(() => {
    if (!open) return;
    load().catch((e: unknown) => setError(e instanceof Error ? e.message : String(e)));
  }, [open, load]);

  // While the newest draft is being written, poll until it's done.
  const pending = data?.drafts[0]?.status === "requested";
  useEffect(() => {
    if (!open || !pending) return;
    let n = 0;
    setWaiting(true);
    const t = setInterval(() => {
      n++;
      load().catch(() => {});
      if (n >= POLL_TRIES) clearInterval(t);
    }, POLL_MS);
    return () => {
      clearInterval(t);
      setWaiting(false);
    };
  }, [open, pending, load]);

  const act = (path: string) => {
    setError(null);
    post(path)
      .then(() => load())
      .then(() => setShown(0))
      .catch((e: unknown) => setError(e instanceof Error ? e.message : String(e)));
  };

  const draft = data?.drafts[shown];
  return (
    <dialog ref={ref} className="sheet draft-sheet" aria-labelledby={`draft-title-${lead.id}`} onClose={onClose}>
      <div className="sheet-head">
        <h2 id={`draft-title-${lead.id}`}>
          CV draft <span className="draft-for">· {lead.company.name}</span>
        </h2>
        <button type="button" className="icon-button" aria-label="Close the draft" onClick={onClose}>
          <Icon name="close" />
        </button>
      </div>
      <div className="sheet-body">
        <p className="draft-lead">{lead.title}</p>
        <div className="draft-actions">
          <button type="button" className="button" disabled={pending} onClick={() => act(`/api/leads/${lead.id}/draft`)}>
            {data?.drafts.length ? "Make a new draft" : "Make a draft"}
          </button>
          {draft && (draft.status === "ready_for_review" || draft.status === "blocked" || draft.status === "failed") && (
            <>
              <button type="button" className="button" disabled={draft.status !== "ready_for_review"} onClick={() => act(`/api/drafts/${draft.id}/approve`)}>
                Approve
              </button>
              <button type="button" className="link-button" onClick={() => act(`/api/drafts/${draft.id}/discard`)}>
                Discard
              </button>
            </>
          )}
        </div>
        <p className="draft-note">Nothing is sent from here. Approving records your decision; the PDFs come later.</p>
        {error && (
          <p className="draft-error" role="alert">
            {error}
          </p>
        )}
        {!data && !error && (
          <p role="status" aria-busy="true">
            Loading drafts…
          </p>
        )}
        {data && data.drafts.length === 0 && <p>No draft yet for this lead.</p>}
        {data && data.drafts.length > 1 && (
          <div className="field">
            <label htmlFor={`draft-version-${lead.id}`}>Version</label>
            <select id={`draft-version-${lead.id}`} name="version" value={shown} onChange={(e) => setShown(Number(e.target.value))}>
              {data.drafts.map((d, i) => (
                <option key={d.id} value={i}>
                  {`${data.drafts.length - i} of ${data.drafts.length} · ${STATUS[d.status]}`}
                </option>
              ))}
            </select>
          </div>
        )}
        {draft && data && (
          <DraftView
            draft={draft}
            evidence={data.evidence}
            waiting={waiting && shown === 0}
            onSent={(id) => {
              setError(null);
              post(`/api/documents/${id}/sent`)
                .then(() => load())
                .catch((e: unknown) => setError(e instanceof Error ? e.message : String(e)));
            }}
          />
        )}
      </div>
    </dialog>
  );
}

export function DraftView(props: { draft: Draft; evidence: Drafts["evidence"]; waiting: boolean; onSent?: (id: number) => void }) {
  const { draft: d, evidence, waiting, onSent } = props;
  const findings = [...(d.findings?.violations ?? []).map((f) => ({ ...f, warning: false })), ...(d.findings?.warnings ?? []).map((f) => ({ ...f, warning: true }))];
  const at = (where: string) => findings.filter((f) => f.where === where);
  const c = d.content;
  return (
    <article className="draft" aria-label="Draft">
      <p className={`draft-status ${d.status}`} role="status">
        {STATUS[d.status]}
        {waiting && " This takes a few seconds."}
        {d.variant && <> · CV: {label("cv_variant", d.variant)}</>}
      </p>
      {d.error && <p className="draft-error">{d.error}</p>}
      {findings.some((f) => f.where === "draft") && (
        <ul className="findings">
          {at("draft").map((f, i) => (
            <FindingItem key={i} f={f} warning={f.warning} evidence={evidence} />
          ))}
        </ul>
      )}
      {c && (
        <>
          <header className="cv-head">
            <p className="cv-name">{c.person.name}</p>
            <p className="cv-meta">
              {[c.person.headline, c.person.location, c.person.email, c.person.phone].filter(Boolean).join(" · ")}
            </p>
            {c.person.links.length > 0 && <p className="cv-meta">{c.person.links.join(" · ")}</p>}
          </header>
          {c.summary.length > 0 && (
            <section aria-label="Summary">
              <h3>Summary</h3>
              <ul className="bullets">
                {c.summary.map((b, i) => (
                  <BulletItem key={i} b={b} where={`summary[${i}]`} at={at} evidence={evidence} />
                ))}
              </ul>
            </section>
          )}
          {(["roles", "projects"] as const).map((kind) =>
            c[kind].length === 0 ? null : (
              <section key={kind} aria-label={kind === "roles" ? "Experience" : "Projects"}>
                <h3>{kind === "roles" ? "Experience" : "Projects"}</h3>
                {c[kind].map((sec) => (
                  <div key={sec.id} className="cv-section">
                    <p className="cv-role">
                      <b>{sec.title ?? sec.name}</b>
                      {sec.employer && <> · {sec.employer}</>}
                      {sec.stack && <> · {sec.stack}</>}
                      <span className="cv-dates">
                        {[sec.location, dates(sec)].filter(Boolean).join(" · ")}
                      </span>
                    </p>
                    <ul className="bullets">
                      {sec.bullets.map((b, i) => (
                        <BulletItem key={i} b={b} where={`${kind}[${sec.id}].bullets[${i}]`} at={at} evidence={evidence} />
                      ))}
                    </ul>
                  </div>
                ))}
              </section>
            ),
          )}
          {c.skills.length > 0 && (
            <section aria-label="Skills">
              <h3>Skills</h3>
              <p>{c.skills.join(" · ")}</p>
              <ul className="findings">
                {findings
                  .filter((f) => f.where.startsWith("skills["))
                  .map((f, i) => (
                    <FindingItem key={i} f={f} warning={f.warning} evidence={evidence} />
                  ))}
              </ul>
            </section>
          )}
          {c.education.length > 0 && (
            <section aria-label="Education">
              <h3>Education</h3>
              <ul className="plain">
                {c.education.map((e) => (
                  <li key={e.id}>
                    <b>{e.school}</b> · {e.degree} <span className="cv-dates">{[e.location, dates(e)].filter(Boolean).join(" · ")}</span>
                  </li>
                ))}
              </ul>
            </section>
          )}
          {c.languages.length > 0 && (
            <section aria-label="Languages">
              <h3>Languages</h3>
              <p>{c.languages.map((l) => `${l.name} (${l.level})`).join(" · ")}</p>
            </section>
          )}
        </>
      )}
      {d.letter && (
        <section aria-label="Cover letter" className="letter">
          <h3>Cover letter</h3>
          <p>{d.letter.greeting}</p>
          <p>{d.letter.opening}</p>
          {d.letter.paragraphs.map((para, p) => (
            <ul key={p} className="bullets letter-paragraph">
              {para.map((b, i) => (
                <BulletItem key={i} b={b} where={`letter[${p}][${i}]`} at={at} evidence={evidence} />
              ))}
            </ul>
          ))}
          <p>
            {d.letter.closing}
            <br />
            {d.letter.name}
          </p>
          <p className="draft-versions">{d.letter.words} words</p>
          {at("letter").length > 0 && (
            <ul className="findings">
              {at("letter").map((f, i) => (
                <FindingItem key={i} f={f} warning={f.warning} evidence={evidence} />
              ))}
            </ul>
          )}
        </section>
      )}
      {d.documents.length > 0 && <DocumentsView documents={d.documents} onSent={onSent} />}
      {d.coverage && <CoverageView skills={d.coverage.skills} />}
      <p className="draft-versions">
        {[d.model, d.prompt_version && `prompt v${d.prompt_version}`, d.evidence_version && `evidence ${d.evidence_version.slice(0, 8)}`]
          .filter(Boolean)
          .join(" · ")}
      </p>
    </article>
  );
}

function BulletItem(props: { b: DraftBullet; where: string; at: (w: string) => (Finding & { warning: boolean })[]; evidence: Drafts["evidence"] }) {
  const { b, where, at, evidence } = props;
  const [open, setOpen] = useState(false);
  const id = `ev-${where.replace(/[^\w]/g, "-")}`;
  const here = at(where);
  return (
    <li className={here.some((f) => !f.warning) ? "has-violation" : undefined}>
      <span className="bullet-text">{b.text}</span>{" "}
      <button type="button" className="link-button evidence-toggle" aria-expanded={open} aria-controls={id} onClick={() => setOpen((o) => !o)}>
        Evidence ({b.evidence_ids.length})
      </button>
      {open && (
        <ul id={id} className="evidence">
          {b.evidence_ids.map((e) => (
            <li key={e}>
              <code translate="no">{e}</code> {evidence[e]?.text ?? "(not in the bank)"}
              {evidence[e]?.retired && " · retired"}
            </li>
          ))}
        </ul>
      )}
      {here.length > 0 && (
        <ul className="findings">
          {here.map((f, i) => (
            <FindingItem key={i} f={f} warning={f.warning} evidence={evidence} />
          ))}
        </ul>
      )}
    </li>
  );
}

function FindingItem({ f, warning, evidence }: { f: Finding; warning: boolean; evidence: Drafts["evidence"] }) {
  const closest = f.closest ? evidence[f.closest] : undefined;
  return (
    <li className={warning ? "finding warning" : "finding violation"}>
      <b>{warning ? "Warning" : "Blocks the draft"}:</b> {RULES[f.rule] ?? f.rule} — {f.detail}.
      {closest && (
        <span className="closest">
          {" "}
          Closest evidence: “{closest.text}”
        </span>
      )}
    </li>
  );
}

const day = (iso: string) => new Date(iso).toLocaleDateString(undefined, { day: "numeric", month: "short", year: "numeric" });

// DocumentsView: the draft's exported PDFs, kept as they were (M5-04). The owner sends
// them and marks what they sent; nothing is sent from here.
function DocumentsView({ documents, onSent }: { documents: Draft["documents"]; onSent?: (id: number) => void }) {
  return (
    <section aria-label="Documents" className="documents">
      <h3>Documents</h3>
      <ul className="plain">
        {documents.map((doc) => (
          <li key={doc.id}>
            <b>{doc.kind === "cv" ? "CV" : "Cover letter"} PDF</b> <span className="cv-dates">made {day(doc.made_at)}</span>{" "}
            {doc.sent_at ? (
              <span>· Sent {day(doc.sent_at)}</span>
            ) : (
              onSent && (
                <button type="button" className="link-button" onClick={() => onSent(doc.id)}>
                  Mark as sent
                </button>
              )
            )}
          </li>
        ))}
      </ul>
    </section>
  );
}

function CoverageView({ skills }: { skills: { name: string; status: string; why: string | null; lines: string[]; suspicious: boolean }[] }) {
  const group = (status: string) => skills.filter((s) => s.status === status);
  const why: Record<string, string> = { learning: "learning", missing: "not in your skills", no_evidence: "no evidence line" };
  return (
    <section aria-label="Coverage" className="coverage">
      <h3>Coverage</h3>
      <dl>
        <dt>Covered</dt>
        <dd>{group("covered").map((s) => `${s.name} (${s.lines.length})`).join(" · ") || "none"}</dd>
        <dt>Skills section only</dt>
        <dd>{group("listed").map((s) => s.name).join(" · ") || "none"}</dd>
        <dt>Gaps</dt>
        <dd>
          {group("gap").length === 0 ? (
            "none"
          ) : (
            <ul className="plain">
              {group("gap").map((s) => (
                <li key={s.name}>
                  {s.name} <span className="cv-dates">({s.why ? why[s.why] : "gap"})</span>
                  {s.suspicious && <b className="suspicious"> · possible injection, ignored</b>}
                </li>
              ))}
            </ul>
          )}
        </dd>
      </dl>
    </section>
  );
}
