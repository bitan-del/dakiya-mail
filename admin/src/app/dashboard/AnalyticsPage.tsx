// Platform-wide analytics. Trend cards on top (4-up), daily email
// timeseries as a stacked bar chart, hourly load + per-worker load
// underneath. No external chart library — the bars are CSS so the
// admin bundle doesn't pay for recharts/d3 for one screen.

import { useQuery } from "@tanstack/react-query";
import { Link } from "react-router-dom";
import {
    BarChart3,
    Mail,
    Megaphone,
    TrendingDown,
    TrendingUp,
    Users,
} from "lucide-react";
import { PageHeader } from "@/components/layout/PageHeader";
import { Skeleton } from "@/components/ui/skeleton";
import {
    getAnalyticsTrends,
    getDailyEmailStats,
    getHourlyEmailStats,
    getUserGrowthStats,
    getWorkerLoadStats,
} from "@/lib/api/client/admin/analytics";
import type { DailyEmailStat, WorkerLoadStat } from "@/lib/api/models/admin";

export default function AnalyticsPage() {
    return (
        <div>
            <PageHeader
                title="Analytics"
                description="Platform-wide send, delivery, engagement, and worker-load metrics."
            />

            <TrendCards />

            <section className="mt-6">
                <h2 className="text-sm font-semibold mb-2">Email volume — last 30 days</h2>
                <DailyEmailChart />
            </section>

            <section className="mt-6 grid gap-4 md:grid-cols-2">
                <HourlyChart />
                <WorkerLoadList />
            </section>

            <section className="mt-6">
                <h2 className="text-sm font-semibold mb-2">User growth — last 30 days</h2>
                <UserGrowthChart />
            </section>
        </div>
    );
}

function TrendCards() {
    const { data, isLoading } = useQuery({
        queryKey: ["admin", "analytics", "trends"],
        queryFn: getAnalyticsTrends,
        staleTime: 60_000,
    });

    if (isLoading) {
        return (
            <div className="grid gap-3 md:grid-cols-4 mb-6">
                {Array.from({ length: 4 }).map((_, i) => (
                    <Skeleton key={i} className="h-24" />
                ))}
            </div>
        );
    }

    return (
        <div className="grid gap-3 md:grid-cols-4 mb-6">
            <TrendCard
                icon={<Users className="size-4" />}
                label="Users"
                pct={data?.users_growth_percent}
            />
            <TrendCard
                icon={<Mail className="size-4" />}
                label="Emails sent"
                pct={data?.emails_growth_percent}
            />
            <TrendCard
                icon={<Megaphone className="size-4" />}
                label="Campaigns"
                pct={data?.campaigns_growth_percent}
            />
            <TrendCard
                icon={<BarChart3 className="size-4" />}
                label="Revenue"
                pct={data?.revenue_growth_percent}
            />
        </div>
    );
}

function TrendCard({
    icon,
    label,
    pct,
}: {
    icon: React.ReactNode;
    label: string;
    pct?: number;
}) {
    const v = pct ?? 0;
    const up = v >= 0;
    const tone =
        v > 0 ? "text-emerald-600" : v < 0 ? "text-red-600" : "text-muted-foreground";
    return (
        <div className="border border-border rounded-lg p-3 bg-card">
            <div className="flex items-center gap-1.5 text-xs text-muted-foreground">
                {icon}
                <span>{label}</span>
            </div>
            <div className={`text-2xl font-semibold tabular-nums mt-1 ${tone}`}>
                {pct == null ? "—" : `${up && v > 0 ? "+" : ""}${v.toFixed(1)}%`}
            </div>
            <div className="text-[10px] text-muted-foreground flex items-center gap-1 mt-0.5">
                {v > 0 ? <TrendingUp className="size-3" /> : v < 0 ? <TrendingDown className="size-3" /> : null}
                vs. previous period
            </div>
        </div>
    );
}

function DailyEmailChart() {
    const { data, isLoading } = useQuery({
        queryKey: ["admin", "analytics", "emails", "daily"],
        queryFn: () => getDailyEmailStats(30),
        staleTime: 60_000,
    });

    if (isLoading) return <Skeleton className="h-48" />;
    const rows = data?.data ?? [];
    if (rows.length === 0) {
        return (
            <div className="text-sm text-muted-foreground border border-border rounded-md p-4 bg-card">
                No email activity in the last 30 days.
            </div>
        );
    }

    const maxSent = Math.max(1, ...rows.map((r) => r.total_sent));
    return (
        <div className="border border-border rounded-lg bg-card p-4">
            <div className="flex items-end gap-1 h-40">
                {rows.map((r) => (
                    <DayBar key={r.date} stat={r} maxSent={maxSent} />
                ))}
            </div>
            <Legend />
        </div>
    );
}

