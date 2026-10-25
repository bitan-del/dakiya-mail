// Settings → SalesBlink.
//
// Hand mailbox warmup to a SalesBlink account instead of the local pool. The
// local pool can only pair this workspace's own mailboxes with each other,
// which providers learn to recognize; SalesBlink runs a large shared pool.
//
// The trade, stated plainly in the UI: enrolling sends the mailbox's SMTP and
// IMAP credentials to SalesBlink, because it has to sign in as the mailbox to
// send warmup mail, read replies and pull messages out of spam. OAuth
// mailboxes cannot be enrolled at all.

import React from "react";
import toast from "react-hot-toast";
import { FlameIcon, Loader2Icon, RefreshCwIcon, ShieldAlertIcon } from "lucide-react";
import { usePermission } from "@/hooks/usePermission";
import { useConfirm } from "@/hooks/context/confirm";
import { NoAccess } from "@/components/layout/NoAccess";
import buildError from "@/lib/helper/buildError";
import type { AppError } from "@/lib/api/client/normalizeError";
import {
    useConnectSalesBlink,
    useDisconnectSalesBlink,
    useSalesBlinkMailboxes,
    useSalesBlinkStatus,
    useSyncSalesBlink,
    useUnenrollSalesBlinkMailbox,
    useEnrollSalesBlinkMailbox,
    useUpdateSalesBlinkSettings,
} from "@/lib/api/hooks/app/salesblink/useSalesBlink";
import {
    SALESBLINK_MAX_PER_DAY,
    SALESBLINK_MIN_PER_DAY,
} from "@/lib/api/models/app/salesblink/SalesBlink";
import { Row, Section, SectionShell } from "../_components/SectionShell";

export default function SalesBlinkSettingsPage() {
    const canManage = usePermission("MANAGE_SETTINGS");
    const status = useSalesBlinkStatus(canManage);

    if (!canManage) return <NoAccess feature="SalesBlink" permissionLabel="Manage settings" />;

    if (status.isLoading || !status.data) {
        return (
            <SectionShell title="SalesBlink" description="Warm your mailboxes in SalesBlink's pool.">
                <div className="py-10 flex justify-center text-slate-400">
                    <Loader2Icon className="w-4 h-4 animate-spin" />
                </div>
            </SectionShell>
        );
    }

    return status.data.connected ? <Connected /> : <Connect />;
}

// ─── not connected ───────────────────────────────────────────────────────

function Connect() {
    const connect = useConnectSalesBlink();
    const [apiKey, setApiKey] = React.useState("");
    const [autoEnroll, setAutoEnroll] = React.useState(true);

    const submit = (e: React.FormEvent) => {
        e.preventDefault();
        if (!apiKey.trim()) return;
        connect.mutate(
            { apiKey: apiKey.trim(), autoEnroll },
            {
                onSuccess: () => {
                    setApiKey("");
                    toast.success("Connected to SalesBlink.");
                },
                onError: (err) => toast.error(buildError(err as unknown as AppError)),
            },
        );
    };

    return (
        <SectionShell title="SalesBlink" description="Warm your mailboxes in SalesBlink's pool instead of the local one.">
            <Section eyebrow="Connect" description="Create a key in SalesBlink under Account Settings → API.">
                <form onSubmit={submit} className="space-y-4">
                    <Row label="API key" description="Stored encrypted on this server and never shown again." align="start">
                        <input
                            type="password"
                            autoComplete="off"
                            value={apiKey}
                            onChange={(e) => setApiKey(e.target.value)}
                            placeholder="Your SalesBlink API key"
                            className="w-full sm:w-80 rounded-md border border-slate-300 px-3 py-1.5 text-[13px] outline-none focus:ring-2 focus:ring-sky-500/40"
                        />
                    </Row>
                    <Row
                        label="Warm new mailboxes automatically"
                        description="Every mailbox you connect from now on is enrolled as soon as it is added."
                    >
                        <input
                            type="checkbox"
                            checked={autoEnroll}
                            onChange={(e) => setAutoEnroll(e.target.checked)}
                            className="size-4 accent-sky-600"
                        />
                    </Row>
                    <button
                        type="submit"
                        disabled={connect.isPending || !apiKey.trim()}
                        className="inline-flex items-center gap-2 rounded-md bg-sky-600 px-3 py-1.5 text-[13px] font-medium text-white disabled:opacity-50"
                    >
                        {connect.isPending && <Loader2Icon className="w-3.5 h-3.5 animate-spin" />}
                        Connect
                    </button>
                </form>

                <CredentialNotice />
            </Section>
        </SectionShell>
    );
}

