// Bulk email list validation.
//
// The verification engine already exists server-side (internal/pkg/emailverify:
// syntax -> MX -> SMTP RCPT probe, plus a random-localpart probe to spot a
// catch-all). What was missing was a way to run a whole list through it, so
// this drives POST /emails/verify per address with a small concurrency window
// and hands back a downloadable CSV.
//
// Concurrency is deliberately low. Each check opens a real SMTP conversation
// with the recipient's MX host from this server's IP; a burst of hundreds of
// parallel probes is indistinguishable from a directory-harvest attack and is
// a good way to get the IP blocked by the very providers you are about to mail.

import React from "react";
import toast from "react-hot-toast";
import { AlertTriangleIcon, DownloadIcon, Loader2Icon, UploadIcon } from "lucide-react";
import { usePermission } from "@/hooks/usePermission";
import { NoAccess } from "@/components/layout/NoAccess";
import verifyAddress, { type VerificationStatus } from "@/lib/api/client/app/emails/verifyAddress";
import buildError from "@/lib/helper/buildError";
import type { AppError } from "@/lib/api/client/normalizeError";

type RowState = "pending" | "working" | "done" | "error";

interface Row {
    email: string;
    state: RowState;
    status?: VerificationStatus;
    reason?: string;
    catchAll?: boolean;
    hasMX?: boolean;
    error?: string;
}

const CONCURRENCY = 5;

const STATUS_STYLE: Record<VerificationStatus, { label: string; cls: string }> = {
    valid: { label: "Valid", cls: "text-emerald-700 bg-emerald-50 border-emerald-200" },
    risky: { label: "Risky", cls: "text-amber-700 bg-amber-50 border-amber-200" },
    invalid: { label: "Invalid", cls: "text-red-700 bg-red-50 border-red-200" },
    unknown: { label: "Unknown", cls: "text-slate-600 bg-slate-50 border-slate-200" },
};

/** Pull addresses out of pasted text or a CSV. Takes the first email-looking
 *  field on each line, so a full contact export works without re-shaping it. */
