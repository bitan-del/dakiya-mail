// Bulk mailbox import.
//
// Connecting mailboxes one form at a time does not scale past a handful, and
// a fleet of Google Workspace mailboxes all share the same server settings —
// only the address and the app password differ. So the input is a two-column
// CSV and the host settings come from a preset.
//
// This drives the same POST /emails/onboarding/smtp-imap the single-mailbox
// form uses, one row at a time with a small concurrency window. That is
// deliberate: the endpoint verifies the SMTP and IMAP login before it stores
// anything, so reusing it means an imported mailbox is exactly as verified as
// a hand-added one. It also means SalesBlink auto-enrolment fires per mailbox
// for free, since that hook lives in the same handler.

import React from "react";
import { Link } from "react-router-dom";
import toast from "react-hot-toast";
import { AlertTriangleIcon, CheckIcon, Loader2Icon, UploadIcon, XIcon } from "lucide-react";
import { usePermission } from "@/hooks/usePermission";
import { NoAccess } from "@/components/layout/NoAccess";
import addEmail from "@/lib/api/client/app/emails/addEmail";
import buildError from "@/lib/helper/buildError";
import type { AppError } from "@/lib/api/client/normalizeError";

// Host presets. Google Workspace and Gmail share these; both need an app
// password (Google turned off basic auth for IMAP/SMTP in May 2025).
const PRESETS = {
    google: { label: "Google Workspace / Gmail", smtpHost: "smtp.gmail.com", smtpPort: 587, imapHost: "imap.gmail.com", imapPort: 993 },
    microsoft: { label: "Microsoft 365 / Outlook", smtpHost: "smtp.office365.com", smtpPort: 587, imapHost: "outlook.office365.com", imapPort: 993 },
    custom: { label: "Custom", smtpHost: "", smtpPort: 587, imapHost: "", imapPort: 993 },
} as const;

type PresetKey = keyof typeof PRESETS;
type Hosts = { label: string; smtpHost: string; smtpPort: number; imapHost: string; imapPort: number };
type RowState = "pending" | "working" | "done" | "failed";

interface ParsedRow {
    email: string;
    password: string;
    name: string;
    state: RowState;
    error?: string;
}

// How many mailboxes to connect at once. Each row is a live SMTP+IMAP login,
// so this is kept low: providers rate-limit bursts of sign-ins from one IP,
// and a failed login is more expensive to retry than a slow import is to wait
// out.
const CONCURRENCY = 3;

/** Parse "email,password[,name]" lines. Tolerates a header row and blank lines. */
function parseCSV(text: string): { rows: ParsedRow[]; bad: string[] } {
    const rows: ParsedRow[] = [];
    const bad: string[] = [];
    for (const raw of text.split(/\r?\n/)) {
        const line = raw.trim();
        if (!line) continue;
        const parts = line.split(",").map((p) => p.trim());
        const [email, password, ...rest] = parts;
        if (!email || !password) {
            bad.push(line);
            continue;
        }
        // Skip a header row rather than trying to connect a mailbox called "email".
        if (/^e-?mail$/i.test(email)) continue;
        if (!email.includes("@")) {
            bad.push(line);
            continue;
        }
        rows.push({ email, password, name: rest.join(",").trim() || email.split("@")[0], state: "pending" });
    }
    return { rows, bad };
}