function DayBar({ stat, maxSent }: { stat: DailyEmailStat; maxSent: number }) {
    const h = (n: number) => `${Math.max(1, (n / maxSent) * 100)}%`;
    return (
        <div
            className="flex-1 flex flex-col items-center group relative"
            title={`${new Date(stat.date).toLocaleDateString()}\nsent: ${stat.total_sent}\ndelivered: ${stat.total_delivered}\nbounced: ${stat.total_bounced}\nreplied: ${stat.total_replied}`}
        >
            <div className="w-full flex-1 flex flex-col justify-end gap-0.5">
                {stat.total_bounced > 0 && (
                    <div
                        className="w-full bg-red-400"
                        style={{ height: h(stat.total_bounced) }}
                    />
                )}
                {stat.total_replied > 0 && (
                    <div
                        className="w-full bg-emerald-400"
                        style={{ height: h(stat.total_replied) }}
                    />
                )}
                {stat.total_delivered > 0 && (
                    <div
                        className="w-full bg-[var(--admin-accent)]"
                        style={{ height: h(stat.total_delivered - stat.total_replied - stat.total_bounced) }}
                    />
                )}
            </div>
            <div className="text-[8px] text-muted-foreground mt-1 -rotate-45 origin-top-left whitespace-nowrap">
                {new Date(stat.date).toLocaleDateString(undefined, {
                    month: "numeric",
                    day: "numeric",
                })}
            </div>
        </div>
    );
}

function Legend() {
    return (
        <div className="flex items-center gap-3 text-[10px] text-muted-foreground mt-3">
            <LegendDot color="bg-[var(--admin-accent)]" label="Delivered" />
            <LegendDot color="bg-emerald-400" label="Replied" />
            <LegendDot color="bg-red-400" label="Bounced" />
        </div>
    );
}

function LegendDot({ color, label }: { color: string; label: string }) {
    return (
        <span className="inline-flex items-center gap-1">
            <span className={`inline-block size-2 rounded ${color}`} />
            {label}
        </span>
    );
}

function HourlyChart() {
    const { data, isLoading } = useQuery({
        queryKey: ["admin", "analytics", "emails", "hourly"],
        queryFn: getHourlyEmailStats,
        staleTime: 60_000,
    });

    if (isLoading) return <Skeleton className="h-48" />;
    const rows = data?.data ?? [];
    const max = Math.max(1, ...rows.map((r) => r.total_sent));

    return (
        <div className="border border-border rounded-lg bg-card p-4">
            <h3 className="text-xs font-semibold mb-3">Today by hour</h3>
            {rows.length === 0 ? (
                <p className="text-xs text-muted-foreground">No sends today.</p>
            ) : (
                <div className="flex items-end gap-1 h-32">
                    {rows.map((r) => (
                        <div
                            key={r.hour}
                            className="flex-1 flex flex-col items-center"
                            title={`${r.hour}:00 — ${r.total_sent} sent`}
                        >
                            <div
                                className="w-full bg-[var(--admin-accent)]"
                                style={{ height: `${Math.max(1, (r.total_sent / max) * 100)}%` }}
                            />
                            <div className="text-[8px] text-muted-foreground mt-1">
                                {r.hour}
                            </div>
                        </div>
                    ))}
                </div>
            )}
        </div>
    );
}

function WorkerLoadList() {
    const { data, isLoading } = useQuery({
        queryKey: ["admin", "analytics", "workers", "load"],
        queryFn: getWorkerLoadStats,
        staleTime: 30_000,
    });

    if (isLoading) return <Skeleton className="h-48" />;
    const rows = (data?.data ?? []).slice().sort(
        (a, b) => b.emails_sent_today - a.emails_sent_today,
    );

    return (
        <div className="border border-border rounded-lg bg-card p-4">
            <h3 className="text-xs font-semibold mb-3">Worker load today</h3>
            {rows.length === 0 ? (
                <p className="text-xs text-muted-foreground">No workers reporting.</p>
            ) : (
                <ul className="space-y-1.5">
                    {rows.slice(0, 10).map((w) => (
                        <WorkerLoadRow key={w.worker_id} w={w} />
                    ))}
                </ul>
            )}
        </div>
    );
}

function WorkerLoadRow({ w }: { w: WorkerLoadStat }) {
    return (
        <li className="flex items-center justify-between text-xs gap-3">
            <Link
                to={`/workers/${w.worker_id}`}
                className="truncate text-[var(--admin-accent-strong)] hover:underline"
            >
                {w.worker_name || w.worker_id.slice(0, 8)}
            </Link>
            <span className="tabular-nums text-muted-foreground">
                {w.emails_sent_today.toLocaleString()} sent
                {w.queued_emails > 0 && (
                    <span className="ml-1 text-amber-600">
                        · {w.queued_emails} queued
                    </span>
                )}
            </span>
        </li>
    );
}

function UserGrowthChart() {
    const { data, isLoading } = useQuery({
        queryKey: ["admin", "analytics", "users", "growth"],
        queryFn: () => getUserGrowthStats(30),
        staleTime: 60_000,
    });

    if (isLoading) return <Skeleton className="h-48" />;
    const rows = data?.data ?? [];
    if (rows.length === 0) {
        return (
            <div className="text-sm text-muted-foreground border border-border rounded-md p-4 bg-card">
                No new users in the last 30 days.
            </div>
        );
    }
    const maxNew = Math.max(1, ...rows.map((r) => r.new_users));
    return (
        <div className="border border-border rounded-lg bg-card p-4">
            <div className="flex items-end gap-1 h-32">
                {rows.map((r) => (
                    <div
                        key={r.date}
                        className="flex-1 flex flex-col items-center"
                        title={`${new Date(r.date).toLocaleDateString()}\n+${r.new_users} new (${r.total_users} total)`}
                    >
                        <div
                            className="w-full bg-emerald-500"
                            style={{ height: `${Math.max(1, (r.new_users / maxNew) * 100)}%` }}
                        />
                    </div>
                ))}
            </div>
            <div className="text-[10px] text-muted-foreground mt-2">
                New users per day (hover for totals).
            </div>
        </div>
    );
}
