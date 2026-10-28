// Deliverability events log: every delivered, deferred, bounced, complained,
// unsubscribed, opened, clicked and replied event the workspace recorded, with
// the send it belongs to. Live: the query rides the ["analytics"] invalidations.

import React from "react";
import { Link } from "react-router-dom";
import { ArrowLeftIcon, ChevronRightIcon, Loader2Icon } from "lucide-react";

import { NoAccess } from "@/components/layout/NoAccess";
import { EmptyBlock, Page, PageBody, PageTopbar, SectionBar } from "@/components/layout/Page";
import { SearchInput } from "@/components/ui/field";
import { usePermission } from "@/hooks/usePermission";
import { cn } from "@/lib/utils";
import useDeliverabilityEvents from "@/lib/api/hooks/app/suppression/useDeliverabilityEvents";
import type { DeliverabilityEventRow, DeliverabilityEventType } from "@/lib/api/models/app/suppression/DeliverabilityEvent";

const TYPES: { value: DeliverabilityEventType; label: string; tone: string }[] = [
    { value: "delivered", label: "Delivered", tone: "bg-emerald-50 text-emerald-700" },
    { value: "deferred", label: "Deferred", tone: "bg-amber-50 text-amber-700" },
    { value: "bounce", label: "Bounced", tone: "bg-rose-50 text-rose-700" },
    { value: "complaint", label: "Complaint", tone: "bg-rose-100 text-rose-800" },
    { value: "unsubscribe", label: "Unsubscribed", tone: "bg-slate-100 text-slate-700" },
    { value: "open", label: "Opened", tone: "bg-sky-50 text-sky-700" },
    { value: "click", label: "Clicked", tone: "bg-indigo-50 text-indigo-700" },
    { value: "reply", label: "Replied", tone: "bg-violet-50 text-violet-700" },
];
const TONE = Object.fromEntries(TYPES.map((t) => [t.value, t])) as Record<string, (typeof TYPES)[number]>;

const RANGES: { value: string; label: string; days: number | null }[] = [
    { value: "7d", label: "7d", days: 7 },
    { value: "30d", label: "30d", days: 30 },
    { value: "90d", label: "90d", days: 90 },
    { value: "all", label: "All", days: null },
];

export default function DeliverabilityEventsPage() {
    const canView = usePermission("VIEW_ANALYTICS");
    const [types, setTypes] = React.useState<DeliverabilityEventType[]>([]);
    const [q, setQ] = React.useState("");
    const [debounced, setDebounced] = React.useState("");
    const [range, setRange] = React.useState("30d");
    const [open, setOpen] = React.useState<string | null>(null);

    React.useEffect(() => {
        const t = setTimeout(() => setDebounced(q.trim()), 250);
        return () => clearTimeout(t);
    }, [q]);

    // Rounded to the hour so the query key stays stable between renders.
    const from = React.useMemo(() => {
        const days = RANGES.find((r) => r.value === range)?.days;
        if (!days) return undefined;
        const d = new Date(Date.now() - days * 86_400_000);
        d.setMinutes(0, 0, 0);
        return d.toISOString().replace(/\.\d{3}Z$/, "Z");
    }, [range]);

    const events = useDeliverabilityEvents({ types, q: debounced, from });
    const rows = events.data?.pages.flatMap((p) => p.data) ?? [];

    function toggleType(t: DeliverabilityEventType) {
        setTypes((cur) => (cur.includes(t) ? cur.filter((x) => x !== t) : [...cur, t]));
    }

    if (!canView) return <NoAccess feature="Deliverability events" permissionLabel="View analytics" />;

    return (
        <Page>
            <PageTopbar eyebrow="Deliverability" subtitle="Events log">
                <div className="flex items-center gap-0.5 rounded-md border border-slate-200 p-0.5">
                    {RANGES.map((r) => (
                        <button
                            key={r.value}
                            type="button"
                            onClick={() => setRange(r.value)}
                            className={cn(
                                "h-6 px-2 rounded text-[11.5px] transition-colors",
                                range === r.value ? "bg-slate-900 text-white" : "text-slate-500 hover:text-slate-900",
                            )}
                        >
                            {r.label}
                        </button>
                    ))}
                </div>
            </PageTopbar>
            <div className="px-5 pt-2">
                <Link to="/app/deliverability" className="inline-flex items-center gap-1 text-[11.5px] text-slate-500 hover:text-slate-900">
                    <ArrowLeftIcon className="w-3 h-3" />
                    Deliverability
                </Link>
            </div>

            <SectionBar label="Events" count={rows.length + (events.hasNextPage ? "+" : "")}>
                <div className="flex items-center gap-1 flex-wrap">
                    {TYPES.map((t) => (
                        <button
                            key={t.value}
                            type="button"
                            onClick={() => toggleType(t.value)}
                            className={cn(
                                "h-6 px-2 rounded text-[11.5px] transition-colors",
                                types.includes(t.value) ? t.tone + " font-medium" : "text-slate-500 hover:text-slate-900 hover:bg-slate-50",
                            )}
                        >
                            {t.label}
                        </button>
                    ))}
                </div>
                <SearchInput value={q} onChange={setQ} placeholder="Search recipient or reason" className="w-full md:w-64" />
            </SectionBar>

            <PageBody>
                {events.isPending ? (
                    <div className="px-5 py-4 space-y-2">
                        {[0, 1, 2, 3].map((i) => (
                            <div key={i} className="h-11 rounded bg-slate-50 animate-pulse" />
                        ))}
                    </div>
                ) : rows.length === 0 ? (
                    <div className="py-10">
                        <EmptyBlock
                            title={types.length || debounced ? "No matching events" : "No events in this window"}
                            body="Bounces, complaints and unsubscribes appear here as they happen. Connect Amazon SES under Integrations to add deliveries and delays."
                        />
                    </div>
                ) : (
                    <div className="divide-y divide-slate-200/60">
                        {rows.map((r) => (
                            <EventRow key={r.id} row={r} open={open === r.id} onToggle={() => setOpen(open === r.id ? null : r.id)} />
                        ))}
                        {events.hasNextPage && (
                            <div className="px-5 py-3">
                                <button
                                    type="button"
                                    onClick={() => events.fetchNextPage()}
                                    disabled={events.isFetchingNextPage}
                                    className="h-7 px-3 rounded-md border border-slate-200 hover:border-slate-300 text-[12px] text-slate-700 inline-flex items-center gap-1.5 transition-colors"
                                >
                                    {events.isFetchingNextPage && <Loader2Icon className="w-3 h-3 animate-spin" />}
                                    Load more
                                </button>
                            </div>
                        )}
                    </div>
                )}
            </PageBody>
        </Page>
    );
}