// CredentialNotice is deliberately prominent: enrolling is the one action here
// that moves mailbox passwords off this server, and that should never be a
// surprise on a self-hosted install.
function CredentialNotice() {
    return (
        <div className="mt-5 flex items-start gap-3 rounded-md border border-amber-200 bg-amber-50 p-3">
            <ShieldAlertIcon className="w-4 h-4 text-amber-600 shrink-0 mt-0.5" />
            <div className="text-[12.5px] text-amber-900 leading-relaxed">
                <p className="font-medium">Enrolling sends mailbox credentials to SalesBlink.</p>
                <p className="mt-1">
                    SalesBlink signs in as each mailbox to send warmup mail, read the replies and rescue messages from
                    spam, so it needs the SMTP and IMAP password. Mailboxes connected with Google or Microsoft sign-in
                    cannot be enrolled — their access is bound to this server.
                </p>
            </div>
        </div>
    );
}

// ─── connected ───────────────────────────────────────────────────────────

function Connected() {
    const status = useSalesBlinkStatus();
    const mailboxes = useSalesBlinkMailboxes();
    const sync = useSyncSalesBlink();
    const disconnect = useDisconnectSalesBlink();
    const update = useUpdateSalesBlinkSettings();
    const confirm = useConfirm();

    const link = status.data?.link;
    const rows = mailboxes.data ?? [];

    const runSync = () =>
        sync.mutate(undefined, {
            onSuccess: (res) => {
                if (res.failed > 0) {
                    toast.error(`Enrolled ${res.enrolled}. ${res.failed} failed — see the table.`);
                } else if (res.enrolled === 0) {
                    toast.success("Every eligible mailbox is already warming.");
                } else {
                    toast.success(`Enrolled ${res.enrolled} mailbox${res.enrolled === 1 ? "" : "es"}.`);
                }
            },
            onError: (err) => toast.error(buildError(err as unknown as AppError)),
        });

    const runDisconnect = () =>
        confirm.show(
            "Disconnect SalesBlink? Mailboxes stop being tracked here. The senders stay in your SalesBlink account — remove them there if you want warmup to stop.",
            () =>
                disconnect.mutate(undefined, {
                    onSuccess: () => toast.success("Disconnected."),
                    onError: (err) => toast.error(buildError(err as unknown as AppError)),
                }),
        );

    const saveSettings = (patch: Partial<{ auto_enroll: boolean; warmup_per_day: number; max_daily_frequency: number; ramp_up_frequency: number; auto_ramp_up: boolean }>) => {
        if (!link) return;
        update.mutate(
            {
                auto_enroll: patch.auto_enroll ?? link.auto_enroll,
                warmup_per_day: patch.warmup_per_day ?? link.warmup_per_day,
                max_daily_frequency: patch.max_daily_frequency ?? link.max_daily_frequency,
                ramp_up_frequency: patch.ramp_up_frequency ?? link.ramp_up_frequency,
                auto_ramp_up: patch.auto_ramp_up ?? link.auto_ramp_up,
            },
            { onError: (err) => toast.error(buildError(err as unknown as AppError)) },
        );
    };

    return (
        <SectionShell
            title="SalesBlink"
            description="Warmup for these mailboxes runs in SalesBlink's pool."
            actions={
                <button
                    onClick={runDisconnect}
                    className="rounded-md border border-slate-300 px-3 py-1.5 text-[13px] text-slate-700"
                >
                    Disconnect
                </button>
            }
        >
            <Section eyebrow="Status">
                <div className="flex flex-wrap gap-6">
                    <Stat label="Warming" value={status.data?.enrolled ?? 0} tone="good" />
                    <Stat label="Not yet enrolled" value={status.data?.eligible ?? 0} />
                    <Stat label="Cannot enrol" value={status.data?.ineligible ?? 0} tone="muted" />
                </div>
                {(status.data?.eligible ?? 0) > 0 && (
                    <button
                        onClick={runSync}
                        disabled={sync.isPending}
                        className="mt-4 inline-flex items-center gap-2 rounded-md bg-sky-600 px-3 py-1.5 text-[13px] font-medium text-white disabled:opacity-50"
                    >
                        {sync.isPending ? (
                            <Loader2Icon className="w-3.5 h-3.5 animate-spin" />
                        ) : (
                            <FlameIcon className="w-3.5 h-3.5" />
                        )}
                        Warm all {status.data?.eligible} remaining
                    </button>
                )}
                {link?.last_error ? (
                    <p className="mt-3 text-[12.5px] text-red-700">{link.last_error}</p>
                ) : null}
            </Section>

            {link && (
                <Section eyebrow="Warmup volume" description={`SalesBlink caps each mailbox at ${SALESBLINK_MAX_PER_DAY} warmup emails a day.`}>
                    <Row label="Enrol new mailboxes automatically" description="Applies as soon as a mailbox is connected.">
                        <input
                            type="checkbox"
                            checked={link.auto_enroll}
                            onChange={(e) => saveSettings({ auto_enroll: e.target.checked })}
                            className="size-4 accent-sky-600"
                        />
                    </Row>
                    <Row label="Starting volume" description="Warmup emails a day when a mailbox is first enrolled.">
                        <NumberInput value={link.warmup_per_day} onCommit={(v) => saveSettings({ warmup_per_day: v })} />
                    </Row>
                    <Row label="Maximum volume" description="Ceiling once ramp-up finishes.">
                        <NumberInput value={link.max_daily_frequency} onCommit={(v) => saveSettings({ max_daily_frequency: v })} />
                    </Row>
                    <Row label="Ramp-up step" description="How much the daily volume grows each step.">
                        <NumberInput value={link.ramp_up_frequency} onCommit={(v) => saveSettings({ ramp_up_frequency: v })} />
                    </Row>
                </Section>
            )}

            <Section
                eyebrow="Mailboxes"
                description="Every mailbox in this workspace and whether SalesBlink is warming it."
                actions={
                    <button
                        onClick={() => void mailboxes.refetch()}
                        className="inline-flex items-center gap-1.5 text-[12.5px] text-slate-600"
                    >
                        <RefreshCwIcon className={`w-3.5 h-3.5 ${mailboxes.isFetching ? "animate-spin" : ""}`} />
                        Refresh
                    </button>
                }
            >
                <MailboxTable rows={rows} loading={mailboxes.isLoading} />
            </Section>
        </SectionShell>
    );
}

