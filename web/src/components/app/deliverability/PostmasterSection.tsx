// Gmail's own reputation, spam rate and SPF/DKIM/DMARC results per sending domain.

import { useState } from "react";
import toast from "react-hot-toast";
import { ExternalLinkIcon, GlobeIcon, UnplugIcon } from "lucide-react";
import { EmptyBlock, SectionBar } from "@/components/layout/Page";
import { Label, TextInput } from "@/components/ui/field";
import PermissionButton from "@/components/ui/PermissionButton";
import { PopoverMenu, PopoverMenuContent, PopoverMenuItem, PopoverMenuTrigger, SelectButton } from "@/components/ui/popover-menu";
import { useConfirm } from "@/hooks/context/confirm";
import { useConnectPostmaster, useDisconnectPostmaster, usePostmaster } from "@/lib/api/hooks/app/analytics/usePostmaster";
import type { AppError } from "@/lib/api/client/normalizeError";
import buildError from "@/lib/helper/buildError";
import type { PostmasterDay, PostmasterReputation } from "@/lib/api/models/app/analytics/Postmaster";

const POSTMASTER_URL = "https://postmaster.google.com/";
const DOCS_URL = "https://docs.dakiyamail.com/guides/deliverability/#gmail-postmaster-tools";

const REPUTATION: Record<PostmasterReputation, { label: string; chip: string }> = {
    high: { label: "High", chip: "bg-emerald-50 text-emerald-700 border-emerald-200" },
    medium: { label: "Medium", chip: "bg-amber-50 text-amber-700 border-amber-200" },
    low: { label: "Low", chip: "bg-orange-50 text-orange-700 border-orange-200" },
    bad: { label: "Bad", chip: "bg-rose-50 text-rose-700 border-rose-200" },
};

function pct(v: number | null | undefined, digits = 1): string {
    return v == null ? "—" : `${(v * 100).toFixed(digits)}%`;
}

// Gmail's bands: under 0.10% is healthy, 0.10% and up is a warning, 0.30% and up is critical.
function spamTone(v: number, warn: number, critical: number): string {
    return v >= critical ? "bg-rose-500" : v >= warn ? "bg-amber-500" : "bg-emerald-500";
}

function lastValue(days: PostmasterDay[], key: "spf_ratio" | "dkim_ratio" | "dmarc_ratio"): number | null {
    for (let i = days.length - 1; i >= 0; i--) {
        const v = days[i][key];
        if (v != null) return v;
    }
    return null;
}

function shortDate(iso: string): string {
    return new Date(`${iso}T00:00:00`).toLocaleDateString("en-US", { month: "short", day: "numeric" });
}