function EventRow({ row, open, onToggle }: { row: DeliverabilityEventRow; open: boolean; onToggle: () => void }) {
    const tone = TONE[row.event_type];
    const bounceType = typeof row.metadata?.bounce_type === "string" ? (row.metadata.bounce_type as string) : "";
    const meta = Object.entries(row.metadata ?? {}).filter(([, v]) => v !== "" && v !== null && v !== undefined);
    return (
        <div>
            <button
                type="button"
                onClick={onToggle}
                className="w-full min-h-11 px-5 py-2 flex items-center gap-3 text-left text-[12.5px] hover:bg-slate-50/60 transition-colors"
            >
                <ChevronRightIcon className={cn("w-3 h-3 text-slate-400 shrink-0 transition-transform", open && "rotate-90")} />
                <span className={cn("w-24 shrink-0 text-center text-[10.5px] px-1.5 py-0.5 rounded", tone?.tone ?? "bg-slate-100 text-slate-600")}>
                    {tone?.label ?? row.event_type}
                    {bounceType && ` · ${bounceType}`}
                </span>
                <div className="min-w-0 flex-1">
                    <div className="text-slate-900 truncate">{row.recipient_email}</div>
                    {row.reason && <div className="text-[11.5px] text-slate-500 truncate">{row.reason}</div>}
                </div>
                <span className="hidden lg:block w-48 truncate text-[11.5px] text-slate-500">{row.campaign_name || "—"}</span>
                <span className="hidden md:block w-48 truncate text-[11.5px] text-slate-500">{row.mailbox_email || "—"}</span>
                <span className="w-32 text-right text-[11.5px] text-slate-400 tabular-nums shrink-0">
                    {new Date(row.created_at).toLocaleString(undefined, { month: "short", day: "numeric", hour: "2-digit", minute: "2-digit" })}
                </span>
            </button>
            {open && (
                <div className="px-5 pb-3 pl-[52px] grid grid-cols-1 md:grid-cols-2 gap-x-6 gap-y-1 text-[11.5px]">
                    <Detail label="Source" value={row.provider} />
                    <Detail label="Campaign" value={row.campaign_name || "Not tied to a campaign"} />
                    <Detail label="Mailbox" value={row.mailbox_email || "Unknown"} />
                    <Detail label="Recorded" value={new Date(row.created_at).toLocaleString()} />
                    {meta.map(([k, v]) => (
                        <Detail key={k} label={k.replace(/_/g, " ")} value={String(v)} mono />
                    ))}
                </div>
            )}
        </div>
    );
}

function Detail({ label, value, mono }: { label: string; value: string; mono?: boolean }) {
    return (
        <div className="flex gap-2 min-w-0">
            <span className="w-28 shrink-0 text-slate-400 capitalize">{label}</span>
            <span className={cn("text-slate-700 truncate", mono && "font-mono")} title={value}>
                {value}
            </span>
        </div>
    );
}