export default function BulkImportPage() {
    const canManage = usePermission("MANAGE_EMAILS");
    const [preset, setPreset] = React.useState<PresetKey>("google");
    const [hosts, setHosts] = React.useState<Hosts>(PRESETS.google);
    const [text, setText] = React.useState("");
    const [rows, setRows] = React.useState<ParsedRow[] | null>(null);
    const [running, setRunning] = React.useState(false);
    const cancelled = React.useRef(false);

    if (!canManage) return <NoAccess feature="Bulk import" permissionLabel="Manage mailboxes" />;

    const choosePreset = (k: PresetKey) => {
        setPreset(k);
        setHosts(PRESETS[k]);
    };

    const onFile = async (f: File | undefined) => {
        if (!f) return;
        setText(await f.text());
    };

    const preview = () => {
        const { rows: parsed, bad } = parseCSV(text);
        if (!parsed.length) {
            toast.error("No usable rows. Each line needs email,app-password");
            return;
        }
        if (bad.length) toast.error(`${bad.length} line${bad.length === 1 ? "" : "s"} skipped — need email,app-password`);
        setRows(parsed);
    };

    const run = async () => {
        if (!rows) return;
        if (!hosts.smtpHost || !hosts.imapHost) {
            toast.error("SMTP and IMAP hosts are required.");
            return;
        }
        setRunning(true);
        cancelled.current = false;

        const queue = rows.map((r, i) => ({ ...r, i }));
        let cursor = 0;

        const mark = (i: number, patch: Partial<ParsedRow>) =>
            setRows((prev) => (prev ? prev.map((r, idx) => (idx === i ? { ...r, ...patch } : r)) : prev));

        const worker = async () => {
            while (!cancelled.current) {
                const item = queue[cursor++];
                if (!item) return;
                if (item.state === "done") continue;
                mark(item.i, { state: "working", error: undefined });
                try {
                    await addEmail({
                        email: item.email,
                        name: item.name,
                        smtp: { username: item.email, password: item.password, host: hosts.smtpHost, port: hosts.smtpPort },
                        imap: { username: item.email, password: item.password, host: hosts.imapHost, port: hosts.imapPort },
                    });
                    mark(item.i, { state: "done" });
                } catch (err) {
                    mark(item.i, { state: "failed", error: buildError(err as AppError) });
                }
            }
        };

        await Promise.all(Array.from({ length: Math.min(CONCURRENCY, queue.length) }, worker));
        setRunning(false);
        if (!cancelled.current) toast.success("Import finished.");
    };

    const done = rows?.filter((r) => r.state === "done").length ?? 0;
    const failed = rows?.filter((r) => r.state === "failed").length ?? 0;
    const total = rows?.length ?? 0;

    return (
        <div className="max-w-3xl mx-auto px-4 py-8 md:px-8">
            <div className="mb-6">
                <h1 className="text-[20px] font-semibold text-slate-900">Import mailboxes</h1>
                <p className="mt-1 text-[13px] text-slate-600">
                    Connect many mailboxes at once from a CSV. Each is verified with a real SMTP and IMAP login, exactly
                    like adding one by hand.
                </p>
            </div>

            <section className="mb-6">
                <h2 className="text-[12px] font-medium uppercase tracking-wide text-slate-500">Server settings</h2>
                <div className="mt-2 flex flex-wrap gap-2">
                    {(Object.keys(PRESETS) as PresetKey[]).map((k) => (
                        <button
                            key={k}
                            onClick={() => choosePreset(k)}
                            className={`rounded-md border px-3 py-1.5 text-[13px] ${
                                preset === k ? "border-sky-500 bg-sky-50 text-sky-800" : "border-slate-300 text-slate-700"
                            }`}
                        >
                            {PRESETS[k].label}
                        </button>
                    ))}
                </div>
                {preset === "custom" && (
                    <div className="mt-3 grid grid-cols-2 gap-3">
                        <input className="rounded-md border border-slate-300 px-3 py-1.5 text-[13px]" placeholder="SMTP host"
                            value={hosts.smtpHost} onChange={(e) => setHosts({ ...hosts, smtpHost: e.target.value })} />
                        <input className="rounded-md border border-slate-300 px-3 py-1.5 text-[13px]" placeholder="SMTP port" type="number"
                            value={hosts.smtpPort} onChange={(e) => setHosts({ ...hosts, smtpPort: Number(e.target.value) })} />
                        <input className="rounded-md border border-slate-300 px-3 py-1.5 text-[13px]" placeholder="IMAP host"
                            value={hosts.imapHost} onChange={(e) => setHosts({ ...hosts, imapHost: e.target.value })} />
                        <input className="rounded-md border border-slate-300 px-3 py-1.5 text-[13px]" placeholder="IMAP port" type="number"
                            value={hosts.imapPort} onChange={(e) => setHosts({ ...hosts, imapPort: Number(e.target.value) })} />
                    </div>
                )}
                {preset === "google" && (
                    <div className="mt-2 flex items-start gap-2 text-[12.5px] text-amber-800">
                        <AlertTriangleIcon className="w-3.5 h-3.5 mt-0.5 shrink-0" />
                        <p>
                            Google needs an <strong>app password</strong>, not the account password — basic auth for IMAP
                            and SMTP was switched off in May 2025. Each mailbox needs 2-Step Verification on, and IMAP
                            enabled.
                        </p>
                    </div>
                )}
            </section>

            <section className="mb-6">
                <h2 className="text-[12px] font-medium uppercase tracking-wide text-slate-500">Mailboxes</h2>
                <p className="mt-1 text-[12.5px] text-slate-500">
                    One per line: <code className="rounded bg-slate-100 px-1">email,app-password</code> — add a third
                    column for the sender name.
                </p>
                <textarea
                    value={text}
                    onChange={(e) => setText(e.target.value)}
                    rows={8}
                    spellCheck={false}
                    placeholder={"ana@yourdomain.com,abcd efgh ijkl mnop,Ana Ruiz\nsam@yourdomain.com,qrst uvwx yzab cdef"}
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
                        Check
                    </button>
                </div>
            </section>

            {rows && (
                <section>
                    <div className="mb-3 flex flex-wrap items-center justify-between gap-3">
                        <h2 className="text-[12px] font-medium uppercase tracking-wide text-slate-500">
                            {total} mailbox{total === 1 ? "" : "es"}
                            {running || done || failed ? ` — ${done} connected, ${failed} failed` : ""}
                        </h2>
                        {running ? (
                            <button onClick={() => { cancelled.current = true; }}
                                className="rounded-md border border-slate-300 px-3 py-1.5 text-[13px] text-slate-700">
                                Stop
                            </button>
                        ) : (
                            <button onClick={() => void run()}
                                className="rounded-md bg-sky-600 px-3 py-1.5 text-[13px] font-medium text-white">
                                {failed > 0 ? "Retry failed" : `Connect ${total}`}
                            </button>
                        )}
                    </div>

                    <ul className="divide-y divide-slate-200/70 rounded-md border border-slate-200">
                        {rows.map((r, i) => (
                            <li key={`${r.email}-${i}`} className="flex items-start gap-3 px-3 py-2">
                                <span className="mt-0.5 shrink-0">
                                    {r.state === "done" && <CheckIcon className="w-4 h-4 text-emerald-600" />}
                                    {r.state === "failed" && <XIcon className="w-4 h-4 text-red-600" />}
                                    {r.state === "working" && <Loader2Icon className="w-4 h-4 animate-spin text-sky-600" />}
                                    {r.state === "pending" && <span className="block w-4 h-4 rounded-full border border-slate-300" />}
                                </span>
                                <div className="min-w-0 flex-1">
                                    <div className="truncate text-[13px] text-slate-900">{r.email}</div>
                                    {r.error && <div className="text-[12px] text-red-700">{r.error}</div>}
                                </div>
                            </li>
                        ))}
                    </ul>

                    {done > 0 && !running && (
                        <p className="mt-4 text-[12.5px] text-slate-600">
                            Connected mailboxes are warming automatically if SalesBlink auto-enrolment is on.{" "}
                            <Link to="/app/settings/salesblink" className="text-sky-700 underline underline-offset-2">
                                Check SalesBlink
                            </Link>
                        </p>
                    )}
                </section>
            )}
        </div>
    );
}