export default function PostmasterSection({ days }: { days: number }) {
    const [domain, setDomain] = useState("");
    const [connecting, setConnecting] = useState(false);
    const q = usePostmaster(domain, days);
    const disconnect = useDisconnectPostmaster();
    const confirm = useConfirm();
    const rep = q.data;

    const askDisconnect = () =>
        confirm.show(
            "Disconnect Google Postmaster Tools? The stored history stays, but it stops updating until you connect again.",
            async () => {
                try {
                    await disconnect.mutateAsync();
                } catch (e) {
                    toast.error(buildError(e as AppError));
                }
            },
        );

    const hasData = (rep?.summary.days_with_data ?? 0) > 0;

    return (
        <>
            <SectionBar label="Gmail Postmaster" count={rep?.domains.length || undefined}>
                {rep && rep.domains.length > 0 && (
                    <PopoverMenu align="end">
                        <PopoverMenuTrigger asChild>
                            <SelectButton icon={<GlobeIcon className="w-3.5 h-3.5" />} label={rep.domain || "Domain"} />
                        </PopoverMenuTrigger>
                        <PopoverMenuContent minWidth={220}>
                            {rep.domains.map((d) => (
                                <PopoverMenuItem key={d.domain} selected={d.domain === rep.domain} onSelect={() => setDomain(d.domain)}>
                                    {d.domain}
                                </PopoverMenuItem>
                            ))}
                        </PopoverMenuContent>
                    </PopoverMenu>
                )}
                {rep?.connected && rep.can_connect && (
                    <button
                        type="button"
                        onClick={askDisconnect}
                        className="h-7 px-2 rounded-md border border-slate-200 hover:border-slate-300 text-[12px] text-slate-600 hover:text-slate-900 inline-flex items-center gap-1.5 transition-colors"
                    >
                        <UnplugIcon className="w-3.5 h-3.5 text-slate-400" />
                        Disconnect
                    </button>
                )}
            </SectionBar>

            {q.isPending ? (
                <div className="px-5 py-4">
                    <div className="h-[160px] rounded-md bg-slate-50 animate-pulse" />
                </div>
            ) : q.isError || !rep ? (
                <EmptyBlock title="Couldn't load Gmail Postmaster" body="Try again in a moment." />
            ) : (
                <>
                    {rep.connected && rep.last_error && (
                        <p className="mx-5 mt-3 rounded-md border border-amber-200 bg-amber-50 px-3 py-2 text-[11.5px] text-amber-800 leading-relaxed">
                            The last sync failed: {rep.last_error}. Check the credentials, or reconnect them.
                        </p>
                    )}

                    {!rep.connected && hasData && (
                        <div className="mx-5 mt-3">
                            <div className="rounded-md border border-slate-200 bg-slate-50 px-3 py-2 flex flex-wrap items-center gap-x-3 gap-y-1.5">
                                <span className="text-[11.5px] text-slate-600">Not connected. This is stored history and it is no longer updating.</span>
                                {rep.can_connect && !connecting && (
                                    <PermissionButton
                                        permission="MANAGE_SETTINGS"
                                        onClick={() => setConnecting(true)}
                                        className="ml-auto h-6 px-2.5 rounded-md bg-sky-600 hover:bg-sky-700 text-white text-[11.5px] font-medium transition-colors"
                                    >
                                        Connect Google
                                    </PermissionButton>
                                )}
                            </div>
                            {connecting && <ConnectForm onDone={() => setConnecting(false)} />}
                        </div>
                    )}

                    {!rep.connected && !hasData ? (
                        <NotConnected canConnect={rep.can_connect} connecting={connecting} setConnecting={setConnecting} />
                    ) : rep.domains.length === 0 ? (
                        <EmptyBlock
                            title="No sending domains to report on"
                            body="Postmaster Tools covers domains you own. Mailboxes on gmail.com or other free providers have none."
                        />
                    ) : !hasData ? (
                        <EmptyBlock
                            title={`No Gmail data for ${rep.domain} yet`}
                            body="Gmail only reports a domain it sees enough mail from, and only after you verify it at postmaster.google.com with the Google account you connected. Data lands about two days after the mail is sent."
                            cta={
                                <a
                                    href={POSTMASTER_URL}
                                    target="_blank"
                                    rel="noreferrer"
                                    className="h-7 px-3 rounded-md border border-slate-200 hover:border-slate-300 text-[12px] text-slate-700 inline-flex items-center gap-1.5"
                                >
                                    Open Postmaster Tools
                                    <ExternalLinkIcon className="w-3 h-3" />
                                </a>
                            }
                        />
                    ) : (
                        <Report rep={rep} />
                    )}
                </>
            )}
        </>
    );
}