function Stat({ label, value, tone }: { label: string; value: number; tone?: "good" | "muted" }) {
    const color = tone === "good" ? "text-emerald-700" : tone === "muted" ? "text-slate-400" : "text-slate-900";
    return (
        <div>
            <div className={`text-[22px] font-semibold leading-none ${color}`}>{value}</div>
            <div className="mt-1 text-[12px] text-slate-500">{label}</div>
        </div>
    );
}

function NumberInput({ value, onCommit }: { value: number; onCommit: (v: number) => void }) {
    const [draft, setDraft] = React.useState(String(value));
    React.useEffect(() => setDraft(String(value)), [value]);
    const commit = () => {
        const n = Number(draft);
        if (!Number.isFinite(n) || n < SALESBLINK_MIN_PER_DAY || n > SALESBLINK_MAX_PER_DAY) {
            setDraft(String(value));
            return;
        }
        if (n !== value) onCommit(n);
    };
    return (
        <input
            type="number"
            min={SALESBLINK_MIN_PER_DAY}
            max={SALESBLINK_MAX_PER_DAY}
            value={draft}
            onChange={(e) => setDraft(e.target.value)}
            onBlur={commit}
            onKeyDown={(e) => e.key === "Enter" && (e.target as HTMLInputElement).blur()}
            className="w-24 rounded-md border border-slate-300 px-3 py-1.5 text-[13px] outline-none focus:ring-2 focus:ring-sky-500/40"
        />
    );
}

