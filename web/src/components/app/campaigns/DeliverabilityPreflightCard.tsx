// Deliverability preflight card for the launch dialog.
//
// Lists every check the gate ran with what it found and how to fix it. It never
// hides a result and never disables Launch on its own: the dialog asks for an
// explicit confirmation when a check failed.

import { AlertTriangleIcon, CheckIcon, RefreshCwIcon, ShieldCheckIcon, XCircleIcon } from "lucide-react";
import type {
    DeliverabilityCheck,
    DeliverabilityPreflight,
    PreflightStatus,
} from "@/lib/api/models/app/campaigns/DeliverabilityPreflight";

const VERDICT: Record<PreflightStatus, { label: string; chip: string }> = {
    pass: { label: "Ready", chip: "bg-emerald-50 text-emerald-700" },
    warn: { label: "Review", chip: "bg-amber-50 text-amber-800" },
    fail: { label: "Risky", chip: "bg-rose-50 text-rose-700" },
};

const ROW: Record<PreflightStatus, { icon: React.ReactNode; tone: string; sr: string }> = {
    pass: { icon: <CheckIcon className="w-3.5 h-3.5 text-emerald-600" />, tone: "", sr: "Passed" },
    warn: { icon: <AlertTriangleIcon className="w-3.5 h-3.5 text-amber-600" />, tone: "bg-amber-50/60", sr: "Warning" },
    fail: { icon: <XCircleIcon className="w-3.5 h-3.5 text-rose-500" />, tone: "bg-rose-50/60", sr: "Failed" },
};

const ORDER: Record<PreflightStatus, number> = { fail: 0, warn: 1, pass: 2 };

function CheckRow({ check }: { check: DeliverabilityCheck }) {
    const row = ROW[check.status];
    const open = check.status !== "pass";
    return (
        <li className={`flex items-start gap-2 px-3 py-2 ${row.tone}`}>
            <span className="shrink-0 mt-0.5">
                {row.icon}
                <span className="sr-only">{row.sr}</span>
            </span>
            <div className="min-w-0 flex-1">
                <p className="text-[12.5px] font-medium text-slate-900 leading-snug">{check.label}</p>
                <p
                    className={`text-[11.5px] text-slate-600 leading-snug ${open ? "mt-0.5" : "truncate"}`}
                    title={open ? undefined : check.detail}
                >
                    {check.detail}
                </p>
                {open && check.fix_hint && (
                    <p className="text-[11.5px] text-slate-900 leading-snug mt-1">
                        <span className="font-medium">How to fix: </span>
                        {check.fix_hint}
                    </p>
                )}
            </div>
        </li>
    );
}

export default function DeliverabilityPreflightCard({
    data,
    loading,
    error,
    onRetry,
}: {
    data?: DeliverabilityPreflight;
    loading: boolean;
    error: boolean;
    onRetry: () => void;
}) {
    // Stable sort: failures first, then warnings, server order within each.
    const checks = [...(data?.checks ?? [])].sort((a, b) => ORDER[a.status] - ORDER[b.status]);

    return (
        <div className="rounded-md border border-slate-200 overflow-hidden">
            <div className="px-3 h-9 flex items-center gap-2 border-b border-slate-200 bg-slate-50/60">
                <ShieldCheckIcon className="w-3.5 h-3.5 text-slate-400" />
                <span className="text-[10px] uppercase tracking-[0.14em] text-slate-400 font-medium">
                    Deliverability preflight
                </span>
                {data && (
                    <span
                        className={`ml-auto h-5 px-1.5 rounded inline-flex items-center text-[11px] font-medium ${VERDICT[data.verdict].chip}`}
                    >
                        {VERDICT[data.verdict].label}
                    </span>
                )}
            </div>

            {loading ? (
                <div className="px-3 py-3 flex items-center gap-2 text-[11.5px] text-slate-400">
                    <span className="campaign-grid text-slate-300" aria-hidden />
                    Running deliverability checks…
                </div>
            ) : error ? (
                <div className="px-3 py-3 flex items-start gap-2">
                    <AlertTriangleIcon className="w-3.5 h-3.5 shrink-0 mt-0.5 text-amber-600" />
                    <p className="text-[11.5px] text-slate-600 leading-snug flex-1">
                        Could not run the deliverability checks. You can retry, or launch without them.
                    </p>
                    <button
                        type="button"
                        onClick={onRetry}
                        className="shrink-0 h-7 px-2 rounded-md border border-slate-200 hover:border-slate-300 text-slate-700 bg-white text-[12px] inline-flex items-center gap-1.5 transition-colors"
                    >
                        <RefreshCwIcon className="w-3 h-3" />
                        Retry
                    </button>
                </div>
            ) : (
                <ul className="divide-y divide-slate-100">
                    {checks.map((c) => (
                        <CheckRow key={c.key} check={c} />
                    ))}
                </ul>
            )}
        </div>
    );
}