function Report({ rep }: { rep: NonNullable<ReturnType<typeof usePostmaster>["data"]> }) {
    const { summary, thresholds } = rep;
    const reputation = summary.latest_reputation ? REPUTATION[summary.latest_reputation] : null;
    const auth = [
        { label: "SPF", latest: lastValue(rep.data, "spf_ratio"), avg: summary.spf_avg },
        { label: "DKIM", latest: lastValue(rep.data, "dkim_ratio"), avg: summary.dkim_avg },
        { label: "DMARC", latest: lastValue(rep.data, "dmarc_ratio"), avg: summary.dmarc_avg },
    ];

    return (
        <>
            <div className="grid lg:grid-cols-[1fr_320px]">
                <div className="px-5 py-4 lg:border-r lg:border-slate-200">
                    <div className="flex flex-wrap items-baseline gap-x-3 gap-y-1 mb-3">
                        <span className="text-[12px] font-medium text-slate-900">User-reported spam rate</span>
                        <span className="text-[11px] text-slate-400">
                            {summary.spam_rate_avg == null
                                ? "no spam rate reported in this window"
                                : `average ${pct(summary.spam_rate_avg, 3)}, peak ${pct(summary.spam_rate_max, 3)}`}
                        </span>
                    </div>
                    <SpamTrend days={rep.data} warn={thresholds.spam_warn} critical={thresholds.spam_critical} />
                    <div className="mt-2 flex flex-wrap items-center gap-x-4 gap-y-1 text-[11px] text-slate-500">
                        <span className="inline-flex items-center gap-1.5">
                            <span className="h-px w-4 border-t border-dashed border-amber-500" />
                            {pct(thresholds.spam_warn, 2)} keep below
                        </span>
                        <span className="inline-flex items-center gap-1.5">
                            <span className="h-px w-4 border-t border-dashed border-rose-500" />
                            {pct(thresholds.spam_critical, 2)} never reach
                        </span>
                        {summary.days_over_warn > 0 && (
                            <span className="text-amber-700">
                                {summary.days_over_warn} {summary.days_over_warn === 1 ? "day" : "days"} at or above {pct(thresholds.spam_warn, 2)}
                                {summary.days_over_critical > 0 ? `, ${summary.days_over_critical} at or above ${pct(thresholds.spam_critical, 2)}` : ""}
                            </span>
                        )}
                    </div>
                </div>

                <aside className="bg-slate-50/40">
                    <div className="px-4 py-3 flex items-center gap-2 border-b border-slate-200/60">
                        <span className="text-[10px] uppercase tracking-[0.14em] text-slate-400 font-medium">Domain reputation</span>
                        <span className="ml-auto">
                            {reputation ? (
                                <span className={`inline-flex items-center rounded px-1.5 h-5 text-[10px] font-medium border ${reputation.chip}`}>{reputation.label}</span>
                            ) : (
                                <span
                                    className="text-[11px] text-slate-400"
                                    title="Gmail reports reputation for domains with enough volume, and not at all on its newer API."
                                >
                                    Not reported
                                </span>
                            )}
                        </span>
                    </div>
                    <div className="divide-y divide-slate-200/60">
                        {auth.map((a) => (
                            <div key={a.label} className="px-4 py-2.5">
                                <div className="flex items-center gap-2">
                                    <span className="text-[12px] text-slate-700">{a.label} pass rate</span>
                                    <span className="ml-auto font-mono text-[12px] text-slate-900 tabular-nums">{pct(a.latest)}</span>
                                </div>
                                <div className="mt-1.5 h-1 rounded-full bg-slate-200/70 overflow-hidden">
                                    <div
                                        className={`h-full rounded-full ${a.latest != null && a.latest < 0.95 ? "bg-amber-500" : "bg-emerald-500"}`}
                                        style={{ width: `${Math.round((a.latest ?? 0) * 100)}%` }}
                                    />
                                </div>
                                <div className="mt-1 text-[10.5px] text-slate-400">window average {pct(a.avg)}</div>
                            </div>
                        ))}
                    </div>
                </aside>
            </div>
            <p className="px-5 py-3 text-[11.5px] text-slate-500 leading-relaxed">
                This is Gmail&apos;s own data for {rep.domain}, up to {summary.latest_date ? shortDate(summary.latest_date) : "the latest day"}.
                It is reported per day and lags the mail by about two days.
                {rep.last_synced_at ? ` Last synced ${new Date(rep.last_synced_at).toLocaleString()}.` : ""}{" "}
                <a href={DOCS_URL} target="_blank" rel="noreferrer" className="text-slate-700 underline underline-offset-2 hover:text-slate-900">
                    How to read it
                </a>
            </p>
        </>
    );
}