function MailboxTable({ rows, loading }: { rows: ReturnType<typeof useSalesBlinkMailboxes>["data"] extends (infer T)[] | undefined ? T[] : never; loading: boolean }) {
    const enroll = useEnrollSalesBlinkMailbox();
    const unenroll = useUnenrollSalesBlinkMailbox();

    if (loading) {
        return (
            <div className="py-8 flex justify-center text-slate-400">
                <Loader2Icon className="w-4 h-4 animate-spin" />
            </div>
        );
    }
    if (!rows.length) {
        return <p className="py-6 text-[13px] text-slate-500">No mailboxes connected yet.</p>;
    }

    return (
        <div className="overflow-x-auto">
            <table className="w-full text-[13px]">
                <thead>
                    <tr className="text-left text-[11.5px] uppercase tracking-wide text-slate-500">
                        <th className="py-2 pr-4 font-medium">Mailbox</th>
                        <th className="py-2 pr-4 font-medium">Warmup</th>
                        <th className="py-2 pr-4 font-medium">Health</th>
                        <th className="py-2 font-medium" />
                    </tr>
                </thead>
                <tbody className="divide-y divide-slate-200/70">
                    {rows.map((r) => (
                        <tr key={r.email_account_id}>
                            <td className="py-2.5 pr-4">
                                <div className="text-slate-900">{r.email}</div>
                                {!r.eligible && r.ineligible_reason && (
                                    <div className="text-[12px] text-slate-500">{r.ineligible_reason}</div>
                                )}
                                {r.last_error && <div className="text-[12px] text-red-700">{r.last_error}</div>}
                            </td>
                            <td className="py-2.5 pr-4">
                                {r.enrolled ? (
                                    <span className="inline-flex items-center gap-1.5 text-emerald-700">
                                        <FlameIcon className="w-3.5 h-3.5" /> Warming
                                    </span>
                                ) : r.eligible ? (
                                    <span className="text-slate-500">Not enrolled</span>
                                ) : (
                                    <span className="text-slate-400">Unavailable</span>
                                )}
                            </td>
                            <td className="py-2.5 pr-4 text-slate-700">
                                {r.health_score == null ? "—" : `${r.health_score}`}
                            </td>
                            <td className="py-2.5 text-right">
                                {r.enrolled ? (
                                    <button
                                        onClick={() => unenroll.mutate(r.email_account_id, { onError: (e) => toast.error(buildError(e as unknown as AppError)) })}
                                        className="text-[12.5px] text-slate-600 hover:text-slate-900"
                                    >
                                        Stop
                                    </button>
                                ) : r.eligible ? (
                                    <button
                                        onClick={() =>
                                            enroll.mutate(r.email_account_id, {
                                                onSuccess: () => toast.success(`${r.email} is warming.`),
                                                onError: (e) => toast.error(buildError(e as unknown as AppError)),
                                            })
                                        }
                                        className="text-[12.5px] text-sky-700 hover:text-sky-900"
                                    >
                                        Warm
                                    </button>
                                ) : null}
                            </td>
                        </tr>
                    ))}
                </tbody>
            </table>
        </div>
    );
}