function parseList(text: string): string[] {
    const seen = new Set<string>();
    const out: string[] = [];
    for (const raw of text.split(/\r?\n/)) {
        const line = raw.trim();
        if (!line) continue;
        const field = line
            .split(/[,;\t]/)
            .map((f) => f.trim().replace(/^["']|["']$/g, ""))
            .find((f) => /^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(f));
        if (!field) continue;
        const email = field.toLowerCase();
        if (seen.has(email)) continue;
        seen.add(email);
        out.push(email);
    }
    return out;
}

function toCSV(rows: Row[]): string {
    const head = "email,status,reason,catch_all,has_mx";
    const body = rows.map((r) =>
        [
            r.email,
            r.status ?? (r.error ? "error" : "not checked"),
            `"${(r.reason ?? r.error ?? "").replace(/"/g, '""')}"`,
            r.catchAll ? "yes" : "no",
            r.hasMX ? "yes" : "no",
        ].join(","),
    );
    return [head, ...body].join("\n");
}

export default function ValidateListPage() {
    const canView = usePermission("VIEW_CAMPAIGNS");
    const [text, setText] = React.useState("");
    const [rows, setRows] = React.useState<Row[] | null>(null);
    const [running, setRunning] = React.useState(false);
    const [filter, setFilter] = React.useState<VerificationStatus | "all">("all");
    const cancelled = React.useRef(false);

    if (!canView) return <NoAccess feature="Email validation" permissionLabel="View campaigns" />;

    const onFile = async (f: File | undefined) => {
        if (!f) return;
        setText(await f.text());
    };

    const preview = () => {
        const emails = parseList(text);
        if (!emails.length) {
            toast.error("No email addresses found.");
            return;
        }
        setRows(emails.map((email) => ({ email, state: "pending" })));
    };

    const run = async () => {
        if (!rows) return;
        setRunning(true);
        cancelled.current = false;

        const queue = rows.map((r, i) => ({ ...r, i })).filter((r) => r.state !== "done");
        let cursor = 0;
        const mark = (i: number, patch: Partial<Row>) =>
            setRows((prev) => (prev ? prev.map((r, idx) => (idx === i ? { ...r, ...patch } : r)) : prev));

        const worker = async () => {
            while (!cancelled.current) {
                const item = queue[cursor++];
                if (!item) return;
                mark(item.i, { state: "working", error: undefined });
                try {
                    const res = await verifyAddress(item.email);
                    mark(item.i, {
                        state: "done",
                        status: res.status,
                        reason: res.reason,
                        catchAll: res.is_catch_all,
                        hasMX: res.has_mx,
                    });
                } catch (err) {
                    mark(item.i, { state: "error", error: buildError(err as AppError) });
                }
            }
        };

        await Promise.all(Array.from({ length: Math.min(CONCURRENCY, queue.length) }, worker));
        setRunning(false);
        if (!cancelled.current) toast.success("Validation finished.");
    };

    const download = () => {
        if (!rows) return;
        const blob = new Blob([toCSV(rows)], { type: "text/csv" });
        const url = URL.createObjectURL(blob);
        const a = document.createElement("a");
        a.href = url;
        a.download = "email-validation.csv";
        a.click();
        URL.revokeObjectURL(url);
    };

    const counts = React.useMemo(() => {
        const c = { valid: 0, risky: 0, invalid: 0, unknown: 0, error: 0, checked: 0 };
        for (const r of rows ?? []) {
            if (r.state === "error") c.error++;
            if (r.status) {
                c[r.status]++;
                c.checked++;
            }
        }
        return c;
    }, [rows]);

    // A run that is entirely "unknown" is nearly always this instance being
    // unable to probe, not a bad list — and there are two very different
    // causes. Read the reason the server gave rather than guessing, so the
    // fix we suggest is the actual fix.
    const unknownReason = React.useMemo(() => {
        if (!rows) return null;
        const checked = rows.filter((r) => r.status);
        if (checked.length < 3 || checked.some((r) => r.status !== "unknown")) return null;
        const heloSkipped = checked.some((r) => (r.reason ?? "").includes("EMAIL_VERIFY_HELO_HOST"));
        return heloSkipped ? "helo" : "port25";
    }, [rows]);

    const shown = (rows ?? []).filter((r) => filter === "all" || r.status === filter);

    return (
        <div className="max-w-3xl mx-auto px-4 py-8 md:px-8">
            <div className="mb-6">
                <h1 className="text-[20px] font-semibold text-slate-900">Validate a list</h1>
                <p className="mt-1 text-[13px] text-slate-600">
                    Check addresses before you mail them. Each is tested with a real MX lookup and SMTP probe, so bad
                    addresses are dropped before they can bounce and cost you reputation.
                </p>
            </div>

            <section className="mb-6">
                <h2 className="text-[12px] font-medium uppercase tracking-wide text-slate-500">Addresses</h2>
                <p className="mt-1 text-[12.5px] text-slate-500">
                    One per line, or paste a CSV — the first email-looking column on each line is used. Duplicates are
                    removed.
                </p>
                <textarea
                    value={text}
                    onChange={(e) => setText(e.target.value)}
                    rows={8}
                    spellCheck={false}
                    placeholder={"ana@example.com\nsam@example.com"}
                    className="mt-2 w-full rounded-md border border-slate-300 px-3 py-2 font-mono text-[12.5px] outline-none focus:ring-2 focus:ring-sky-500/40"
                />
                <div className="mt-2 flex flex-wrap items-center gap-3">
                    <label className="inline-flex cursor-pointer items-center gap-1.5 text-[12.5px] text-slate-600">
                        <UploadIcon className="w-3.5 h-3.5" />
                        Upload CSV
                        <input type="file" accept=".csv,text/csv,text/plain" className="hidden"
                            onChange={(e) => void onFile(e.target.files?.[0])} />
                    </label>
                    <button onClick={preview} disabled={!text.trim()}
                        className="rounded-md border border-slate-300 px-3 py-1.5 text-[13px] text-slate-700 disabled:opacity-50">
                        Check list
                    </button>
                </div>
            </section>

            {rows && (
                <section>
                    <div className="mb-3 flex flex-wrap items-center justify-between gap-3">
                        <h2 className="text-[12px] font-medium uppercase tracking-wide text-slate-500">
                            {rows.length} address{rows.length === 1 ? "" : "es"}
                            {counts.checked ? ` — ${counts.checked} checked` : ""}
                        </h2>
                        <div className="flex items-center gap-2">
                            {counts.checked > 0 && (
                                <button onClick={download}
                                    className="inline-flex items-center gap-1.5 rounded-md border border-slate-300 px-3 py-1.5 text-[13px] text-slate-700">
                                    <DownloadIcon className="w-3.5 h-3.5" /> CSV
                                </button>
                            )}
                            {running ? (
                                <button onClick={() => { cancelled.current = true; }}
                                    className="rounded-md border border-slate-300 px-3 py-1.5 text-[13px] text-slate-700">
                                    Stop
                                </button>
                            ) : (
                                <button onClick={() => void run()}
                                    className="rounded-md bg-sky-600 px-3 py-1.5 text-[13px] font-medium text-white">
                                    {counts.checked ? "Check remaining" : `Validate ${rows.length}`}
                                </button>
                            )}
                        </div>
                    </div>

                    {counts.checked > 0 && (
                        <div className="mb-3 flex flex-wrap gap-2">
                            {(["all", "valid", "risky", "invalid", "unknown"] as const).map((k) => (
                                <button key={k} onClick={() => setFilter(k)}
                                    className={`rounded-md border px-2.5 py-1 text-[12.5px] ${
                                        filter === k ? "border-slate-400 bg-slate-100 text-slate-900" : "border-slate-200 text-slate-600"
                                    }`}>
                                    {k === "all" ? `All ${rows.length}` : `${STATUS_STYLE[k].label} ${counts[k]}`}
                                </button>
                            ))}
                        </div>
                    )}

                    {unknownReason && (
                        <div className="mb-3 flex items-start gap-2 rounded-md border border-amber-200 bg-amber-50 p-3 text-[12.5px] text-amber-900">
                            <AlertTriangleIcon className="w-4 h-4 shrink-0 mt-0.5" />
                            {unknownReason === "helo" ? (
                                <p>
                                    Every address came back <strong>unknown</strong> because the SMTP probe was skipped:
                                    this instance has no public hostname to greet the recipient's server with. Set{" "}
                                    <code className="rounded bg-amber-100 px-1">EMAIL_VERIFY_HELO_HOST</code> to a real,
                                    fully-qualified hostname for this server (a bare name, <code>localhost</code>, or a{" "}
                                    <code>.local</code> suffix is refused). Until then only syntax and MX are checked —
                                    which still catches dead domains, but not dead mailboxes.
                                </p>
                            ) : (
                                <p>
                                    Every address came back <strong>unknown</strong>. That usually means this server
                                    cannot open outbound connections on port 25 — most cloud providers and home ISPs
                                    block it by default — rather than anything being wrong with the list. Until port 25
                                    is open, treat these results as "not checked".
                                </p>
                            )}
                        </div>
                    )}

                    <ul className="divide-y divide-slate-200/70 rounded-md border border-slate-200">
                        {shown.map((r) => (
                            <li key={r.email} className="flex items-center gap-3 px-3 py-2">
                                <span className="w-4 shrink-0">
                                    {r.state === "working" && <Loader2Icon className="w-4 h-4 animate-spin text-sky-600" />}
                                    {r.state === "pending" && <span className="block w-3 h-3 rounded-full border border-slate-300" />}
                                </span>
                                <div className="min-w-0 flex-1">
                                    <div className="truncate text-[13px] text-slate-900">{r.email}</div>
                                    {(r.reason || r.error) && (
                                        <div className={`text-[12px] ${r.error ? "text-red-700" : "text-slate-500"}`}>
                                            {r.error ?? r.reason}
                                            {r.catchAll ? " · catch-all domain" : ""}
                                        </div>
                                    )}
                                </div>
                                {r.status && (
                                    <span className={`shrink-0 rounded border px-2 py-0.5 text-[11.5px] ${STATUS_STYLE[r.status].cls}`}>
                                        {STATUS_STYLE[r.status].label}
                                    </span>
                                )}
                            </li>
                        ))}
                    </ul>

                    <p className="mt-4 text-[12.5px] text-slate-500">
                        <strong>Risky</strong> usually means a catch-all domain: the server accepts every address, so
                        acceptance proves nothing. <strong>Unknown</strong> means the check was inconclusive — a timeout
                        or greylisting — not that the address is bad.
                    </p>
                </section>
            )}
        </div>
    );
}