// The spam rate per day against Gmail's two thresholds. Days without a rate leave a gap.
function SpamTrend({ days, warn, critical }: { days: PostmasterDay[]; warn: number; critical: number }) {
    const max = Math.max(critical * 1.15, ...days.map((d) => d.spam_rate ?? 0)) * 1.1;
    const y = (v: number) => (1 - v / max) * 100;
    const x = (i: number) => ((i + 0.5) / days.length) * 100;

    // One polyline per run of consecutive reporting days.
    const runs: string[] = [];
    let cur: string[] = [];
    days.forEach((d, i) => {
        if (d.spam_rate == null) {
            if (cur.length > 1) runs.push(cur.join(" "));
            cur = [];
            return;
        }
        cur.push(`${x(i).toFixed(2)},${y(d.spam_rate).toFixed(2)}`);
    });
    if (cur.length > 1) runs.push(cur.join(" "));

    return (
        <div>
            <div className="relative h-[150px]">
                <svg viewBox="0 0 100 100" preserveAspectRatio="none" className="absolute inset-0 w-full h-full overflow-visible">
                    <line x1="0" x2="100" y1={y(warn)} y2={y(warn)} stroke="#f59e0b" strokeWidth="1" strokeDasharray="4 3" vectorEffect="non-scaling-stroke" />
                    <line x1="0" x2="100" y1={y(critical)} y2={y(critical)} stroke="#f43f5e" strokeWidth="1" strokeDasharray="4 3" vectorEffect="non-scaling-stroke" />
                    <line x1="0" x2="100" y1="100" y2="100" stroke="#e2e8f0" strokeWidth="1" vectorEffect="non-scaling-stroke" />
                    {runs.map((points) => (
                        <polyline key={points} points={points} fill="none" stroke="#0284c7" strokeWidth="1.5" strokeLinejoin="round" vectorEffect="non-scaling-stroke" />
                    ))}
                </svg>
                {days.map((d, i) =>
                    d.spam_rate == null ? null : (
                        <span
                            key={d.date}
                            title={`${shortDate(d.date)}: ${pct(d.spam_rate, 3)} reported as spam`}
                            className={`absolute size-2 -ml-1 -mt-1 rounded-full ring-2 ring-white ${spamTone(d.spam_rate, warn, critical)}`}
                            style={{ left: `${x(i)}%`, top: `${y(d.spam_rate)}%` }}
                        />
                    ),
                )}
            </div>
            <div className="mt-1 flex justify-between font-mono text-[9.5px] text-slate-400">
                <span>{shortDate(days[0].date)}</span>
                <span>{shortDate(days[days.length - 1].date)}</span>
            </div>
        </div>
    );
}

function NotConnected({ canConnect, connecting, setConnecting }: { canConnect: boolean; connecting: boolean; setConnecting: (v: boolean) => void }) {
    return (
        <div className="px-5 py-8 max-w-[640px] mx-auto">
            <p className="text-[12.5px] text-slate-700 font-medium text-center">See what Gmail thinks of your sending domains</p>
            <p className="mt-1 text-[11.5px] text-slate-400 text-center leading-relaxed">
                Gmail Postmaster Tools reports domain reputation, the share of your mail users mark as spam, and SPF, DKIM and DMARC results straight from Gmail.
            </p>
            <ol className="mt-5 space-y-2 text-[12px] text-slate-600 leading-relaxed list-decimal pl-5">
                <li>
                    Verify each sending domain at{" "}
                    <a href={POSTMASTER_URL} target="_blank" rel="noreferrer" className="text-slate-900 underline underline-offset-2">
                        postmaster.google.com
                    </a>{" "}
                    with a DNS record, using the Google account that will own the data.
                </li>
                <li>
                    In Google Cloud, enable the Gmail Postmaster Tools API, create an OAuth client, and authorize it for the{" "}
                    <code className="font-mono text-[11px]">postmaster.readonly</code> and{" "}
                    <code className="font-mono text-[11px]">postmaster.traffic.readonly</code> scopes to get a refresh token.
                </li>
                <li>Connect the client ID, secret and refresh token here. They are stored encrypted on this server.</li>
            </ol>

            {!canConnect ? (
                <p className="mt-5 text-center text-[11.5px] text-slate-500">
                    Google credentials are shared by the whole instance, so the operator of this instance connects them.
                </p>
            ) : connecting ? (
                <ConnectForm onDone={() => setConnecting(false)} />
            ) : (
                <div className="mt-5 flex justify-center gap-1.5">
                    <PermissionButton
                        permission="MANAGE_SETTINGS"
                        onClick={() => setConnecting(true)}
                        className="h-7 px-3 rounded-md bg-sky-600 hover:bg-sky-700 text-white text-[12px] font-medium transition-colors"
                    >
                        Connect Google
                    </PermissionButton>
                    <a
                        href={DOCS_URL}
                        target="_blank"
                        rel="noreferrer"
                        className="h-7 px-3 rounded-md border border-slate-200 hover:border-slate-300 text-[12px] text-slate-700 inline-flex items-center gap-1.5"
                    >
                        Setup guide
                        <ExternalLinkIcon className="w-3 h-3" />
                    </a>
                </div>
            )}
        </div>
    );
}

function ConnectForm({ onDone }: { onDone: () => void }) {
    const connect = useConnectPostmaster();
    const [clientId, setClientId] = useState("");
    const [clientSecret, setClientSecret] = useState("");
    const [refreshToken, setRefreshToken] = useState("");
    const ready = clientId.trim() !== "" && clientSecret.trim() !== "" && refreshToken.trim() !== "";

    const submit = async () => {
        try {
            await connect.mutateAsync({ client_id: clientId, client_secret: clientSecret, refresh_token: refreshToken });
            toast.success("Google Postmaster Tools connected");
            onDone();
        } catch (e) {
            toast.error(buildError(e as AppError));
        }
    };

    return (
        <div className="mt-5 rounded-md border border-slate-200 bg-white p-4 space-y-3">
            <div>
                <Label>Client ID</Label>
                <TextInput value={clientId} onChange={setClientId} placeholder="123456789-abc.apps.googleusercontent.com" autoComplete="off" className="w-full" />
            </div>
            <div>
                <Label>Client secret</Label>
                <TextInput value={clientSecret} onChange={setClientSecret} type="password" autoComplete="off" className="w-full" />
            </div>
            <div>
                <Label>Refresh token</Label>
                <TextInput
                    value={refreshToken}
                    onChange={setRefreshToken}
                    type="password"
                    autoComplete="off"
                    className="w-full"
                    onKeyDown={(e) => {
                        if (e.key === "Enter" && ready && !connect.isPending) void submit();
                    }}
                />
            </div>
            <div className="flex justify-end gap-1.5">
                <button
                    type="button"
                    onClick={onDone}
                    className="h-7 px-3 rounded-md border border-slate-200 hover:border-slate-300 text-[12px] text-slate-700"
                >
                    Cancel
                </button>
                <button
                    type="button"
                    disabled={!ready || connect.isPending}
                    onClick={submit}
                    className="h-7 px-3 rounded-md bg-sky-600 hover:bg-sky-700 disabled:opacity-50 text-white text-[12px] font-medium transition-colors"
                >
                    {connect.isPending ? "Checking with Google" : "Connect"}
                </button>
            </div>
        </div>
    );
}
